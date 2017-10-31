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

func (g *MetricsFilter) GetIndex() int {
	return 2
}

func (g *MetricsFilter) NewFilter(url *motan.Url) motan.Filter {
	return &MetricsFilter{}
}

func (g *MetricsFilter) GetName() string {
	return "metrics"
}

func (g *MetricsFilter) HasNext() bool {
	if g.next != nil {
		return true
	}
	return false
}

func (g *MetricsFilter) GetType() int32 {
	return motan.EndPointFilterType
}

func (g *MetricsFilter) SetNext(e motan.EndPointFilter) {
	g.next = e
	return
}

func (g *MetricsFilter) GetNext() motan.EndPointFilter {
	if g.next != nil {
		return g.next
	}
	return nil
}

func (g *MetricsFilter) Filter(caller motan.Caller, request motan.Request) motan.Response {
	start := time.Now()

	response := g.GetNext().Filter(caller, request)

	m_p := strings.Replace(request.GetAttachments()["M_p"], ".", "_", -1)
	key := fmt.Sprintf("%s:%s:%s:%s", request.GetAttachments()["M_s"], request.GetAttachments()["M_g"], m_p, request.GetMethod())
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
