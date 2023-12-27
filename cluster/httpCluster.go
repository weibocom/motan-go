package cluster

import (
	"sync"

	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/http"
	"github.com/weibocom/motan-go/log"
	"github.com/weibocom/motan-go/protocol"
)

const (
	HTTPProxyPreloadKey = "preload"
)

type clusterHolder struct {
	initialized bool
	url         *core.URL
	lock        sync.Mutex
	cluster     *MotanCluster
	proxy       bool
	extFactory  core.ExtensionFactory
	context     *core.Context
}

func (c *clusterHolder) getCluster() *MotanCluster {
	if c.initialized {
		return c.cluster
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.initialized {
		return c.cluster
	}

	motanCluster := NewCluster(c.context, c.extFactory, c.url, c.proxy)
	c.cluster = motanCluster
	c.initialized = true
	return c.cluster
}

func (c *clusterHolder) destroy() {
	c.lock.Lock()
	defer c.lock.Unlock()
	if !c.initialized {
		return
	}
	c.cluster.Destroy()
	c.initialized = false
}

func newClusterHolder(url *core.URL, context *core.Context, extFactory core.ExtensionFactory, proxy bool) *clusterHolder {
	return &clusterHolder{
		url:        url,
		proxy:      proxy,
		extFactory: extFactory,
		context:    context,
	}
}

type HTTPCluster struct {
	url *core.URL
	// for get the service from uri
	uriConverter     http.URIConverter
	proxy            bool
	upstreamClusters map[string]*clusterHolder
	lock             sync.RWMutex
	extFactory       core.ExtensionFactory
	context          *core.Context
	serviceRegistry  core.ServiceDiscoverableRegistry
}

func NewHTTPCluster(url *core.URL, proxy bool, context *core.Context, extFactory core.ExtensionFactory) *HTTPCluster {
	c := &HTTPCluster{
		url:        url,
		proxy:      proxy,
		context:    context,
		extFactory: extFactory,
	}
	domain := url.GetParam(http.DomainKey, "")
	if domain == "" {
		vlog.Errorf("HTTP client has no domain config")
		return nil
	}
	registryID := url.GetParam(core.RegistryKey, "")
	// TODO: for multiple registry
	if registryURL, ok := context.RegistryURLs[registryID]; ok {
		registry := extFactory.GetRegistry(registryURL)
		if registry == nil {
			vlog.Errorf("Can not found registry %s : %v", registryID, registryURL)
			return nil
		}
		if sr, ok := registry.(core.ServiceDiscoverableRegistry); ok {
			c.serviceRegistry = sr
		}
	}
	c.upstreamClusters = make(map[string]*clusterHolder, 16)
	c.uriConverter = http.NewLocationMatcherFromContext(domain, context)
	preload := core.TrimSplit(url.GetParam(HTTPProxyPreloadKey, ""), ",")
	// TODO: find service by location then do warm up
	for _, service := range preload {
		if service == "" {
			continue
		}
		c.getMotanCluster(service)
	}
	return c
}

func (c *HTTPCluster) GetRuntimeInfo() map[string]interface{} {
	info := map[string]interface{}{
		core.RuntimeNameKey: c.GetName(),
	}
	if c.url != nil {
		info[core.RuntimeUrlKey] = c.url.ToExtInfo()
	}
	return info
}

func (c *HTTPCluster) CanServe(uri string) (string, bool) {
	service := c.uriConverter.URIToServiceName(uri, nil)
	if service == "" {
		return "", false
	}
	// if the registry can not find all service of it
	// we just use preload services
	if c.serviceRegistry == nil {
		c.lock.RLock()
		defer c.lock.RUnlock()
		if _, ok := c.upstreamClusters[service]; ok {
			return service, true
		}
		return "", false
	}

	if core.ServiceInGroup(c.serviceRegistry, c.url.Group, service) {
		return service, true
	}
	return "", false
}

func (c *HTTPCluster) GetIdentity() string {
	return c.url.GetIdentity()
}

func (c *HTTPCluster) Notify(registryURL *core.URL, urls []*core.URL) {
}

func (c *HTTPCluster) GetName() string {
	return "httpCluster"
}

func (c *HTTPCluster) GetURL() *core.URL {
	return c.url
}

func (c *HTTPCluster) SetURL(url *core.URL) {
	c.url = url
}

func (c *HTTPCluster) IsAvailable() bool {
	return true
}

func (c *HTTPCluster) getMotanCluster(service string) *MotanCluster {
	c.lock.RLock()
	if holder, ok := c.upstreamClusters[service]; ok {
		c.lock.RUnlock()
		return holder.getCluster()
	}
	c.lock.RUnlock()
	holder := func() *clusterHolder {
		c.lock.Lock()
		defer c.lock.Unlock()
		if holder, ok := c.upstreamClusters[service]; ok {
			return holder
		}
		url := c.url.Copy()
		url.Path = service
		holder := newClusterHolder(url, c.context, c.extFactory, c.proxy)
		c.upstreamClusters[service] = holder
		return holder
	}()
	return holder.getCluster()
}

func (c *HTTPCluster) removeMotanCluster(service string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.upstreamClusters, service)
}

func (c *HTTPCluster) Call(request core.Request) core.Response {
	cluster := c.getMotanCluster(request.GetServiceName())
	if request.GetAttachment(protocol.MSource) == "" {
		request.SetAttachment(protocol.MSource, c.url.GetParam(core.ApplicationKey, ""))
	}
	return cluster.Call(request)
}

func (c *HTTPCluster) Destroy() {
	c.lock.Lock()
	defer c.lock.Unlock()
	for _, cc := range c.upstreamClusters {
		cc.destroy()
	}
	c.upstreamClusters = make(map[string]*clusterHolder)
}

func (c *HTTPCluster) SetSerialization(s core.Serialization) {
}

func (c *HTTPCluster) SetProxy(proxy bool) {
	c.proxy = proxy
}
