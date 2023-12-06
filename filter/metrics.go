package filter

import (
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/metrics"
	"github.com/weibocom/motan-go/protocol"
)

const (
	MetricsTotalCountSuffix    = ".total_count"
	MetricsTotalCountSuffixLen = len(MetricsTotalCountSuffix)

	MetricsBizErrorCountSuffix   = ".biz_error_count"
	MetricsOtherErrorCountSuffix = ".other_error_count"
	MetricsSlowCountSuffix       = ".slow_count"
)

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
	return Metrics
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
	start := time.Now()
	response := m.GetNext().Filter(caller, request)

	proxy := false
	provider := false
	ctx := request.GetRPCContext(false)
	if ctx != nil {
		proxy = ctx.Proxy
	}
	// get role
	role := "motan-client"
	switch caller.(type) {
	case motan.Provider:
		provider = true
		if proxy {
			role = "motan-server-agent"
		} else {
			role = "motan-server"
		}
	case motan.EndPoint:
		if proxy {
			role = "motan-client-agent"
		}
	}
	//get application
	application := request.GetAttachment(protocol.MSource)
	if provider {
		application = caller.GetURL().GetParam(motan.ApplicationKey, "")
	}
	key := metrics.Escape(role) +
		":" + metrics.Escape(application) +
		":" + metrics.Escape(request.GetMethod())
	addMetric(metrics.Escape(request.GetAttachment(protocol.MGroup)),
		metrics.Escape(request.GetAttachment(protocol.MPath)),
		key, time.Since(start).Nanoseconds()/1e6, response)
	return response
}

func addMetric(group string, service string, key string, cost int64, response motan.Response) {
	metrics.AddCounter(group, service, key+MetricsTotalCountSuffix, 1) //total_count
	if response.GetException() != nil {                                //err_count
		exception := response.GetException()
		if exception.ErrType == motan.BizException {
			metrics.AddCounter(group, service, key+MetricsBizErrorCountSuffix, 1)
		} else {
			metrics.AddCounter(group, service, key+MetricsOtherErrorCountSuffix, 1)
		}
	}
	metrics.AddCounter(group, service, key+metrics.ElapseTimeSuffix(cost), 1)
	if cost > 200 {
		metrics.AddCounter(group, service, key+MetricsSlowCountSuffix, 1)
	}
	metrics.AddHistograms(group, service, key, cost)
}

func (m *MetricsFilter) SetContext(context *motan.Context) {
	metrics.StartReporter(context)
}
