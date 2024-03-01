package filter

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/config"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/metrics"
	"github.com/weibocom/motan-go/protocol"
)

func TestMetricsFilter(t *testing.T) {
	application := "testApplication"
	url := mockURL()
	url.PutParam(motan.ApplicationKey, application)
	mf := (&MetricsFilter{}).NewFilter(url).(motan.EndPointFilter)
	assert.NotNil(t, mf, "new filter")
	assert.Equal(t, motan.EndPointFilterType, int(mf.GetType()), "filter type")
	assert.Equal(t, Metrics, mf.GetName(), "filter name")

	// test filter
	factory := initFactory()
	mf = factory.GetFilter(Metrics).(motan.EndPointFilter)
	mf.SetNext(motan.GetLastEndPointFilter())
	mf.(*MetricsFilter).SetContext(&motan.Context{Config: config.NewConfig()})
	request := defaultRequest()

	request.GetRPCContext(true).Proxy = true
	request.SetAttachment(protocol.MSource, application)
	request.SetAttachment(protocol.MPath, testService)
	assert.Nil(t, metrics.GetStatItem(testGroup, "", testService), "metric stat")
	ep := factory.GetEndPoint(url)
	provider := factory.GetProvider(url)

	request2 := request.Clone().(motan.Request)
	request2.GetRPCContext(true).Proxy = false
	tests := []struct {
		name    string
		caller  motan.Caller
		request motan.Request
		keys    []string
	}{
		{name: "proxyClient", caller: ep, request: request, keys: []string{"motan-client-agent", application, testMethod}},
		{name: "proxyServer", caller: provider, request: request, keys: []string{"motan-server-agent", application, testMethod}},
		{name: "Client", caller: ep, request: request2, keys: []string{"motan-client", application, testMethod}},
		{name: "Server", caller: provider, request: request2, keys: []string{"motan-server", application, testMethod}},
	}
	var getKeysStr = func(keys []string) string {
		return metrics.Escape(keys[0]) + ":" + metrics.Escape(keys[1]) + ":" + metrics.Escape(keys[2])
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mf.Filter(test.caller, test.request)
			time.Sleep(10 * time.Millisecond)
			// The metrics filter has do escape
			assert.Equal(t, 1, int(metrics.GetStatItem(testGroup, "", testService).SnapshotAndClear().Count(getKeysStr(test.keys)+MetricsTotalCountSuffix)), "metric count")
		})
	}
	// test different client application
	motan.GetSwitcherManager().GetSwitcher(motan.MetricsReqApplication).SetValue(true)
	request3 := request.Clone().(motan.Request)
	request3.SetAttachment(protocol.MSource, "test")
	time.Sleep(10 * time.Millisecond)
	mf.Filter(ep, request3)
	time.Sleep(1000 * time.Millisecond)
	snapShot := metrics.GetStatItem(testGroup, "", testService).SnapshotAndClear()
	// The metrics filter has do escape
	assert.Equal(t, 1, int(snapShot.Count(getKeysStr([]string{"motan-client-agent", application, testMethod})+MetricsTotalCountSuffix)), "metric count")
	assert.Equal(t, 1, int(snapShot.Count(getKeysStr([]string{"motan-client-agent", "test", testMethod})+MetricsTotalCountSuffix)), "metric count")
	// test switcher
	motan.GetSwitcherManager().GetSwitcher(motan.MetricsReqApplication).SetValue(false)
	request4 := request.Clone().(motan.Request)
	time.Sleep(10 * time.Millisecond)
	mf.Filter(ep, request4)
	time.Sleep(1000 * time.Millisecond)
	snapShot1 := metrics.GetStatItem(testGroup, "", testService).SnapshotAndClear()
	// The metrics filter has do escape
	assert.Equal(t, 1, int(snapShot1.Count(getKeysStr([]string{"motan-client-agent", application, testMethod})+MetricsTotalCountSuffix)), "metric count")
	assert.Equal(t, 0, int(snapShot1.Count(getKeysStr([]string{"motan-client-agent", "test", testMethod})+MetricsTotalCountSuffix)), "metric count")
}

func TestAddMetric(t *testing.T) {
	keys := []string{"motan-client-agent", "testApplication", testMethod}
	factory := initFactory()
	mf := factory.GetFilter(Metrics).(motan.EndPointFilter)
	mf.(*MetricsFilter).SetContext(&motan.Context{Config: config.NewConfig()})
	response1 := &motan.MotanResponse{ProcessTime: 100}
	response2 := &motan.MotanResponse{ProcessTime: 100, Exception: &motan.Exception{ErrType: motan.BizException}}
	response3 := &motan.MotanResponse{ProcessTime: 100, Exception: &motan.Exception{ErrType: motan.FrameworkException}}
	response4 := &motan.MotanResponse{ProcessTime: 1000}
	var getKeysStr = func(keys []string) string {
		return metrics.Escape(keys[0]) + ":" + metrics.Escape(keys[1]) + ":" + metrics.Escape(keys[2])
	}
	tests := []struct {
		name     string
		response motan.Response
		keys     []string
	}{
		{name: "no exception", response: response1, keys: []string{MetricsTotalCountSuffix}},
		{name: "biz exception", response: response2, keys: []string{MetricsTotalCountSuffix, MetricsBizErrorCountSuffix}},
		{name: "other exception", response: response3, keys: []string{MetricsTotalCountSuffix, MetricsOtherErrorCountSuffix}},
		{name: "slow count", response: response4, keys: []string{MetricsTotalCountSuffix, MetricsSlowCountSuffix}},
		{name: "time", response: response1, keys: []string{MetricsTotalCountSuffix, ".Less200ms"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			addMetricWithKeys(testGroup, "", testService, keys, test.response.GetProcessTime(), test.response)
			time.Sleep(10 * time.Millisecond)
			snap := metrics.GetStatItem(testGroup, "", testService).SnapshotAndClear()
			for _, k := range test.keys {
				assert.True(t, snap.Count(getKeysStr(keys)+k) > 0, fmt.Sprintf("key '%s'", k))
			}
		})
	}
}
