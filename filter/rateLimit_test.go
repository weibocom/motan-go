package filter

import (
	assert2 "github.com/stretchr/testify/assert"
	"strconv"
	"testing"
	"time"

	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
)

var offset = 2
var rate = 10 // times per second

func TestRateLimitFilter(t *testing.T) {
	assert := assert2.New(t)
	//Init
	defaultExtFactory := &core.DefaultExtensionFactory{}
	defaultExtFactory.Initialize()
	RegistDefaultFilters(defaultExtFactory)
	endpoint.RegistDefaultEndpoint(defaultExtFactory)
	url := &core.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint"}
	caller := defaultExtFactory.GetEndPoint(url)
	request := &core.MotanRequest{Method: "testMethod"}

	//Test NewFilter
	param := map[string]string{"rateLimit": strconv.Itoa(rate), "conf-id": "zha"}
	filterURL := &core.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint", Parameters: param}
	f := defaultExtFactory.GetFilter("rateLimit")
	if f == nil {
		t.Error("Can not find rateLimit filter!")
	}
	if f != nil {
		f = f.NewFilter(filterURL)
	}
	ef := f.(core.EndPointFilter)
	ef.SetNext(core.GetLastEndPointFilter())

	//Test serviceFilter
	startTime := time.Now()
	for i := 0; i < int(defaultCapacity)+offset; i++ {
		ef.Filter(caller, request)
	}
	if elapsed := time.Since(startTime); elapsed < time.Duration(offset-1)*time.Second/time.Duration(rate) {
		t.Error("Test serviceFilter failed! elapsed:", elapsed)
	}

	//Test wrong param
	param = map[string]string{"rateLimit": "jia", "conf-id": "zha"}
	caller, ef = getEf(param)
	for i := 0; i < int(defaultCapacity)+offset; i++ {
		resp := ef.Filter(caller, request)
		assert.Nil(resp.GetException())
	}
	//Test methodFilter
	param = map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "zha"}
	filterURL = &core.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint", Parameters: param}
	ef = defaultExtFactory.GetFilter("rateLimit").NewFilter(filterURL).(core.EndPointFilter)
	ef.SetNext(core.GetLastEndPointFilter())
	startTime = time.Now()
	for i := 0; i < int(defaultCapacity)+offset; i++ {
		ef.Filter(caller, request)
	}
	if elapsed := time.Since(startTime); elapsed < time.Duration(offset-1)*time.Second/time.Duration(rate) {
		t.Error("Test methodFilter failed! elapsed:", elapsed)
	}

	//Test wrong input
	param = map[string]string{"rateLimit.testMethod": "jia", "conf-id": "jia", "timeout.testMethod": "50"}
	caller, ef = getEf(param)
	for i := 0; i < int(defaultCapacity)+offset; i++ {
		resp := ef.Filter(caller, request)
		assert.Nil(resp.GetException())
	}
	param = map[string]string{"rateLimit.": "10", "conf-id": "jia", "timeout.testMethod": "50"}
	caller, ef = getEf(param)
	for i := 0; i < int(defaultCapacity)+offset; i++ {
		resp := ef.Filter(caller, request)
		assert.Nil(resp.GetException())
	}
	//Test switcher
	param = map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "zha"}
	filterURL = &core.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint", Parameters: param}
	ef = defaultExtFactory.GetFilter("rateLimit").NewFilter(filterURL).(core.EndPointFilter)
	ef.SetNext(core.GetLastEndPointFilter())
	core.GetSwitcherManager().GetSwitcher("zha_rateLimit").SetValue(false)
	startTime = time.Now()
	for i := 0; i < int(defaultCapacity)+offset; i++ {
		ef.Filter(caller, request)
	}
	if elapsed := time.Since(startTime); elapsed > time.Duration(offset+1)*time.Second/time.Duration(rate) {
		t.Error("Test switcher failed! elapsed:", elapsed)
	}
	core.GetSwitcherManager().GetSwitcher("zha_rateLimit").SetValue(true)
}

func TestRateLimitTimeout(t *testing.T) {
	assert := assert2.New(t)
	request := &core.MotanRequest{Method: "testMethod"}
	//Test NewFilter
	param := map[string]string{"rateLimit": strconv.Itoa(rate), "conf-id": "Jia", "timeout": "50"}
	caller, ef := getEf(param)
	//Test serviceFilter
	for i := 0; i < int(defaultCapacity)+offset; i++ {
		resp := ef.Filter(caller, request)
		if i >= int(defaultCapacity) {
			assert.NotNil(resp.GetException())
			assert.Equal(resp.GetException().ErrMsg, "[rateLimit] wait time exceed timeout(50ms)")
		}
	}
	//Test methodFilter
	param = map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "timeout.testMethod": "50"}
	caller, ef = getEf(param)
	for i := 0; i < int(defaultCapacity)+offset; i++ {
		resp := ef.Filter(caller, request)
		if i >= int(defaultCapacity) {
			assert.NotNil(resp.GetException())
			assert.Equal(resp.GetException().ErrMsg, "[rateLimit] method testMethod wait time exceed timeout(50ms)")
		}
	}

	//Test wrong param
	param = map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "timeout.": "50"}
	caller, ef = getEf(param)
	for i := 0; i < int(defaultCapacity)+offset; i++ {
		resp := ef.Filter(caller, request)
		assert.Nil(resp.GetException())
	}

	param = map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "timeout.testMethod": "jia"}
	caller, ef = getEf(param)
	for i := 0; i < int(defaultCapacity)+offset; i++ {
		resp := ef.Filter(caller, request)
		assert.Nil(resp.GetException())
	}

	//Test switcher
	param = map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "timeout.testMethod": "50"}
	caller, ef = getEf(param)
	core.GetSwitcherManager().GetSwitcher("Jia_rateLimit").SetValue(false)
	for i := 0; i < int(defaultCapacity)+offset; i++ {
		resp := ef.Filter(caller, request)
		assert.Nil(resp.GetException())
	}
	core.GetSwitcherManager().GetSwitcher("Jia_rateLimit").SetValue(true)
}

func TestCapacity(t *testing.T) {
	//Init
	capacity := 200
	request := &core.MotanRequest{Method: "testMethod"}
	param := map[string]string{"rateLimit": strconv.Itoa(rate), "conf-id": "Jia", "capacity": strconv.Itoa(capacity)}
	caller, ef := getEf(param)
	//Test serviceFilter
	startTime := time.Now()
	for i := 0; i < capacity+offset; i++ {
		ef.Filter(caller, request)
	}
	if elapsed := time.Since(startTime); elapsed < time.Duration(offset-1)*time.Second/time.Duration(rate) {
		t.Error("Test serviceFilter failed! elapsed:", elapsed.String(), " ", time.Duration(offset-1)*time.Second/time.Duration(rate))
	}

	//Test wrong params
	param = map[string]string{"rateLimit": strconv.Itoa(rate), "conf-id": "Jia", "capacity": "jia"}
	caller, ef = getEf(param)
	startTime = time.Now()
	for i := 0; i < int(defaultCapacity)+offset; i++ {
		ef.Filter(caller, request)
	}
	if elapsed := time.Since(startTime); elapsed < time.Duration(offset-1)*time.Second/time.Duration(rate) {
		t.Error("Test methodFilter failed! elapsed:", elapsed)
	}

	//Test methodFilter
	param = map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "capacity.testMethod": strconv.Itoa(capacity)}
	caller, ef = getEf(param)
	startTime = time.Now()
	for i := 0; i < capacity+offset; i++ {
		ef.Filter(caller, request)
	}
	if elapsed := time.Since(startTime); elapsed < time.Duration(offset-1)*time.Second/time.Duration(rate) {
		t.Error("Test methodFilter failed! elapsed:", elapsed)
	}

	//Test wrong params
	param = map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "capacity.testMethod": "jia"}
	caller, ef = getEf(param)
	startTime = time.Now()
	// rateLimit will use default capacity
	for i := 0; i < int(defaultCapacity)+offset; i++ {
		ef.Filter(caller, request)
	}
	if elapsed := time.Since(startTime); elapsed < time.Duration(offset-1)*time.Second/time.Duration(rate) {
		t.Error("Test methodFilter failed! elapsed:", elapsed)
	}

	//Test wrong input
	param = map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "capacity.": "1000"}
	caller, ef = getEf(param)
	startTime = time.Now()
	// rateLimit will use default capacity
	for i := 0; i < int(defaultCapacity)+offset; i++ {
		ef.Filter(caller, request)
	}
	if elapsed := time.Since(startTime); elapsed < time.Duration(offset-1)*time.Second/time.Duration(rate) {
		t.Error("Test methodFilter failed! elapsed:", elapsed)
	}
}

func TestOther(t *testing.T) {
	assert := assert2.New(t)
	defaultExtFactory := &core.DefaultExtensionFactory{}
	defaultExtFactory.Initialize()
	RegistDefaultFilters(defaultExtFactory)
	endpoint.RegistDefaultEndpoint(defaultExtFactory)
	f := defaultExtFactory.GetFilter("rateLimit")
	assert.Equal(f.GetIndex(), 3)
	assert.Equal(f.GetName(), RateLimit)
	assert.Equal(f.HasNext(), false)
	assert.Equal(int(f.GetType()), core.EndPointFilterType)
}

func getEf(param map[string]string) (core.EndPoint, core.EndPointFilter) {
	defaultExtFactory := &core.DefaultExtensionFactory{}
	defaultExtFactory.Initialize()
	RegistDefaultFilters(defaultExtFactory)
	endpoint.RegistDefaultEndpoint(defaultExtFactory)
	url := &core.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint"}
	caller := defaultExtFactory.GetEndPoint(url)
	filterURL := &core.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint", Parameters: param}
	f := defaultExtFactory.GetFilter("rateLimit")
	f = f.NewFilter(filterURL)
	ef := f.(core.EndPointFilter)
	ef.SetNext(core.GetLastEndPointFilter())
	return caller, ef
}
