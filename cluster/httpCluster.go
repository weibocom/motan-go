package cluster

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/http"
	"github.com/weibocom/motan-go/lb"
	"github.com/weibocom/motan-go/log"
	"github.com/weibocom/motan-go/protocol"
)

const (
	HTTPProxyPreloadKey       = "preload"
	HTTPProxyDomainIdcNetmask = "domainIdcNetmask"
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
	masterGroup      string
	lock             sync.RWMutex
	extFactory       core.ExtensionFactory
	context          *core.Context
	serviceRegistry  core.ServiceDiscoverableRegistry
}

type HTTPResolveIdcFunc func(domain string, httpCluster *HTTPCluster) []string

var HTTPResolveIdc HTTPResolveIdcFunc = HTTPResolveIdcByDNS

func HTTPResolveIdcByDNS(domain string, httpCluster *HTTPCluster) []string {
	netmaskStr := httpCluster.GetURL().GetStringValue(HTTPProxyDomainIdcNetmask)
	if netmaskStr == "" {
		return nil
	}
	var resolvedIPs []net.IP
	for i := 0; i < 10; i++ {
		ips, err := net.LookupIP(domain)
		if err != nil {
			vlog.Errorf("Resolve host %s failed: %s", domain, err.Error())
			time.Sleep(10 * time.Millisecond)
			continue
		}
		resolvedIPs = ips
		break
	}
	if len(resolvedIPs) == 0 {
		return nil
	}
	idcMasks := http.PatternSplit(netmaskStr, http.CommaSplitPattern)
	type idcNet struct {
		idc   string
		ipNet *net.IPNet
	}
	idcNets := make([]idcNet, 0, len(idcMasks))
	for _, idcMask := range idcMasks {
		if idcMask == "" {
			continue
		}
		// TODO: with ipv6 we need more check for this
		// configuration like 192.168.1.0/24:idc
		netAndIDC := core.TrimSplit(idcMask, ":")
		if len(netAndIDC) < 2 {
			panic("wrong idc netmask: " + idcMask)
		}
		_, ipNet, err := net.ParseCIDR(netAndIDC[0])
		if err != nil {
			panic("wrong idc netmask " + idcMask + " error: " + err.Error())
		}
		idcNets = append(idcNets, idcNet{idc: netAndIDC[1], ipNet: ipNet})
	}
	idcMap := make(map[string]string, len(resolvedIPs))
	for _, ip := range resolvedIPs {
		for _, n := range idcNets {
			if n.ipNet.Contains(ip) {
				idcMap[n.idc] = n.idc
				break
			}
		}
	}
	idcs := make([]string, 0, len(idcMap))
	for k := range idcMap {
		idcs = append(idcs, k)
	}
	return idcs
}

func NewHTTPCluster(url *core.URL, proxy bool, context *core.Context, extFactory core.ExtensionFactory) *HTTPCluster {
	c := &HTTPCluster{
		url:         url,
		proxy:       proxy,
		context:     context,
		extFactory:  extFactory,
		masterGroup: url.Group,
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
		// with http cluster we prepare the group and just do not use backup group
		originalGroup := url.Group
		if strings.Contains(originalGroup, backupGroupDelimiter) {
			groups := core.TrimSplit(originalGroup, ",")
			if len(groups) > 0 {
				c.masterGroup = groups[0]
			}
		} else if strings.Contains(originalGroup, clusterIdcPlaceHolder) {
			var groups []string
			idcs := HTTPResolveIdc(domain, c)
			if len(idcs) != 0 {
				groups = make([]string, 0, len(idcs))
				for _, idc := range idcs {
					groups = append(groups, strings.Replace(originalGroup, clusterIdcPlaceHolder, idc, 1))
				}
			} else if gr, ok := registry.(core.GroupDiscoverableRegistry); ok {
				groups = GetAllMatchedGroup(originalGroup, gr)
			}
			if len(groups) >= 1 {
				c.masterGroup = groups[0]
				url.SetGroup(strings.Join(groups, ","))
			}
			vlog.Infof("Http cluster for domain %s resolve group %s to %s", domain, originalGroup, url.Group)
		}
		// http cluster do not use backup group as default
		if url.GetStringValue(core.GroupLoadBalanceKey) == "" {
			url.PutParam(core.GroupLoadBalanceKey, lb.Random)
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

func (c *HTTPCluster) Context() *core.Context {
	return c.context
}

func (c *HTTPCluster) CanServe(uri string) (string, bool) {
	service := c.uriConverter.URIToServiceName(uri)
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

	if core.ServiceInGroup(c.serviceRegistry, c.masterGroup, service) {
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
