package filter

import (
	"testing"
	"time"

	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
)

func TestRateLimitFilter(t *testing.T) {
	//Init
	defaultExtFactory := &core.DefaultExtensionFactory{}
	defaultExtFactory.Initialize()
	RegistDefaultFilters(defaultExtFactory)
	endpoint.RegistDefaultEndpoint(defaultExtFactory)
	url := &core.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint"}
	caller := defaultExtFactory.GetEndPoint(url)
	request := &core.MotanRequest{Method: "testMethod"}

	//Test NewFilter
	param := map[string]string{"rateLimit": "1", "conf-id": "zha"}
	filterURL := &core.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint", Parameters: param}
	f := defaultExtFactory.GetFilter("rateLimit")
	if f == nil {
		t.Error("Can not find rateLimit filter!")
	}
	f = f.NewFilter(filterURL)
	ef := f.(core.EndPointFilter)
	ef.SetNext(core.GetLastEndPointFilter())

	//Test serviceFilter
	startTime := time.Now()
	for i := 0; i < 1001; i++ {
		ef.Filter(caller, request)
	}
	if elapsed1 := time.Since(startTime); elapsed1 < time.Second {
		t.Error("Test serviceFilter failed! elapsed1:", elapsed1)
	}

	//Test methodFilter
	param = map[string]string{"rateLimit.testMethod": "1", "conf-id": "zha"}
	filterURL = &core.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint", Parameters: param}
	ef = defaultExtFactory.GetFilter("rateLimit").NewFilter(filterURL).(core.EndPointFilter)
	ef.SetNext(core.GetLastEndPointFilter())
	startTime = time.Now()
	for i := 0; i < 1001; i++ {
		ef.Filter(caller, request)
	}
	if elapsed1 := time.Since(startTime); elapsed1 < time.Second {
		t.Error("Test methodFilter failed! elapsed1:", elapsed1)
	}

	//Test switcher
	param = map[string]string{"rateLimit.testMethod": "1", "conf-id": "zha"}
	filterURL = &core.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint", Parameters: param}
	ef = defaultExtFactory.GetFilter("rateLimit").NewFilter(filterURL).(core.EndPointFilter)
	ef.SetNext(core.GetLastEndPointFilter())
	core.GetSwitcherManager().GetSwitcher("zha_rateLimit").SetValue(false)
	startTime = time.Now()
	for i := 0; i < 1001; i++ {
		ef.Filter(caller, request)
	}
	if elapsed1 := time.Since(startTime); elapsed1 > time.Second {
		t.Error("Test switcher failed! elapsed:", elapsed1)
	}
}
