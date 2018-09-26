package filter

import (
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/metrics"
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
	application := request.GetAttachment("M_s")
	if provider {
		application = caller.GetURL().GetParam(motan.ApplicationKey, "")
	}
	key := role + ":" + application + ":" + request.GetMethod()
	addMetric(request.GetAttachment("M_g"), request.GetAttachment("M_p"), key, time.Since(start).Nanoseconds()/1e6, response)
	return response
}

func addMetric(group string, service string, key string, cost int64, response motan.Response) {
	metrics.AddCounter(group, service, key+".total_count", 1) //total_count
	if response.GetException() != nil {                       //err_count
		exception := response.GetException()
		if exception.ErrType == motan.BizException {
			metrics.AddCounter(group, service, key+".biz_error_count", 1)
		} else {
			metrics.AddCounter(group, service, key+".other_error_count", 1)
		}
	}
	metrics.AddCounter(group, service, key+metrics.ElapseTimeSuffix(cost), 1)
	if cost > 200 {
		metrics.AddCounter(group, service, key+".slow_count", 1)
	}
	metrics.AddHistograms(group, service, key, cost)
}

func (m *MetricsFilter) SetContext(context *motan.Context) {
	metrics.StartReporter(context)
}
