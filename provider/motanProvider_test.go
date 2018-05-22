package provider

import (
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/serialize"
	"testing"
)

func TestGetName(t *testing.T) {
	mProvider := MotanProvider{gctx: &motan.Context{}}
	mProvider.gctx.ConfigFile = "../main/agentdemo.yaml"
	mProvider.gctx.Initialize() //cfg.Config's field is private.
	mProvider.url = &motan.URL{Host: "127.0.0.1", Port: 8105, Protocol: "motan2", Parameters: map[string]string{"conf-id": "reverseProxyService", "serialization": "simple"}}
	factory := &motan.DefaultExtentionFactory{}
	factory.Initialize()
	serialize.RegistDefaultSerializations(factory)
	mProvider.extFactory = factory
	mProvider.Initialize()
	argsStr := map[string]string{"num1": "12345", "num2": "54321"}
	request := &motan.MotanRequest{ServiceName: "com.weibo.motan.demo.service.ReverseProxyService", Method: "MotanConnStr", Arguments: []interface{}{argsStr}}
	request.Attachment = make(map[string]string, 0)
	res := mProvider.Call(request)
	if res.GetValue().(string) != "1234554321" {
		t.Errorf("Incorrect response! response:%+v", res.GetValue())
	}
}
