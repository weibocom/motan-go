package server

import (
	"errors"
	"fmt"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	Motan2 = "motan2"
	CGI    = "cgi"
)

const (
	Default = "default"
)

func RegistDefaultServers(extFactory motan.ExtentionFactory) {
	extFactory.RegistExtServer(Motan2, func(url *motan.URL) motan.Server {
		return &MotanServer{URL: url}
	})
	extFactory.RegistExtServer(CGI, func(url *motan.URL) motan.Server {
		return &MotanServer{URL: url}
	})
}

func RegistDefaultMessageHandlers(extFactory motan.ExtentionFactory) {
	extFactory.RegistryExtMessageHandler(Default, func() motan.MessageHandler {
		return &DefaultMessageHandler{}
	})
}

type DefaultExporter struct {
	url        *motan.URL
	Registrys  []motan.Registry
	extFactory motan.ExtentionFactory
	server     motan.Server
	provider   motan.Provider

	// 服务管理单位，负责服务注册、心跳、导出和销毁，内部包含provider，与provider是一对一关系
}

func (d *DefaultExporter) Export(server motan.Server, extFactory motan.ExtentionFactory, context *motan.Context) (err error) {
	if d.provider == nil {
		err = errors.New("no provider for export")
		return err
	}
	d.extFactory = extFactory
	d.server = server
	d.url = d.provider.GetURL()
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
	d.Registrys = registries
	// TODO heartbeat or 200 switcher
	vlog.Infof("export url %s success.\n", d.url.GetIdentity())
	return nil
}

func (d *DefaultExporter) Unexport() error {
	for _, r := range d.Registrys {
		r.UnRegister(d.url)
	}
	d.server.GetMessageHandler().RmProvider(d.provider)
	return nil
}

func (d *DefaultExporter) SetProvider(provider motan.Provider) {
	d.provider = provider
}

func (d *DefaultExporter) GetProvider() motan.Provider {
	return d.provider
}

func (d *DefaultExporter) GetURL() *motan.URL {
	return d.url
}

func (d *DefaultExporter) SetURL(url *motan.URL) {
	d.url = url
}

type DefaultMessageHandler struct {
	providers map[string]motan.Provider
}

func (d *DefaultMessageHandler) Initialize() {
	d.providers = make(map[string]motan.Provider)
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
		vlog.Errorf("provider call panic. req:%s\n", motan.GetReqInfo(request))
	})
	p := d.providers[request.GetServiceName()]
	if p != nil {
		res = p.Call(request)
		res.GetRPCContext(true).GzipSize = int(p.GetURL().GetIntValue(motan.GzipSizeKey, 0))
		return res
	}
	vlog.Errorf("not found provider for %s\n", motan.GetReqInfo(request))
	return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "not found provider for " + request.GetServiceName(), ErrType: motan.ServiceException})
}

type FilterProviderWarper struct {
	provider motan.Provider
	filter   motan.EndPointFilter
}

func (f *FilterProviderWarper) SetService(s interface{}) {
	f.provider.SetService(s)
}

func (f *FilterProviderWarper) GetURL() *motan.URL {
	return f.provider.GetURL()
}

func (f *FilterProviderWarper) SetURL(url *motan.URL) {
	f.provider.SetURL(url)
}

func (f *FilterProviderWarper) GetPath() string {
	return f.provider.GetPath()
}

func (f *FilterProviderWarper) IsAvailable() bool {
	return f.provider.IsAvailable()
}

func (f *FilterProviderWarper) Destroy() {
	f.provider.Destroy()
}

func (f *FilterProviderWarper) Call(request motan.Request) (res motan.Response) {
	return f.filter.Filter(f.provider, request)
}

func WarperWithFilter(provider motan.Provider, extFactory motan.ExtentionFactory) motan.Provider {
	var lastf motan.EndPointFilter
	lastf = motan.GetLastEndPointFilter()
	_, filters := motan.GetURLFilters(provider.GetURL(), extFactory)
	for _, f := range filters {
		if ef, ok := f.NewFilter(provider.GetURL()).(motan.EndPointFilter); ok {
			ef.SetNext(lastf)
			lastf = ef
		}
	}
	return &FilterProviderWarper{provider: provider, filter: lastf}
}
