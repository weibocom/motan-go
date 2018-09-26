package filter

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/config"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/metrics"
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
	request.SetAttachment("M_s", application)
	request.SetAttachment("M_p", testService)
	assert.Nil(t, metrics.GetStatItem(testGroup, testService), "metirc stat")
	ep := factory.GetEndPoint(url)
	provider := factory.GetProvider(url)

	request2 := request.Clone().(motan.Request)
	request2.GetRPCContext(true).Proxy = false
	tests := []struct {
		name    string
		caller  motan.Caller
		request motan.Request
		key     string
	}{
		{name: "proxyClient", caller: ep, request: request, key: "motan-client-agent:" + application + ":" + testMethod},
		{name: "proxyServer", caller: provider, request: request, key: "motan-server-agent:" + application + ":" + testMethod},
		{name: "Client", caller: ep, request: request2, key: "motan-client:" + application + ":" + testMethod},
		{name: "Server", caller: provider, request: request2, key: "motan-server:" + application + ":" + testMethod},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mf.Filter(test.caller, test.request)
			time.Sleep(10 * time.Millisecond)
			assert.Equal(t, 1, int(metrics.GetStatItem(testGroup, testService).SnapshotAndClear().Count(test.key+".total_count")), "metric count")
		})
	}
}

func TestAddMetric(t *testing.T) {
	key := "motan-client-agent:testApplication:" + testMethod
	factory := initFactory()
	mf := factory.GetFilter(Metrics).(motan.EndPointFilter)
	mf.(*MetricsFilter).SetContext(&motan.Context{Config: config.NewConfig()})
	response1 := &motan.MotanResponse{ProcessTime: 100}
	response2 := &motan.MotanResponse{ProcessTime: 100, Exception: &motan.Exception{ErrType: motan.BizException}}
	response3 := &motan.MotanResponse{ProcessTime: 100, Exception: &motan.Exception{ErrType: motan.FrameworkException}}
	response4 := &motan.MotanResponse{ProcessTime: 1000}

	tests := []struct {
		name     string
		response motan.Response
		keys     []string
	}{
		{name: "no exception", response: response1, keys: []string{".total_count"}},
		{name: "biz exception", response: response2, keys: []string{".total_count", ".biz_error_count"}},
		{name: "other exception", response: response3, keys: []string{".total_count", ".other_error_count"}},
		{name: "slow count", response: response4, keys: []string{".total_count", ".slow_count"}},
		{name: "time", response: response1, keys: []string{".total_count", ".Less200ms"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			addMetric(testGroup, testService, key, test.response.GetProcessTime(), test.response)
			time.Sleep(10 * time.Millisecond)
			snap := metrics.GetStatItem(testGroup, testService).SnapshotAndClear()
			for _, k := range test.keys {
				assert.True(t, snap.Count(key+k) > 0, fmt.Sprintf("key '%s'", k))
			}
		})
	}
}
