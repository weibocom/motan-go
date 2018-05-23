package provider

import (
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
	"github.com/weibocom/motan-go/serialize"
	"testing"
)

func TestGetName(t *testing.T) {
	mProvider := MotanProvider{gctx: &motan.Context{}}
	mProvider.gctx.ConfigFile = "test.yaml"
	mProvider.gctx.Initialize()
	mProvider.url = &motan.URL{Host: "127.0.0.1", Port: 8104, Protocol: "motan2", Parameters: map[string]string{"conf-id": "reverseProxyService", "serialization": "simple"}}
	factory := &motan.DefaultExtentionFactory{}
	factory.Initialize()
	serialize.RegistDefaultSerializations(factory)
	endpoint.RegistDefaultEndpoint(factory)
	mProvider.extFactory = factory
	mProvider.Initialize()
	request := &motan.MotanRequest{}
	res := mProvider.Call(request)
	if res.GetValue().(string) != "ok" {
		t.Errorf("Incorrect response! response:%+v", res.GetValue())
	}
}
