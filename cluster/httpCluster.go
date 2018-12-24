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

type cacheCluster struct {
	initialized bool
	url         *core.URL
	lock        sync.Mutex
	cluster     *MotanCluster
	proxy       bool
	extFactory  core.ExtensionFactory
	context     *core.Context
}

func (c *cacheCluster) getCluster() *MotanCluster {
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

func (c *cacheCluster) destroy() {
	c.lock.Lock()
	defer c.lock.Unlock()
	if !c.initialized {
		return
	}
	c.cluster.Destroy()
	c.initialized = false
}

func newCacheCluster(url *core.URL, context *core.Context, extFactory core.ExtensionFactory, proxy bool) *cacheCluster {
	return &cacheCluster{
		url:        url,
		proxy:      proxy,
		extFactory: extFactory,
		context:    context,
	}
}

type HTTPCluster struct {
	url              *core.URL
	serviceDiscover  http.ServiceDiscover
	proxy            bool
	upstreamClusters map[string]*cacheCluster
	lock             sync.Mutex
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
	domain := url.GetParam("domain", "")
	if domain == "" {
		vlog.Errorf("When use a http proxy client domain must configured")
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
	c.serviceDiscover = http.NewLocationMatcherFromContext(domain, context)
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

func (c *HTTPCluster) CanServe(uri string) (string, bool) {
	service := c.serviceDiscover.DiscoverService(uri)
	if service == "" {
		return "", false
	}
	// if the registry can not find all service of it
	// we just use preload services
	if c.serviceRegistry == nil {
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
	if cc, ok := c.upstreamClusters[service]; ok {
		return cc.getCluster()
	}
	c.lock.Lock()
	if cc, ok := c.upstreamClusters[service]; ok {
		c.lock.Unlock()
		return cc.getCluster()
	}
	newUpstreamClusters := make(map[string]*cacheCluster, len(c.upstreamClusters)+1)
	for u, cc := range c.upstreamClusters {
		newUpstreamClusters[u] = cc
	}
	url := c.url.Copy()
	url.Path = service
	cc := newCacheCluster(url, c.context, c.extFactory, c.proxy)
	newUpstreamClusters[service] = cc
	c.upstreamClusters = newUpstreamClusters
	c.lock.Unlock()
	return cc.getCluster()
}

func (c *HTTPCluster) removeMotanCluster(service string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if _, ok := c.upstreamClusters[service]; !ok {
		return
	}
	newUpstreamClusters := make(map[string]*cacheCluster, len(c.upstreamClusters))
	for upstream, cluster := range c.upstreamClusters {
		if upstream == service {
			continue
		}
		newUpstreamClusters[upstream] = cluster
	}
	c.upstreamClusters = newUpstreamClusters
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
	clusters := c.upstreamClusters
	c.upstreamClusters = make(map[string]*cacheCluster)
	for _, cc := range clusters {
		cc.destroy()
	}
}

func (c *HTTPCluster) SetSerialization(s core.Serialization) {
}

func (c *HTTPCluster) SetProxy(proxy bool) {
	c.proxy = proxy
}
