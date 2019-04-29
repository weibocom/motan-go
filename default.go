package motan

import (
	"net/http"
	"sync"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
	"github.com/weibocom/motan-go/filter"
	"github.com/weibocom/motan-go/ha"
	"github.com/weibocom/motan-go/lb"
	"github.com/weibocom/motan-go/provider"
	"github.com/weibocom/motan-go/registry"
	"github.com/weibocom/motan-go/serialize"
	"github.com/weibocom/motan-go/server"
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
		defaultManageHandlers["/status"] = status

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
		defaultManageHandlers["/debug/stat/system"] = debug
		defaultManageHandlers["/debug/stat/process"] = debug

		switcher := &SwitcherHandler{}
		defaultManageHandlers["/switcher/set"] = switcher
		defaultManageHandlers["/switcher/get"] = switcher
		defaultManageHandlers["/switcher/getAll"] = switcher

		dynamicConfigurer := &DynamicConfigurerHandler{}
		defaultManageHandlers["/registry/register"] = dynamicConfigurer
		defaultManageHandlers["/registry/unregister"] = dynamicConfigurer
		defaultManageHandlers["/registry/subscribe"] = dynamicConfigurer
		defaultManageHandlers["/registry/list"] = dynamicConfigurer
		defaultManageHandlers["/registry/info"] = dynamicConfigurer
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
