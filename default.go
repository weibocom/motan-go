package motan

import (
	"sync"

	motan "github.com/weibocom/motan-go/core"
	endpoint "github.com/weibocom/motan-go/endpoint"
	filter "github.com/weibocom/motan-go/filter"
	ha "github.com/weibocom/motan-go/ha"
	lb "github.com/weibocom/motan-go/lb"
	provider "github.com/weibocom/motan-go/provider"
	registry "github.com/weibocom/motan-go/registry"
	serialize "github.com/weibocom/motan-go/serialize"
	server "github.com/weibocom/motan-go/server"
)

var (
	once              sync.Once
	defaultExtFactory *motan.DefaultExtentionFactory
)

func GetDefaultExtFactory() motan.ExtentionFactory {
	once.Do(func() {
		defaultExtFactory = &motan.DefaultExtentionFactory{}
		defaultExtFactory.Initialize()
		AddDefaultExt(defaultExtFactory)
	})
	return defaultExtFactory
}

func AddDefaultExt(d motan.ExtentionFactory) {

	// all default extension
	filter.RegistDefaultFilters(d)
	ha.RegistDefaultHa(d)
	lb.RegistDefaultLb(d)
	endpoint.RegistDefaultEndpoint(d)
	provider.RegistDefaultProvider(d)
	registry.RegistDefaultRegistry(d)
	server.RegistDefaultServers(d)
	server.RegistDefaultMessageHandlers(d)
	serialize.RegistDefaultSerializations(d)
}
