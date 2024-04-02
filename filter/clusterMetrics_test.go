package filter

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/config"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/metrics"
	"github.com/weibocom/motan-go/protocol"
	"testing"
	"time"
)

func TestClusterMetricsFilter(t *testing.T) {
	application := "testApplication"
	url := mockURL()
	url.PutParam(motan.ApplicationKey, application)
	url.PutParam("haStrategy", "test")
	mf := (&ClusterMetricsFilter{}).NewFilter(url).(motan.ClusterFilter)
	assert.NotNil(t, mf, "new filter")
	assert.Equal(t, motan.ClusterFilterType, int(mf.GetType()), "filter type")
	assert.Equal(t, ClusterMetrics, mf.GetName(), "filter name")

	metrics.StartReporter(&motan.Context{Config: config.NewConfig()})
	// test filter
	factory := initFactory()
	mf = factory.GetFilter(ClusterMetrics).(motan.ClusterFilter)
	mf.SetNext(motan.GetLastClusterFilter())
	request := defaultRequest()
	var getKeysStr = func(keys []string) string {
		return metrics.Escape(keys[0]) + ":" + metrics.Escape(keys[1]) + ":" + metrics.Escape(keys[2])
	}
	request.GetRPCContext(true).Proxy = true
	request.SetAttachment(protocol.MSource, application)
	request.SetAttachment(protocol.MPath, testService)
	assert.Nil(t, metrics.GetStatItem(testGroup, "", testService), "metric stat")
	haStrategy := &testHA{url}
	lb := factory.GetLB(url)
	// test different client application
	motan.GetSwitcherManager().GetSwitcher(motan.MetricsReqApplication).SetValue(true)
	request3 := request.Clone().(motan.Request)
	request3.SetAttachment(protocol.MSource, "test")
	time.Sleep(10 * time.Millisecond)
	mf.Filter(haStrategy, lb, request3)
	time.Sleep(1000 * time.Millisecond)
	snapShot := metrics.GetStatItem(testGroup, ".cluster", testService).SnapshotAndClear()
	// The metrics filter has do escape
	assert.Equal(t, 1, int(snapShot.Count(getKeysStr([]string{"motan-client-agent", application, testMethod})+MetricsTotalCountSuffix)), "metric count")
	assert.Equal(t, 1, int(snapShot.Count(getKeysStr([]string{"motan-client-agent", "test", testMethod})+MetricsTotalCountSuffix)), "metric count")
	// test switcher
	motan.GetSwitcherManager().GetSwitcher(motan.MetricsReqApplication).SetValue(false)
	request4 := request.Clone().(motan.Request)
	time.Sleep(10 * time.Millisecond)
	mf.Filter(haStrategy, lb, request4)
	time.Sleep(1000 * time.Millisecond)
	snapShot1 := metrics.GetStatItem(testGroup, ".cluster", testService).SnapshotAndClear()
	// The metrics filter has do escape
	assert.Equal(t, 1, int(snapShot1.Count(getKeysStr([]string{"motan-client-agent", application, testMethod})+MetricsTotalCountSuffix)), "metric count")
	assert.Equal(t, 0, int(snapShot1.Count(getKeysStr([]string{"motan-client-agent", "test", testMethod})+MetricsTotalCountSuffix)), "metric count")
}

type testHA struct {
	url *motan.URL
}

func (f *testHA) GetName() string {
	return "test"
}

func (f *testHA) GetURL() *motan.URL {
	return f.url
}

func (f *testHA) SetURL(url *motan.URL) {
	f.url = url
}

func (f *testHA) Call(request motan.Request, loadBalance motan.LoadBalance) motan.Response {
	errorResponse := getErrorResponseWithCode(request.GetRequestID(), 500, fmt.Sprintf("test"))
	return errorResponse
}

func getErrorResponseWithCode(requestID uint64, errCode int, errMsg string) *motan.MotanResponse {
	return motan.BuildExceptionResponse(requestID, &motan.Exception{ErrCode: errCode, ErrMsg: errMsg, ErrType: motan.ServiceException})
}
