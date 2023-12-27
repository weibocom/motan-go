package filter

import (
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/metrics"
	"github.com/weibocom/motan-go/protocol"
	"sync"
	"time"
)

const (
	MetricsTotalCountSuffix    = ".total_count"
	MetricsTotalCountSuffixLen = len(MetricsTotalCountSuffix)

	MetricsBizErrorCountSuffix   = ".biz_error_count"
	MetricsOtherErrorCountSuffix = ".other_error_count"
	MetricsSlowCountSuffix       = ".slow_count"
)

var (
	metricOnce            = sync.Once{}
	metricsReqAppSwitcher *motan.Switcher
)

type MetricsFilter struct {
	next motan.EndPointFilter
}

func (m *MetricsFilter) GetRuntimeInfo() map[string]interface{} {
	return GetFilterRuntimeInfo(m)
}

func (m *MetricsFilter) GetIndex() int {
	return 2
}

func (m *MetricsFilter) NewFilter(url *motan.URL) motan.Filter {
	initReqAppSwitcher()
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
	start := getFilterStartTime(caller, request)
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
	application := caller.GetURL().GetParam(motan.ApplicationKey, "")
	if !provider && metricsReqAppSwitcher.IsOpen() {
		// to support application in caller URL
		reqApplication := request.GetAttachment(protocol.MSource)
		if application != reqApplication {
			keys := []string{role, reqApplication, request.GetMethod()}
			addMetricWithKeys(request.GetAttachment(protocol.MGroup), "", request.GetServiceName(),
				keys, time.Since(start).Nanoseconds()/1e6, response)
		}
	}
	keys := []string{role, application, request.GetMethod()}
	addMetricWithKeys(request.GetAttachment(protocol.MGroup), "", request.GetServiceName(),
		keys, time.Since(start).Nanoseconds()/1e6, response)
	return response
}

// addMetricWithKeys arguments: group & groupSuffix & service &  keys elements is text without escaped
func addMetricWithKeys(group, groupSuffix string, service string, keys []string, cost int64, response motan.Response) {
	metrics.AddCounterWithKeys(group, groupSuffix, service, keys, MetricsTotalCountSuffix, 1) //total_count
	if response.GetException() != nil {                                                       //err_count
		exception := response.GetException()
		if exception.ErrType == motan.BizException {
			metrics.AddCounterWithKeys(group, groupSuffix, service, keys, MetricsBizErrorCountSuffix, 1)
		} else {
			metrics.AddCounterWithKeys(group, groupSuffix, service, keys, MetricsOtherErrorCountSuffix, 1)
		}
	}
	metrics.AddCounterWithKeys(group, groupSuffix, service, keys, metrics.ElapseTimeSuffix(cost), 1)
	if cost > 200 {
		metrics.AddCounterWithKeys(group, groupSuffix, service, keys, MetricsSlowCountSuffix, 1)
	}
	metrics.AddHistogramsWithKeys(group, groupSuffix, service, keys, "", cost)
}

func (m *MetricsFilter) SetContext(context *motan.Context) {
	metrics.StartReporter(context)
}

func initReqAppSwitcher() {
	// registry default switcher value here
	// if the switcher has already been registered in Context.Initialize,
	// the default value will not overwrite it.
	metricOnce.Do(func() {
		metricsReqAppSwitcher = motan.GetSwitcherManager().GetOrRegister(motan.MetricsReqApplication, false)
	})
}
