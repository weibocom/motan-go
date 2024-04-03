package server

import (
	"errors"
	"fmt"
	"github.com/weibocom/motan-go/meta"
	mpro "github.com/weibocom/motan-go/protocol"
	"github.com/weibocom/motan-go/provider"
	"sync"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	Motan2 = "motan2"
	Motan  = "motan"
	CGI    = "cgi"
)

const (
	Default = "default"
)

func RegistDefaultServers(extFactory motan.ExtensionFactory) {
	extFactory.RegistExtServer(Motan2, func(url *motan.URL) motan.Server {
		return &MotanServer{URL: url}
	})
	extFactory.RegistExtServer(Motan, func(url *motan.URL) motan.Server {
		return &MotanServer{URL: url}
	})
	extFactory.RegistExtServer(CGI, func(url *motan.URL) motan.Server {
		return &MotanServer{URL: url}
	})
}

func RegistDefaultMessageHandlers(extFactory motan.ExtensionFactory) {
	extFactory.RegistryExtMessageHandler(Default, func() motan.MessageHandler {
		return &DefaultMessageHandler{}
	})
}

type DefaultExporter struct {
	url        *motan.URL
	Registries []motan.Registry
	extFactory motan.ExtensionFactory
	server     motan.Server
	provider   motan.Provider
	lock       sync.Mutex
	available  bool
	exported   bool

	// 服务管理单位，负责服务注册、心跳、导出和销毁，内部包含provider，与provider是一对一关系
}

func (d *DefaultExporter) GetRuntimeInfo() map[string]interface{} {
	info := map[string]interface{}{}
	if d.url != nil {
		info[motan.RuntimeUrlKey] = d.url.ToExtInfo()
	}
	info[motan.RuntimeProviderKey] = d.provider.GetRuntimeInfo()
	info[motan.RuntimeIsAvailableKey] = d.available
	return info
}

func (d *DefaultExporter) Export(server motan.Server, extFactory motan.ExtensionFactory, context *motan.Context) (err error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	if d.exported {
		return errors.New("exporter already exported")
	}

	if d.provider == nil {
		return errors.New("no provider for export")
	}
	d.extFactory = extFactory
	d.server = server
	d.url = d.provider.GetURL()
	// add server side meta info to the url, so these meta info can be passed to the client side through the registration mechanism.
	if d.url.GetBoolValue(motan.URLRegisterMeta, motan.DefaultRegisterMeta) {
		for k, v := range meta.GetEnvMeta() {
			d.url.PutParam(k, v)
		}
	}
	d.url.PutParam(motan.NodeTypeKey, motan.NodeTypeService) // node type must be service in export
	regs, ok := d.url.Parameters[motan.RegistryKey]
	if !ok {
		errInfo := fmt.Sprintf("registry not found! url %+v", d.url)
		err = errors.New(errInfo)
		vlog.Errorln(errInfo)
		return err
	}
	arr := motan.TrimSplit(regs, ",")
	registries := make([]motan.Registry, 0, len(arr))
	for _, r := range arr {
		if registryURL, ok := context.RegistryURLs[r]; ok {
			registry := d.extFactory.GetRegistry(registryURL)
			if registry != nil {
				registry.Register(d.url)
				registries = append(registries, registry)
			}
		} else {
			err = errors.New("registry is invalid: " + r)
			vlog.Errorln("registry is invalid: " + r)
		}
	}
	d.Registries = registries
	// TODO heartbeat or 200 switcher
	d.exported = true
	d.available = true
	vlog.Infof("export url %s success.", d.url.GetIdentity())
	return nil
}

func (d *DefaultExporter) Unexport() error {
	d.lock.Lock()
	defer d.lock.Unlock()
	if !d.exported {
		return nil
	}
	for _, r := range d.Registries {
		r.UnRegister(d.url)
	}
	d.server.GetMessageHandler().RmProvider(d.provider)
	d.exported = false
	// TODO: gracefully destroy provider
	return nil
}

func (d *DefaultExporter) SetProvider(provider motan.Provider) {
	d.provider = provider
}

func (d *DefaultExporter) GetProvider() motan.Provider {
	return d.provider
}

func (d *DefaultExporter) Available() {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.available = true
}

func (d *DefaultExporter) Unavailable() {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.available = false
}

func (d *DefaultExporter) IsAvailable() bool {
	return d.available
}

func (d *DefaultExporter) GetURL() *motan.URL {
	return d.url
}

func (d *DefaultExporter) SetURL(url *motan.URL) {
	d.url = url
}

type DefaultMessageHandler struct {
	providers          map[string]motan.Provider
	frameworkProviders map[string]motan.Provider
}

func (d *DefaultMessageHandler) GetName() string {
	return "defaultMessageHandler"
}

func (d *DefaultMessageHandler) GetRuntimeInfo() map[string]interface{} {
	info := map[string]interface{}{}
	info[motan.RuntimeMessageHandlerTypeKey] = d.GetName()
	providersInfo := map[string]interface{}{}
	for s, provider := range d.providers {
		providersInfo[s] = provider.GetRuntimeInfo()
	}
	info[motan.RuntimeProvidersKey] = providersInfo
	return info
}

func (d *DefaultMessageHandler) Initialize() {
	d.providers = make(map[string]motan.Provider)
	d.frameworkProviders = make(map[string]motan.Provider)
	d.initFrameworkServiceProvider()
}

func (d *DefaultMessageHandler) initFrameworkServiceProvider() {
	d.frameworkProviders[meta.MetaServiceName] = &provider.MetaProvider{}
}

func (d *DefaultMessageHandler) AddProvider(p motan.Provider) error {
	d.providers[p.GetPath()] = p
	return nil
}

func (d *DefaultMessageHandler) RmProvider(p motan.Provider) {
	dp := d.providers[p.GetPath()]
	if dp != nil && p == dp {
		delete(d.providers, p.GetPath())
	}
}

func (d *DefaultMessageHandler) GetProvider(serviceName string) motan.Provider {
	return d.providers[serviceName]
}

func (d *DefaultMessageHandler) Call(request motan.Request) (res motan.Response) {
	defer motan.HandlePanic(func() {
		res = motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "provider call panic", ErrType: motan.ServiceException})
		vlog.Errorf("provider call panic. req:%s", motan.GetReqInfo(request))
	})
	if mfs := request.GetAttachment(mpro.MFrameworkService); mfs != "" {
		if fp, ok := d.frameworkProviders[request.GetServiceName()]; ok {
			return fp.(motan.Provider).Call(request)
		}
		//throw specific exception to avoid triggering forced fusing on the client side。
		return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 501, ErrMsg: motan.ServiceNotSupport, ErrType: motan.ServiceException})
	}
	p := d.providers[request.GetServiceName()]
	if p != nil {
		res = p.Call(request)
		res.GetRPCContext(true).GzipSize = int(p.GetURL().GetIntValue(motan.GzipSizeKey, 0))
		return res
	}
	vlog.Errorf("%s%s%s", motan.ProviderNotExistPrefix, request.GetServiceName(), motan.GetReqInfo(request))
	return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: motan.EProviderNotExist, ErrMsg: motan.ProviderNotExistPrefix + request.GetServiceName(), ErrType: motan.ServiceException})
}

type FilterProviderWrapper struct {
	provider motan.Provider
	filter   motan.EndPointFilter
}

func (f *FilterProviderWrapper) GetRuntimeInfo() map[string]interface{} {
	info := f.provider.GetRuntimeInfo()
	var filterInfo []interface{}
	filter := f.filter
	for filter != nil {
		filterInfo = append(filterInfo, filter.GetRuntimeInfo())
		filter = filter.GetNext()
	}
	if len(filterInfo) > 0 {
		info[motan.RuntimeFiltersKey] = filterInfo
	}
	return info
}

func (f *FilterProviderWrapper) SetService(s interface{}) {
	f.provider.SetService(s)
}

func (f *FilterProviderWrapper) GetURL() *motan.URL {
	return f.provider.GetURL()
}

func (f *FilterProviderWrapper) SetURL(url *motan.URL) {
	f.provider.SetURL(url)
}

func (f *FilterProviderWrapper) GetPath() string {
	return f.provider.GetPath()
}

func (f *FilterProviderWrapper) IsAvailable() bool {
	return f.provider.IsAvailable()
}

func (f *FilterProviderWrapper) Destroy() {
	f.provider.Destroy()
}

func (f *FilterProviderWrapper) Call(request motan.Request) (res motan.Response) {
	return f.filter.Filter(f.provider, request)
}

func WrapWithFilter(provider motan.Provider, extFactory motan.ExtensionFactory, context *motan.Context) motan.Provider {
	var lastf motan.EndPointFilter
	lastf = motan.GetLastEndPointFilter()
	_, filters := motan.GetURLFilters(provider.GetURL(), extFactory)
	for _, f := range filters {
		if filter := f.NewFilter(provider.GetURL()); filter != nil {
			if ef, ok := filter.(motan.EndPointFilter); ok {
				motan.CanSetContext(ef, context)
				ef.SetNext(lastf)
				lastf = ef
			}
		}
	}
	fpw := &FilterProviderWrapper{provider: provider, filter: lastf}
	vlog.Infof("FilterProviderWrapper url: %+v, filter size:%d, filters:%s", provider.GetURL(), len(filters), motan.GetEPFilterInfo(fpw.filter))
	return fpw
}
