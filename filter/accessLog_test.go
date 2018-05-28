package filter

import (
	"fmt"
	"testing"

	motan "github.com/weibocom/motan-go/core"
	endpoint "github.com/weibocom/motan-go/endpoint"
)

func TestFilter(t *testing.T) {
	defaultExtFactory := &motan.DefaultExtentionFactory{}
	defaultExtFactory.Initialize()

	RegistDefaultFilters(defaultExtFactory)
	endpoint.RegistDefaultEndpoint(defaultExtFactory)
	f := defaultExtFactory.GetFilter("accessLog")
	if f == nil {
		t.Fatal("can not find accesslog filter!")
	}
	url := &motan.URL{Host: "127.0.0.1", Port: 7888, Protocol: "mockEndpoint"}
	f = f.NewFilter(url)
	ef := f.(motan.EndPointFilter)
	ef.SetNext(motan.GetLastEndPointFilter())
	caller := defaultExtFactory.GetEndPoint(url)
	arguments := []interface{}{url, "xxx", 123, true}
	attachments := motan.NewConcurrentStringMap()
	attachments.Store("key1", "value1")
	attachments.Store("key2", "value2")
	request := &motan.MotanRequest{RequestID: 11234, ServiceName: "com.weibo.TestService", Method: "testMethod", Arguments: arguments, Attachment: attachments}
	res := ef.Filter(caller, request)
	fmt.Printf("res:%+v", res)
}
