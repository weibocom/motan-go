package vlog

import (
	"encoding/json"
	"fmt"
	"golang.org/x/time/rate"
	"runtime"
	"sync"
	"time"
)

type Entrypoint int

const (
	EntrypointInfoln Entrypoint = iota
	EntrypointInfof
	EntrypointWarningln
	EntrypointWarningf
	EntrypointErrorln
	EntrypointErrorf
	EntrypointFatalln
	EntrypointFatalf
	EntrypointAccessLog
	EntrypointMetricsLog
)

var (
	asyncFilterItemChan = make(chan *FilterItem, 5000)
	asyncFilterLock     = sync.RWMutex{}
	asyncFilters        = make(map[string]AsyncFilter)
	// limit log frequency when channel full
	asyncLimit = rate.NewLimiter(rate.Every(time.Second*5), 1)
)

type Caller struct {
	Ok   bool
	File string
	Line int
}

type FilterItem struct {
	Level      LogLevel
	Content    string
	Entrypoint Entrypoint
	Caller     *Caller
}

type AsyncFilter interface {
	GetChannel() chan *FilterItem
}

func init() {
	startAsyncFilterConsumer()
}

func AddAsyncFilter(filterName string, filter AsyncFilter) {
	asyncFilterLock.Lock()
	defer asyncFilterLock.Unlock()
	asyncFilters[filterName] = filter
}

func startAsyncFilterConsumer() {
	go func() {
		for {
			select {
			case item := <-asyncFilterItemChan:
				asyncFilterLock.RLock()
				for filterName, filter := range asyncFilters {
					callAsyncFilter(filterName, filter, item)
				}
				asyncFilterLock.RUnlock()
			}
		}
	}()
}

func callAsyncFilter(filterName string, filter AsyncFilter, item *FilterItem) {
	defer func() {
		//limit high frequency log
		if err := recover(); err != nil && asyncLimit.Allow() {
			Errorf("asyncFilter %s consumer error, err:%v", filterName, err)
		}
	}()
	select {
	case filter.GetChannel() <- item:
	default:
		//limit high frequency log
		if asyncLimit.Allow() {
			Errorf("asyncFilter %s  channel is full, drop log", filterName)
		}
	}
}

func doAsyncFilters(level LogLevel, entrypoint Entrypoint, format string, fields ...interface{}) {
	if len(asyncFilters) == 0 {
		return
	}
	item := &FilterItem{
		Level:      level,
		Content:    getLogContent(entrypoint, format, fields...),
		Entrypoint: entrypoint,
		Caller:     getCaller(),
	}

	select {
	case asyncFilterItemChan <- item:
	default:
		//limit high frequency log
		if asyncLimit.Allow() {
			Errorf("vlog asyncFilter chan is full, drop log")
		}
	}
}

func getCaller() *Caller {
	if !*filterCallerSwitch {
		return nil
	}
	_, file, line, ok := runtime.Caller(3)
	return &Caller{
		Ok:   ok,
		File: file,
		Line: line,
	}
}

func getLogContent(entrypoint Entrypoint, format string, fields ...interface{}) string {
	if entrypoint == EntrypointAccessLog {
		accessLog, _ := json.Marshal(fields[0].(*AccessLogEntity))
		return string(accessLog)
	}
	msg := format
	if msg == "" && len(fields) > 0 {
		msg = fmt.Sprint(fields...)
	} else if msg != "" && len(fields) > 0 {
		msg = fmt.Sprintf(format, fields...)
	}
	return msg
}
