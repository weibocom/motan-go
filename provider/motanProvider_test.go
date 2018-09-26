package provider

import (
	"testing"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
)

const (
	serviceName  = "reverseProxyService"
	confFilePath = "test.yaml"
)

func TestGetName(t *testing.T) {
	//init factory
	factory := &motan.DefaultExtensionFactory{}
	factory.Initialize()
	endpoint.RegistDefaultEndpoint(factory)
	RegistDefaultProvider(factory)

	//init motanProvider
	mContext := motan.Context{}
	mContext.ConfigFile = confFilePath
	mContext.Initialize()
	request := &motan.MotanRequest{}
	url := mContext.ServiceURLs[serviceName]

	//call correct
	providerCorr := MotanProvider{url: url, extFactory: factory}
	providerCorr.Initialize()
	responseCorr := providerCorr.Call(request)
	if responseCorr.GetValue() == nil || responseCorr.GetValue().(string) != "ok" {
		t.Errorf("Incorrect correct response! response:%+v", responseCorr)
	}

	//call error
	url.Parameters[motan.ProxyKey] = "error"
	providerErr := MotanProvider{url: url, extFactory: factory}
	providerErr.Initialize()
	responseErr := providerErr.Call(request)
	if responseErr.GetException().ErrMsg != "reverse proxy call err: motanProvider is unavailable" {
		t.Errorf("Incorrect error response! response:%+v", responseErr)
	}

}
