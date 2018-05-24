package provider

import (
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
	"github.com/weibocom/motan-go/serialize"
	"testing"
)

const (
	serviceName  = "reverseProxyService"
	confFilePath = "test.yaml"
)

func TestGetName(t *testing.T) {
	//init context
	mContext := motan.Context{}
	mContext.ConfigFile = confFilePath
	mContext.Initialize()

	//init factory
	factory := &motan.DefaultExtentionFactory{}
	factory.Initialize()
	serialize.RegistDefaultSerializations(factory)
	endpoint.RegistDefaultEndpoint(factory)
	RegistDefaultProvider(factory)

	//init & call provider
	pUrl := mContext.ServiceURLs[serviceName]
	mProvider := factory.GetProvider(pUrl)
	motan.Initialize(mProvider)
	request := &motan.MotanRequest{}
	res := mProvider.Call(request)
	if res.GetValue() == nil || res.GetValue().(string) != "ok" {
		if res.GetException().ErrMsg != "reverse proxy call err: motanProvider is unavailable" {
			t.Errorf("Incorrect response! response:%+v", res)
		}
	}
}
