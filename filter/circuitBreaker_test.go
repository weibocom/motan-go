package filter

import (
	"github.com/afex/hystrix-go/hystrix"
	assert2 "github.com/stretchr/testify/assert"
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
	param := map[string]string{core.TimeOutKey: "2", SleepWindowField: "300", IncludeBizException: "jia"}
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
	time.Sleep(100 * time.Millisecond) //wait until async call complete
	countLock.RLock()
	if count != 171 && count != 172 {
		t.Error("Test sleepWindow failed! count:", count)
	}
	countLock.RUnlock()
}

func TestGetConfigStr(t *testing.T) {
	assert := assert2.New(t)
	conf := &hystrix.CommandConfig{
		RequestVolumeThreshold: 10,
		SleepWindow:            300,
		MaxConcurrentRequests:  500,
		ErrorPercentThreshold:  50,
	}
	res := getConfigStr(conf)
	assert.Equal(res, "requestThreshold:10 sleepWindow:300 errorPercent:50 maxConcurrent:500 ")
	conf = &hystrix.CommandConfig{}
	res = getConfigStr(conf)
	assert.Equal(res, "requestThreshold:20 sleepWindow:5000 errorPercent:50 maxConcurrent:5000 ")
}

func TestBuildConfig(t *testing.T) {
	assert := assert2.New(t)
	valid := core.URL{
		Parameters: map[string]string{
			RequestVolumeThresholdField: "10",
			core.TimeOutKey:             "10",
			SleepWindowField:            "10",
			MaxConcurrentField:          "10",
			ErrorPercentThreshold:       "10",
		},
	}
	conf := buildCommandConfig("test", &valid)
	assert.Equal(conf.ErrorPercentThreshold, 10)
	assert.Equal(conf.Timeout, 20)
	assert.Equal(conf.MaxConcurrentRequests, 10)
	assert.Equal(conf.SleepWindow, 10)
	assert.Equal(conf.RequestVolumeThreshold, 10)
	invalid := core.URL{
		Parameters: map[string]string{
			RequestVolumeThresholdField: "-1",
			core.TimeOutKey:             "-1",
			SleepWindowField:            "-1",
			MaxConcurrentField:          "-1",
			ErrorPercentThreshold:       "200",
		},
	}
	conf = buildCommandConfig("test", &invalid)
	assert.Equal(conf.ErrorPercentThreshold, 50)
	assert.Equal(conf.Timeout, 2000)
	assert.Equal(conf.MaxConcurrentRequests, defaultMaxConcurrent)
	assert.Equal(conf.SleepWindow, 5000)
	assert.Equal(conf.RequestVolumeThreshold, 20)
	empty := core.URL{
		Parameters: map[string]string{
			RequestVolumeThresholdField: "-1",
			core.TimeOutKey:             "-1",
			SleepWindowField:            "-1",
			MaxConcurrentField:          "-1",
			ErrorPercentThreshold:       "200",
		},
	}
	conf = buildCommandConfig("test", &empty)
	assert.Equal(conf.ErrorPercentThreshold, 50)
	assert.Equal(conf.Timeout, 2000)
	assert.Equal(conf.MaxConcurrentRequests, defaultMaxConcurrent)
	assert.Equal(conf.SleepWindow, 5000)
	assert.Equal(conf.RequestVolumeThreshold, 20)
}

func TestOtherCB(t *testing.T) {
	assert := assert2.New(t)
	defaultExtFactory := &core.DefaultExtensionFactory{}
	defaultExtFactory.Initialize()
	RegistDefaultFilters(defaultExtFactory)
	endpoint.RegistDefaultEndpoint(defaultExtFactory)
	f := defaultExtFactory.GetFilter(CircuitBreaker)
	assert.Equal(f.GetIndex(), 20)
	assert.Equal(f.HasNext(), false)
	assert.Equal(f.GetName(), CircuitBreaker)
	assert.Equal(int(f.GetType()), core.EndPointFilterType)
}

func TestMockException(t *testing.T) {
	assert := assert2.New(t)
	defaultExtFactory := &core.DefaultExtensionFactory{}
	defaultExtFactory.Initialize()
	RegistDefaultFilters(defaultExtFactory)
	endpoint.RegistDefaultEndpoint(defaultExtFactory)
	url := &core.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint"}
	caller := defaultExtFactory.GetEndPoint(url)
	request := &core.MotanRequest{Method: "testMethod"}

	//Test NewFilter
	param := map[string]string{core.TimeOutKey: "2", SleepWindowField: "300", IncludeBizException: "jia"}
	filterURL := &core.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint", Parameters: param}
	f := defaultExtFactory.GetFilter(CircuitBreaker)
	if f == nil {
		t.Error("Can not find circuitBreaker filter!")
	}
	f = f.NewFilter(filterURL)
	ef := f.(core.EndPointFilter)
	ef.SetNext(new(mockExceptionEPFilter))
	res := ef.Filter(caller, request)
	assert.NotNil(res.GetException())
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
	vlog.Errorf("should not set next in mockEndPointFilter! filer:%s", nextFilter.GetName())
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

type mockExceptionEPFilter struct{}

func (m *mockExceptionEPFilter) GetName() string {
	return "mockEndPointFilter"
}

func (m *mockExceptionEPFilter) NewFilter(url *core.URL) core.Filter {
	return core.GetLastEndPointFilter()
}

func (m *mockExceptionEPFilter) Filter(caller core.Caller, request core.Request) core.Response {
	return defaultErrMotanResponse(request, "mock exception")
}

func (m *mockExceptionEPFilter) HasNext() bool {
	return false
}

func (m *mockExceptionEPFilter) SetNext(nextFilter core.EndPointFilter) {
	vlog.Errorf("should not set next in mockEndPointFilter! filer:%s", nextFilter.GetName())
}
func (m *mockExceptionEPFilter) GetNext() core.EndPointFilter {
	return nil
}
func (m *mockExceptionEPFilter) GetIndex() int {
	return 100
}
func (m *mockExceptionEPFilter) GetType() int32 {
	return core.EndPointFilterType
}
