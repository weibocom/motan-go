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

var defaultCap = 1000

func TestRateLimitFilter(t *testing.T) {
	request := &core.MotanRequest{Method: "testMethod"}
	testItems := []testItem{
		{
			title: "plain service rateLimit",
			param: map[string]string{"rateLimit": strconv.Itoa(rate), "conf-id": "zha"},
		},
		{
			title: "plain method rateLimit",
			param: map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "zha"},
		},
		{
			title: "rateLimit switcher off",
			param: map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "zha"},
			preCall: func() {
				core.GetSwitcherManager().GetSwitcher("zha_rateLimit").SetValue(false)
			},
			afterCall: func() {
				core.GetSwitcherManager().GetSwitcher("zha_rateLimit").SetValue(true)
			},
		},
	}
	for i, j := range testItems {
		if j.preCall != nil {
			j.preCall()
		}
		caller, ef := getEf(j.param)
		if ef == nil {
			t.Error("Can not find rateLimit filter!")
			return
		}
		startTime := time.Now()
		for i := 0; i < defaultCap+offset; i++ {
			ef.Filter(caller, request)
		}
		elapsed := time.Since(startTime)
		if i == 2 {
			if elapsed > time.Duration(offset-1)*time.Second/time.Duration(rate) {
				t.Error("Test ", j.title, " failed! elapsed:", elapsed)
			}
		} else {
			if elapsed < time.Duration(offset-1)*time.Second/time.Duration(rate) {
				t.Error("Test ", j.title, " failed! elapsed:", elapsed)
			}
		}
		if j.afterCall != nil {
			j.afterCall()
		}
	}
}

func TestWrongParam(t *testing.T) {
	assert := assert2.New(t)
	testItems := []testItem{
		{
			title: "wrong type of service rate value",
			param: map[string]string{"rateLimit": "jia", "conf-id": "zha"},
		},
		{
			title: "wrong type of method rate value",
			param: map[string]string{"rateLimit.testMethod": "jia", "conf-id": "jia"},
		},
		{
			title: "wrong method rate key",
			param: map[string]string{"rateLimit.": "10", "conf-id": "jia"},
		},
		{
			title: "wrong type of method timeout value",
			param: map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "testMethod().requestTimeout": "jia"},
		},
		{
			title: "negative value of method timeout",
			param: map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "testMethod().requestTimeout": "-1"},
		},
		{
			title: "wrong service rate value and wrong type of method timeout",
			param: map[string]string{"rateLimit.testMethod": "-1", "conf-id": "Jia", "testMethod().requestTimeout": "jia"},
		},
		{
			title: "negative service rate value",
			param: map[string]string{"rateLimit": "-1", "conf-id": "jia"},
		},
	}
	request := &core.MotanRequest{Method: "testMethod"}
	for _, j := range testItems {
		caller, ef := getEf(j.param)
		for i := 0; i < defaultCap+offset; i++ {
			resp := ef.Filter(caller, request)
			assert.Nil(resp.GetException())
		}
	}
}

func TestRateLimitTimeout(t *testing.T) {
	assert := assert2.New(t)
	request := &core.MotanRequest{Method: "testMethod"}

	testItems := []testItem{
		{
			title:  "plain service max duration(timeout)",
			param:  map[string]string{"rateLimit": strconv.Itoa(rate), "conf-id": "Jia", "requestTimeout": "50"},
			errMsg: "[rateLimit] wait time exceed timeout(50ms)",
		},
		{
			title:  "plain method max duration(timeout)",
			param:  map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "testMethod().requestTimeout": "50"},
			errMsg: "[rateLimit] method testMethod wait time exceed timeout(50ms)",
		},
	}
	for _, j := range testItems {
		caller, ef := getEf(j.param)
		for i := 0; i < defaultCap+offset; i++ {
			resp := ef.Filter(caller, request)
			if i >= defaultCap {
				assert.NotNil(resp.GetException())
				assert.Equal(resp.GetException().ErrMsg, j.errMsg)
			}
		}
	}

	//Test switcher
	param := map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "testMethod().requestTimeout": "50"}
	caller, ef := getEf(param)
	core.GetSwitcherManager().GetSwitcher("Jia_rateLimit").SetValue(false)
	for i := 0; i < defaultCap+offset; i++ {
		resp := ef.Filter(caller, request)
		assert.Nil(resp.GetException())
	}
	core.GetSwitcherManager().GetSwitcher("Jia_rateLimit").SetValue(true)
}

func TestCapacity(t *testing.T) {
	capacity := 200
	request := &core.MotanRequest{Method: "testMethod"}
	testItems := []testItem{
		{
			title: "plain service capacity",
			param: map[string]string{"rateLimit": strconv.Itoa(rate), "conf-id": "Jia", "capacity": strconv.Itoa(capacity)},
		},
		{
			title: "plain method capacity",
			param: map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "testMethod().capacity": strconv.Itoa(capacity)},
		},
		{
			title: "wrong type of service capacity value",
			param: map[string]string{"rateLimit": strconv.Itoa(rate), "conf-id": "Jia", "capacity": "jia"},
		},
		{
			title: "wrong type of method capacity value",
			param: map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "testMethod().capacity": "jia"},
		},
		{
			title: "wrong value of method capacity",
			param: map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "testMethod().capacity": "-1"},
		},
	}
	for k, j := range testItems {
		caller, ef := getEf(j.param)
		startTime := time.Now()
		for i := 0; i < capacity+offset; i++ {
			ef.Filter(caller, request)
		}
		if k >= 2 {
			if elapsed := time.Since(startTime); elapsed > time.Duration(offset-1)*time.Second/time.Duration(rate) {
				t.Error("Test ", j.title, " failed! elapsed:", elapsed.String(), " ", time.Duration(offset-1)*time.Second/time.Duration(rate))
			}
		} else {
			if elapsed := time.Since(startTime); elapsed < time.Duration(offset-1)*time.Second/time.Duration(rate) {
				t.Error("Test ", j.title, " failed! elapsed:", elapsed.String(), " ", time.Duration(offset-1)*time.Second/time.Duration(rate))
			}
		}
	}
	param := map[string]string{"rateLimit.testMethod": strconv.Itoa(rate), "conf-id": "Jia", "capacity.testMethod": "5"}
	caller, ef := getEf(param)
	// rateLimit will use default capacity
	for i := 0; i < defaultCap+offset; i++ {
		resp := ef.Filter(caller, request)
		assert2.Nil(t, resp.GetException())
	}
}

func TestCapacityEdge(t *testing.T) {
	request := &core.MotanRequest{Method: "testMethod"}
	bigRate := 2501
	properRate := 600
	properCapacity := properRate * 2
	testItems := []testItem{
		{
			title: "max service capacity",
			param: map[string]string{"rateLimit": strconv.Itoa(bigRate), "conf-id": "Jia"},
		},
		{
			title: "max method capacity",
			param: map[string]string{"rateLimit.testMethod": strconv.Itoa(bigRate), "conf-id": "Jia"},
		},
		{
			title: "proper service capacity",
			param: map[string]string{"rateLimit": strconv.Itoa(properRate), "conf-id": "Jia"},
		},
		{
			title: "proper method capacity",
			param: map[string]string{"rateLimit.testMethod": strconv.Itoa(properRate), "conf-id": "Jia"},
		},
	}
	for k, j := range testItems {
		caller, ef := getEf(j.param)
		startTime := time.Now()
		if k >= 2 {
			for i := 0; i < properCapacity+offset; i++ {
				ef.Filter(caller, request)
			}
			if elapsed := time.Since(startTime); elapsed < time.Duration(offset-1)*time.Second/time.Duration(properRate) {
				t.Error("Test ", j.title, " failed! elapsed:", elapsed.String(), " ", time.Duration(offset-1)*time.Second/time.Duration(rate))
			}
		} else {
			for i := 0; i < maxCapacity+offset; i++ {
				ef.Filter(caller, request)
			}
			if elapsed := time.Since(startTime); elapsed < time.Duration(offset-1)*time.Second/time.Duration(bigRate) {
				t.Error("Test ", j.title, " failed! elapsed:", elapsed.String(), " ", time.Duration(offset-1)*time.Second/time.Duration(rate))
			}
		}

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

type testItem struct {
	title     string
	param     map[string]string
	preCall   func()
	afterCall func()
	errMsg    string
}
