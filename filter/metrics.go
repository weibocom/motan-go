package filter

import (
	"fmt"
	"strings"
	"sync"
	"time"

	motan "github.com/weibocom/motan-go/core"

	"github.com/weibocom/motan-go/metrics"
)

var initSync sync.Once

type MetricsFilter struct {
	next motan.EndPointFilter
}

func (m *MetricsFilter) GetIndex() int {
	return 2
}

func (m *MetricsFilter) NewFilter(url *motan.URL) motan.Filter {
	return &MetricsFilter{}
}

func (m *MetricsFilter) GetName() string {
	return "metrics"
}

func (m *MetricsFilter) HasNext() bool {
	if m.next != nil {
		return true
	}
	return false
}

func (m *MetricsFilter) GetType() int32 {
	return motan.EndPointFilterType
}

func (m *MetricsFilter) SetNext(e motan.EndPointFilter) {
	m.next = e
	return
}

func (m *MetricsFilter) GetNext() motan.EndPointFilter {
	if m.next != nil {
		return m.next
	}
	return nil
}

func (m *MetricsFilter) Filter(caller motan.Caller, request motan.Request) motan.Response {
	role := "server"
	switch caller.(type){
	case motan.Provider:
		role = "server-agent"
	case motan.EndPoint:
		role = "client-agent"
	}

	start := time.Now()

	response := m.GetNext().Filter(caller, request)

	key := strings.Map(func(r rune) rune {
		if metrics.Charmap[r] {
			return '_'
		}
		return r
	}, fmt.Sprintf("motan-%s:%s:%s:%s:%s", role,request.GetAttachments()["M_s"], request.GetAttachments()["M_g"], request.GetAttachments()["M_p"], request.GetMethod()))
	keyCount := key + ".total_count"
	metrics.AddCounter(keyCount, 1) //total_count

	if response.GetException() != nil { //err_count
		exception := response.GetException()
		if exception.ErrType == motan.BizException {
			bizErrCountKey := key + ".biz_error_count"
			metrics.AddCounter(bizErrCountKey, 1)
		} else {
			otherErrCountKey := key + ".other_error_count"
			metrics.AddCounter(otherErrCountKey, 1)
		}
	}

	end := time.Now()
	cost := end.Sub(start).Nanoseconds() / 1e6
	metrics.AddCounter((key + "." + metrics.ElapseTimeString(cost)), 1)

	if cost > 200 {
		metrics.AddCounter(key+".slow_count", 1)
	}

	metrics.AddHistograms(key, cost)
	return response
}

func (m *MetricsFilter) SetContext(context *motan.Context) {
	initSync.Do(func() {
		metrics.Run(context.Config)
	})
}
