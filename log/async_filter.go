package vlog

import (
	"golang.org/x/time/rate"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

const (
	asyncFilterLogItemChanLen = 5000
)

var (
	lock                   = &sync.Mutex{}
	asyncFilterLogItemChan chan *AsyncFilterLogItem
	asyncFilters           = sync.Map{}
	asyncLimit             = rate.NewLimiter(rate.Every(time.Second*10), 1)
)

type Caller struct {
	Ok   bool
	File string
	Line int
}

type AsyncFilterLogItem struct {
	Level   LogLevel
	Name    string
	Format  string
	Fields  []interface{}
	Stack   string
	Callers map[string]*Caller
}

type AsyncFilter interface {
	StackRequired(level LogLevel, name string) bool
	CallerRequired(level LogLevel, name string) (bool, int)
	GetChannel() chan *AsyncFilterLogItem
}

func AddAsyncFilter(filterName string, filter AsyncFilter) {
	startAsyncFilterConsumer()
	asyncFilters.Store(filterName, filter)
}

func startAsyncFilterConsumer() {
	if asyncFilterLogItemChan != nil {
		return
	}
	lock.Lock()
	defer lock.Unlock()
	if asyncFilterLogItemChan != nil {
		return
	}
	asyncFilterLogItemChan = make(chan *AsyncFilterLogItem, asyncFilterLogItemChanLen)
	go func() {
		defer func() {
			if err := recover(); err != nil && asyncLimit.Allow() {
				Errorf("consumer error, err:%v", err)
			}
		}()
		for {
			select {
			case item := <-asyncFilterLogItemChan:
				asyncFilters.Range(func(filterName, filter interface{}) bool {
					select {
					case filter.(AsyncFilter).GetChannel() <- item:
					default:
						if asyncLimit.Allow() {
							Errorf("%s filter channel is full, drop log", filterName.(string))
						}
					}
					return true
				})
			}
		}
	}()
}

func doAsyncFilters(level LogLevel, name, format string, fields ...interface{}) {
	if asyncFilterLogItemChan == nil {
		return
	}

	item := &AsyncFilterLogItem{
		Level:   level,
		Name:    name,
		Format:  format,
		Fields:  fields,
		Stack:   "",
		Callers: map[string]*Caller{},
	}

	asyncFilters.Range(func(filterName, filter interface{}) bool {
		if item.Stack == "" || filter.(AsyncFilter).StackRequired(level, name) {
			item.Stack = string(debug.Stack())
		}

		requiredCaller, skip := filter.(AsyncFilter).CallerRequired(level, name)
		if requiredCaller {
			_, file, line, ok := runtime.Caller(skip)
			item.Callers[filterName.(string)] = &Caller{
				Ok:   ok,
				File: file,
				Line: line,
			}
		}
		return true
	})

	select {
	case asyncFilterLogItemChan <- item:
	default:
		if asyncLimit.Allow() {
			Errorf("async chan is full, drop log")
		}
	}
}
