package cluster

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

type MotanCluster struct {
	Context        *motan.Context
	url            *motan.Url
	Registrys      []motan.Registry
	HaStrategy     motan.HaStrategy
	LoadBalance    motan.LoadBalance
	Refers         []motan.EndPoint
	Filters        []motan.Filter
	clusterFilter  motan.ClusterFilter
	extFactory     motan.ExtentionFactory
	registryRefers map[string][]motan.EndPoint
	notifyLock     sync.Mutex
	available      bool
	closed         bool
	proxy          bool
}

func (m *MotanCluster) IsAvaiable() bool {
	return m.available
}

func NewCluster(url *motan.Url, proxy bool) *MotanCluster {
	cluster := &MotanCluster{url: url, proxy: proxy}
	return cluster
}

func (m *MotanCluster) GetName() string {
	return "MotanCluster"
}

func (m *MotanCluster) GetUrl() *motan.Url {
	return m.url
}

func (m *MotanCluster) SetUrl(url *motan.Url) {
	m.url = url
}
func (m *MotanCluster) Call(request motan.Request) motan.Response {
	if m.available {
		return m.clusterFilter.Filter(m.HaStrategy, m.LoadBalance, request)
	} else {
		vlog.Infoln("cluster:" + m.GetIdentity() + "is not available!")
		return motan.BuildExceptionResponse(request.GetRequestId(), &motan.Exception{ErrCode: 500, ErrMsg: "service cluster not available. maybe caused by degrade.", ErrType: motan.ServiceException})
	}

}
func (m *MotanCluster) InitCluster() bool {
	m.registryRefers = make(map[string][]motan.EndPoint)
	//ha
	m.HaStrategy = m.extFactory.GetHa(m.url)
	//lb
	m.LoadBalance = m.extFactory.GetLB(m.url)
	//filter
	m.initFilters()
	// parse registry and subscribe
	m.parseRegistry()

	if m.clusterFilter == nil {
		m.clusterFilter = motan.GetLastClusterFilter()
	}
	if m.Filters == nil {
		m.Filters = make([]motan.Filter, 0)
	}
	//TODO weather has available refers
	m.available = true
	m.closed = false
	vlog.Infof("init MotanCluster %s\n", m.GetIdentity())

	return true
}
func (m *MotanCluster) SetLoadBalance(loadBalance motan.LoadBalance) {
	m.LoadBalance = loadBalance
}
func (m *MotanCluster) SetHaStrategy(haStrategy motan.HaStrategy) {
	m.HaStrategy = haStrategy
}
func (m *MotanCluster) GetRefers() []motan.EndPoint {
	return m.Refers
}
func (m *MotanCluster) refresh() {
	newRefers := make([]motan.EndPoint, 0, 32)
	for _, v := range m.registryRefers {
		for _, e := range v {
			newRefers = append(newRefers, e)
		}
	}
	m.Refers = newRefers
	m.LoadBalance.OnRefresh(newRefers)
}
func (m *MotanCluster) AddRegistry(registry motan.Registry) {
	m.Registrys = append(m.Registrys, registry)
}
func (m *MotanCluster) Notify(registryUrl *motan.Url, urls []*motan.Url) {
	vlog.Infof("cluster %s receive notify size %d. \n", m.GetIdentity(), len(urls))
	m.notifyLock.Lock()
	defer m.notifyLock.Unlock()
	// process weight if has
	urls = processWeight(m, urls)
	endpoints := make([]motan.EndPoint, 0, len(urls))
	endpointMap := make(map[string]motan.EndPoint)
	if eps, ok := m.registryRefers[registryUrl.GetIdentity()]; ok {
		for _, ep := range eps {
			endpointMap[ep.GetUrl().GetIdentity()] = ep
		}
	}

	for _, u := range urls {
		if u == nil {
			vlog.Errorln("cluster recieve nil url!")
			continue
		}
		vlog.Infof("cluster %s received notify url:%s:%d\n", m.GetIdentity(), u.Host, u.Port)
		if !u.CanServe(m.url) {
			vlog.Infof("cluster notify:can not use server:%+v\n", u)
			continue
		}
		var ep motan.EndPoint
		if tempEp, ok := endpointMap[u.GetIdentity()]; ok {
			ep = tempEp
			delete(endpointMap, u.GetIdentity())
		}
		if ep == nil {
			newUrl := u.Copy()
			newUrl.MergeParams(m.url.Parameters)
			ep = m.extFactory.GetEndPoint(newUrl)

			if ep != nil {
				motan.Initialize(ep)
				ep.SetProxy(m.proxy)
				ep.SetSerialization(motan.GetSerialization(newUrl, m.extFactory))
				ep = m.addFilter(ep, m.Filters)
			}
		}

		if ep != nil {
			endpoints = append(endpoints, ep)
		}
	}
	if len(endpoints) == 0 {
		if len(m.registryRefers) > 1 {
			delete(m.registryRefers, registryUrl.GetIdentity())
		} else {
			// notify will ignored if endpoints size is 0 in single regisry mode
			vlog.Infof("cluster %s notify endpoint is 0. notify ignored.\n", m.GetIdentity())
			return
		}
	} else {
		m.registryRefers[registryUrl.GetIdentity()] = endpoints
	}
	m.refresh()
	for _, ep := range endpointMap {
		ep.Destroy()
	}
}

// remove rule protocol && set weight
func processWeight(m *MotanCluster, urls []*motan.Url) []*motan.Url {
	weight := ""
	if len(urls) > 0 && urls[len(urls)-1] != nil && urls[len(urls)-1].Protocol == "rule" {
		url := urls[len(urls)-1]
		weight = url.Parameters["weight"]
		urls = urls[:len(urls)-1]
	}
	m.LoadBalance.SetWeight(weight)
	return urls
}

func (m *MotanCluster) addFilter(ep motan.EndPoint, filters []motan.Filter) motan.EndPoint {
	fep := &motan.FilterEndPoint{Url: ep.GetUrl(), Caller: ep}
	statusFilters := make([]motan.Status, 0, len(filters))
	var lastf motan.EndPointFilter
	lastf = motan.GetLastEndPointFilter()
	for _, f := range filters {
		if ef, ok := f.NewFilter(ep.GetUrl()).(motan.EndPointFilter); ok {
			motan.CanSetContext(ef, m.Context)
			ef.SetNext(lastf)
			lastf = ef
			if sf, ok := ef.(motan.Status); ok {
				statusFilters = append(statusFilters, sf)
			}
		}
	}
	fep.StatusFilters = statusFilters
	fep.Filter = lastf
	return fep
}
func (m *MotanCluster) GetIdentity() string {
	return m.url.GetIdentity()
}
func (m *MotanCluster) Destroy() {
	if !m.closed {
		m.notifyLock.Lock()
		defer m.notifyLock.Unlock()
		vlog.Infof("cluster %s will destroy.\n", m.url.GetIdentity())
		for _, r := range m.Registrys {
			vlog.Infof("unsubscribe from registry %s .\n", r.GetUrl().GetIdentity())
			r.Unsubscribe(m.url, m)
		}
		for _, e := range m.Refers {
			vlog.Infof("destroy endpoint %s .\n", e.GetUrl().GetIdentity())
			e.Destroy()
		}
		m.closed = true
	}
}

func (m *MotanCluster) SetExtFactory(factory motan.ExtentionFactory) {
	m.extFactory = factory
}

func (m *MotanCluster) parseRegistry() (err error) {
	regs, ok := m.url.Parameters[motan.RegistryKey]
	if !ok {
		errInfo := fmt.Sprintf("registry not found! url %+v", m.url)
		err = errors.New(errInfo)
		vlog.Errorln(errInfo)
	}
	arr := strings.Split(regs, ",")
	registries := make([]motan.Registry, 0, len(arr))
	for _, r := range arr {
		if registryUrl, ok := m.Context.RegistryUrls[r]; ok {
			registry := m.extFactory.GetRegistry(registryUrl)
			if registry != nil {
				if _, ok := registry.(motan.DiscoverCommand); ok {
					registry = GetCommandRegistryWarper(m, registry)
				}
				registry.Subscribe(m.url, m)
				registries = append(registries, registry)
				urls := registry.Discover(m.url)
				m.Notify(registryUrl, urls)
			}
		} else {
			err = errors.New("registry is invalid: " + r)
			vlog.Errorln("registry is invalid: " + r)
		}

	}
	m.Registrys = registries
	return err
}

func (m *MotanCluster) initFilters() {
	clusterFilter, endpointFilters := motan.GetUrlFilters(m.url, m.extFactory)
	if clusterFilter != nil {
		m.clusterFilter = clusterFilter
	}
	if len(endpointFilters) > 0 {
		m.Filters = endpointFilters
	}
}

func (m *MotanCluster) NotifyAgentCommand(commandInfo string) {
	for _, reg := range m.Registrys {
		if notifyRegisry, ok := reg.(motan.CommandNotifyListener); ok {
			notifyRegisry.NotifyCommand(m.url, AGENT_CMD, commandInfo)
		}
	}
}
