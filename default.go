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
	"net/http"
)

var (
	extOnce           sync.Once
	handlerOnce       sync.Once
	defaultExtFactory *motan.DefaultExtensionFactory
	// all default manage handlers
	defaultManageHandlers map[string]http.Handler
	// PermissionCheck is default permission check for manage request
	PermissionCheck = NoPermissionCheck
)

type PermissionCheckFunc func(r *http.Request) bool

func NoPermissionCheck(r *http.Request) bool {
	return true
}

func GetDefaultManageHandlers() map[string]http.Handler {
	handlerOnce.Do(func() {
		defaultManageHandlers = make(map[string]http.Handler, 16)
		status := &StatusHandler{}
		defaultManageHandlers["/"] = status
		defaultManageHandlers["/200"] = status
		defaultManageHandlers["/503"] = status
		defaultManageHandlers["/version"] = status
		info := &InfoHandler{}
		defaultManageHandlers["/getConfig"] = info
		defaultManageHandlers["/getReferService"] = info
		debug := &DebugHandler{}
		defaultManageHandlers["/debug/pprof/"] = debug
		defaultManageHandlers["/debug/pprof/cmdline"] = debug
		defaultManageHandlers["/debug/pprof/profile"] = debug
		defaultManageHandlers["/debug/pprof/symbol"] = debug
		defaultManageHandlers["/debug/pprof/trace"] = debug
		defaultManageHandlers["/debug/mesh/trace"] = debug
		defaultManageHandlers["/debug/pprof/sw"] = debug
	})
	return defaultManageHandlers
}

func GetDefaultExtFactory() motan.ExtensionFactory {
	extOnce.Do(func() {
		defaultExtFactory = &motan.DefaultExtensionFactory{}
		defaultExtFactory.Initialize()
		AddDefaultExt(defaultExtFactory)
	})
	return defaultExtFactory
}

func AddDefaultExt(d motan.ExtensionFactory) {

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
