package filter

import (
	"fmt"
	"testing"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
	"github.com/weibocom/motan-go/provider"
)

var (
	testService = "com.weibo.testService"
	testGroup   = "testGroup"
	testMethod  = "testMethod"
)

func TestFilter(t *testing.T) {
	factory := initFactory()
	f := factory.GetFilter(AccessLog)
	if f == nil {
		t.Fatal("can not find accessLog filter!")
	}
	url := mockURL()
	f = f.NewFilter(url)
	ef := f.(motan.EndPointFilter)
	ef.SetNext(motan.GetLastEndPointFilter())
	caller := factory.GetEndPoint(url)
	request := defaultRequest()
	res := ef.Filter(caller, request)
	fmt.Printf("res:%+v", res)
}

func initFactory() motan.ExtensionFactory {
	defaultExtFactory := &motan.DefaultExtensionFactory{}
	defaultExtFactory.Initialize()

	RegistDefaultFilters(defaultExtFactory)
	endpoint.RegistDefaultEndpoint(defaultExtFactory)
	provider.RegistDefaultProvider(defaultExtFactory)
	return defaultExtFactory
}

func mockURL() *motan.URL {
	url := &motan.URL{Host: "127.0.0.1", Port: 7888, Protocol: endpoint.Mock}
	url.PutParam(motan.ProviderKey, provider.Mock)
	return url
}

func defaultRequest() *motan.MotanRequest {
	return getRequest(testService, testGroup, testMethod)
}
func getRequest(service string, group string, method string) *motan.MotanRequest {
	r := &motan.MotanRequest{ServiceName: service, Method: method}
	r.SetAttachment("M_g", group)
	return r
}
