package filter

import (
	assert2 "github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
	lb2 "github.com/weibocom/motan-go/lb"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	clusterCount               = int64(0)
	clusterCountLock           sync.RWMutex
	clusterFilterSleepTime     = 7 * time.Millisecond
	clusterFilterSleepTimeLock sync.RWMutex
)

func TestClusterCircuitBreakerFilter(t *testing.T) {
	//Init
	defaultExtFactory := &core.DefaultExtensionFactory{}
	defaultExtFactory.Initialize()
	RegistDefaultFilters(defaultExtFactory)
	lb2.RegistDefaultLb(defaultExtFactory)
	endpoint.RegistDefaultEndpoint(defaultExtFactory)
	url := &core.URL{Host: "127.0.0.1", Port: 8888, Protocol: "mockEndpoint"}
	ha := new(mockHA)
	lb := defaultExtFactory.GetLB(url)
	lb.OnRefresh([]core.EndPoint{defaultExtFactory.GetEndPoint(url)})
	request := &core.MotanRequest{Method: "testMethod"}

	//Test NewFilter
	param := map[string]string{core.TimeOutKey: "2", SleepWindowField: "300", IncludeBizException: "jia"}
	filterURL := &core.URL{Host: "127.0.0.1", Port: 8888, Protocol: "mockEndpoint", Parameters: param}
	f := defaultExtFactory.GetFilter(ClusterCircuitBreaker)
	if f == nil {
		t.Error("Can not find circuitBreaker filter!")
	} else {
		f = f.NewFilter(filterURL)
	}
	ef := f.(core.ClusterFilter)
	ef.SetNext(new(mockClusterFilter))

	//Test circuitBreakerTimeout & requestVolumeThreshold
	for i := 0; i < 30; i++ {
		ef.Filter(ha, lb, request)
	}
	time.Sleep(10 * time.Millisecond) //wait until async call complete
	clusterCountLock.RLock()
	if clusterCount != 20 && clusterCount != 21 {
		t.Error("Test clusterCircuitBreakerTimeout failed! count:", clusterCount)
	}
	clusterCountLock.RUnlock()

	//Test sleepWindow
	time.Sleep(350 * time.Millisecond) //wait until SleepWindowField
	for i := 0; i < 5; i++ {
		ef.Filter(ha, lb, request)
	}
	time.Sleep(10 * time.Millisecond) //wait until async call complete
	clusterCountLock.RLock()
	if clusterCount != 21 && clusterCount != 22 {
		t.Error("Test sleepWindow failed! count:", clusterCount)
	}
	clusterCountLock.RUnlock()

	////Test errorPercentThreshold
	//time.Sleep(350 * time.Millisecond) //wait until SleepWindowField
	//clusterFilterSleepTimeLock.Lock()
	//clusterFilterSleepTime = 0 * time.Millisecond
	//clusterFilterSleepTimeLock.Unlock()
	//for i := 0; i < 100; i++ {
	//	ef.Filter(ha, lb, request)
	//}
	//time.Sleep(10 * time.Millisecond) //wait until async call complete
	//clusterFilterSleepTimeLock.Lock()
	//clusterFilterSleepTime = 7 * time.Millisecond
	//clusterFilterSleepTimeLock.Unlock()
	//for i := 0; i < 50; i++ {
	//	ef.Filter(ha, lb, request)
	//}
	//time.Sleep(5000 * time.Millisecond) //wait until async call complete
	//clusterCountLock.RLock()
	//if clusterCount != 171 && clusterCount != 172 {
	//	t.Error("Test sleepWindow failed! count: ", clusterCount)
	//}
	//clusterCountLock.RUnlock()
}

func TestClusterCircuitBreakerOther(t *testing.T) {
	assert := assert2.New(t)
	defaultExtFactory := &core.DefaultExtensionFactory{}
	defaultExtFactory.Initialize()
	RegistDefaultFilters(defaultExtFactory)
	f := defaultExtFactory.GetFilter(ClusterCircuitBreaker)
	assert.Equal(int(f.GetType()), core.ClusterFilterType)
	assert.Equal(f.GetIndex(), 20)
	assert.Equal(f.HasNext(), false)
	assert.Equal(f.GetName(), ClusterCircuitBreaker)
}

type mockClusterFilter struct{}

func (c *mockClusterFilter) GetIndex() int {
	return 101
}

func (c *mockClusterFilter) NewFilter(*core.URL) core.Filter {
	return core.GetLastClusterFilter()
}

func (c *mockClusterFilter) Filter(ha core.HaStrategy, lb core.LoadBalance, request core.Request) core.Response {
	clusterCountLock.Lock()
	atomic.AddInt64(&clusterCount, 1)
	clusterCountLock.Unlock()
	clusterFilterSleepTimeLock.RLock()
	time.Sleep(clusterFilterSleepTime)
	clusterFilterSleepTimeLock.RUnlock()
	return ha.Call(request, lb)
}

func (c *mockClusterFilter) GetName() string {
	return ClusterCircuitBreaker
}

func (c *mockClusterFilter) HasNext() bool {
	return false
}

func (c *mockClusterFilter) GetType() int32 {
	return core.ClusterFilterType
}

func (c *mockClusterFilter) SetNext(core.ClusterFilter) {
	return
}

func (c *mockClusterFilter) GetNext() core.ClusterFilter {
	return nil
}

type mockHA struct {
	url *core.URL
}

func (f *mockHA) GetName() string {
	return "mockHA"
}
func (f *mockHA) GetURL() *core.URL {
	return f.url
}
func (f *mockHA) SetURL(url *core.URL) {
	f.url = url
}
func (f *mockHA) Call(request core.Request, loadBalance core.LoadBalance) core.Response {
	ep := loadBalance.Select(request)
	if ep == nil {
		return defaultErrMotanResponse(request, "no ep")
	}
	response := ep.Call(request)
	if response.GetException() == nil || response.GetException().ErrType == core.BizException {
		return response
	}
	lastErr := response.GetException()
	return defaultErrMotanResponse(request, lastErr.ErrMsg)
}
