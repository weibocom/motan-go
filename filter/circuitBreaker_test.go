package filter

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
	"github.com/weibocom/motan-go/log"
)

var (
	count               = int64(0)
	countLock           sync.RWMutex
	filterSleepTime     = 7 * time.Millisecond
	filterSleepTimeLock sync.RWMutex
)

func TestCircuitBreakerFilter(t *testing.T) {
	//Init
	defaultExtFactory := &core.DefaultExtensionFactory{}
	defaultExtFactory.Initialize()
	RegistDefaultFilters(defaultExtFactory)
	endpoint.RegistDefaultEndpoint(defaultExtFactory)
	url := &core.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint"}
	caller := defaultExtFactory.GetEndPoint(url)
	request := &core.MotanRequest{Method: "testMethod"}

	//Test NewFilter
	param := map[string]string{core.TimeOutKey: "2", SleepWindowField: "300"}
	filterURL := &core.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint", Parameters: param}
	f := defaultExtFactory.GetFilter(CircuitBreaker)
	if f == nil {
		t.Error("Can not find circuitBreaker filter!")
	}
	f = f.NewFilter(filterURL)
	ef := f.(core.EndPointFilter)
	ef.SetNext(new(mockEndPointFilter))

	//Test circuitBreakerTimeout & requestVolumeThreshold
	for i := 0; i < 30; i++ {
		ef.Filter(caller, request)
	}
	time.Sleep(10 * time.Millisecond) //wait until async call complete
	countLock.RLock()
	if count != 20 && count != 21 {
		t.Error("Test circuitBreakerTimeout failed! count:", count)
	}
	countLock.RUnlock()

	//Test sleepWindow
	time.Sleep(350 * time.Millisecond) //wait until SleepWindowField
	for i := 0; i < 5; i++ {
		ef.Filter(caller, request)
	}
	time.Sleep(10 * time.Millisecond) //wait until async call complete
	countLock.RLock()
	if count != 21 && count != 22 {
		t.Error("Test sleepWindow failed! count:", count)
	}
	countLock.RUnlock()

	//Test errorPercentThreshold
	time.Sleep(350 * time.Millisecond) //wait until SleepWindowField
	filterSleepTimeLock.Lock()
	filterSleepTime = 0 * time.Millisecond
	filterSleepTimeLock.Unlock()
	for i := 0; i < 100; i++ {
		ef.Filter(caller, request)
	}
	time.Sleep(10 * time.Millisecond) //wait until async call complete
	filterSleepTimeLock.Lock()
	filterSleepTime = 7 * time.Millisecond
	filterSleepTimeLock.Unlock()
	for i := 0; i < 50; i++ {
		ef.Filter(caller, request)
	}
	time.Sleep(10 * time.Millisecond) //wait until async call complete
	countLock.RLock()
	if count != 171 && count != 172 {
		t.Error("Test sleepWindow failed! count:", count)
	}
	countLock.RUnlock()
}

type mockEndPointFilter struct{}

func (m *mockEndPointFilter) GetName() string {
	return "mockEndPointFilter"
}

func (m *mockEndPointFilter) NewFilter(url *core.URL) core.Filter {
	return core.GetLastEndPointFilter()
}

func (m *mockEndPointFilter) Filter(caller core.Caller, request core.Request) core.Response {
	countLock.Lock()
	atomic.AddInt64(&count, 1)
	countLock.Unlock()
	filterSleepTimeLock.RLock()
	time.Sleep(filterSleepTime)
	filterSleepTimeLock.RUnlock()
	return caller.Call(request)
}

func (m *mockEndPointFilter) HasNext() bool {
	return false
}

func (m *mockEndPointFilter) SetNext(nextFilter core.EndPointFilter) {
	vlog.Errorf("should not set next in mockEndPointFilter! filer:%s\n", nextFilter.GetName())
}
func (m *mockEndPointFilter) GetNext() core.EndPointFilter {
	return nil
}
func (m *mockEndPointFilter) GetIndex() int {
	return 100
}
func (m *mockEndPointFilter) GetType() int32 {
	return core.EndPointFilterType
}
