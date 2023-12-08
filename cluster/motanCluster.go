package cluster

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

type MotanCluster struct {
	Context        *motan.Context
	url            *motan.URL
	Registries     []motan.Registry
	HaStrategy     motan.HaStrategy
	LoadBalance    motan.LoadBalance
	Refers         []motan.EndPoint
	Filters        []motan.Filter
	clusterFilter  motan.ClusterFilter
	extFactory     motan.ExtensionFactory
	registryRefers map[string][]motan.EndPoint
	notifyLock     sync.Mutex
	available      bool
	closed         bool
	proxy          bool

	// cached identity
	identity motan.AtomicString
}

func (m *MotanCluster) IsAvailable() bool {
	return m.available
}

func NewCluster(context *motan.Context, extFactory motan.ExtensionFactory, url *motan.URL, proxy bool) *MotanCluster {
	cluster := &MotanCluster{
		Context:    context,
		extFactory: extFactory,
		url:        url,
		proxy:      proxy,
	}
	cluster.initCluster()
	return cluster
}

func (m *MotanCluster) GetName() string {
	return "MotanCluster"
}

func (m *MotanCluster) GetURL() *motan.URL {
	return m.url
}

func (m *MotanCluster) SetURL(url *motan.URL) {
	m.url = url
}

func (m *MotanCluster) Call(request motan.Request) (res motan.Response) {
	defer motan.HandlePanic(func() {
		res = motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "cluster call panic", ErrType: motan.ServiceException})
		vlog.Errorf("cluster call panic. req:%s", motan.GetReqInfo(request))
	})
	if m.available {
		return m.clusterFilter.Filter(m.HaStrategy, m.LoadBalance, request)
	}
	vlog.Infoln("cluster:" + m.GetIdentity() + "is not available!")
	return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "cluster not available, maybe caused by degrade", ErrType: motan.ServiceException})
}

func (m *MotanCluster) initCluster() bool {
	m.registryRefers = make(map[string][]motan.EndPoint)
	//ha
	m.HaStrategy = m.extFactory.GetHa(m.url)
	//lb
	m.LoadBalance = m.extFactory.GetLB(m.url)
	//filter should initialize after HaStrategy
	m.initFilters()

	if m.clusterFilter == nil {
		m.clusterFilter = motan.GetLastClusterFilter()
	}
	if m.Filters == nil {
		m.Filters = make([]motan.Filter, 0)
	}
	//TODO whether has available refers
	m.available = true
	m.closed = false

	// parse registry and subscribe
	err := m.parseRegistry()
	if err != nil {
		vlog.Errorf("init MotanCluster fail. cluster:%s, err:%s", m.GetIdentity(), err.Error())
		return false
	}
	vlog.Infof("init MotanCluster %s", m.GetIdentity())
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
	// shuffle endpoints list avoid to call to determine server nodes when the list is not change.
	newRefers = m.ShuffleEndpoints(newRefers)
	m.Refers = newRefers
	m.LoadBalance.OnRefresh(newRefers)
}

func (m *MotanCluster) ShuffleEndpoints(endpoints []motan.EndPoint) []motan.EndPoint {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(endpoints), func(i, j int) { endpoints[i], endpoints[j] = endpoints[j], endpoints[i] })
	return endpoints
}

func (m *MotanCluster) AddRegistry(registry motan.Registry) {
	m.Registries = append(m.Registries, registry)
}

func (m *MotanCluster) Notify(registryURL *motan.URL, urls []*motan.URL) {
	vlog.Infof("cluster %s receive notify size %d. ", m.GetIdentity(), len(urls))
	m.notifyLock.Lock()
	defer m.notifyLock.Unlock()
	// process weight if any
	urls = processWeight(m, urls)
	endpoints := make([]motan.EndPoint, 0, len(urls))
	endpointMap := make(map[string]motan.EndPoint)
	if eps, ok := m.registryRefers[registryURL.GetIdentity()]; ok {
		for _, ep := range eps {
			endpointMap[ep.GetURL().GetIdentity()] = ep
		}
	}

	for _, u := range urls {
		if u == nil {
			vlog.Errorln("cluster receive nil url!")
			continue
		}
		vlog.Infof("cluster %s received notify url:%s:%d", m.GetIdentity(), u.Host, u.Port)
		if !u.CanServe(m.url) {
			vlog.Infof("cluster notify:can not use server:%+v", u)
			continue
		}
		var ep motan.EndPoint
		if tempEp, ok := endpointMap[u.GetIdentity()]; ok {
			ep = tempEp
			delete(endpointMap, u.GetIdentity())
		}
		if ep == nil {
			newURL := u.Copy()
			newURL.MergeParams(m.url.Parameters)
			ep = m.extFactory.GetEndPoint(newURL)

			if ep != nil {
				ep.SetProxy(m.proxy)
				serialization := motan.GetSerialization(newURL, m.extFactory)
				if serialization == nil {
					vlog.Warningf("MotanCluster can not find Serialization in DefaultExtensionFactory! url:%+v", m.url)
				} else {
					ep.SetSerialization(serialization)
				}
				motan.Initialize(ep)
				ep = m.addFilter(ep, m.Filters)
			}
		}

		if ep != nil {
			endpoints = append(endpoints, ep)
		}
	}
	if len(endpoints) == 0 {
		if len(m.registryRefers) > 1 {
			delete(m.registryRefers, registryURL.GetIdentity())
		} else {
			// ignored if endpoints size is 0 in single registry mode
			vlog.Infof("cluster %s notify endpoint is 0. notify ignored.", m.GetIdentity())
			return
		}
	} else {
		m.registryRefers[registryURL.GetIdentity()] = endpoints
	}
	m.refresh()
	go func() {
		defer motan.HandlePanic(nil)
		time.Sleep(time.Second * 2)
		for _, ep := range endpointMap {
			ep.Destroy()
		}
	}()
}

// remove rule protocol && set weight
func processWeight(m *MotanCluster, urls []*motan.URL) []*motan.URL {
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
	fep := &motan.FilterEndPoint{URL: ep.GetURL(), Caller: ep}
	statusFilters := make([]motan.Status, 0, len(filters))
	var lf motan.EndPointFilter
	lf = motan.GetLastEndPointFilter()
	for _, f := range filters {
		if filter := f.NewFilter(ep.GetURL()); filter != nil {
			if ef, ok := filter.(motan.EndPointFilter); ok {
				motan.CanSetContext(ef, m.Context)
				ef.SetNext(lf)
				lf = ef
				if sf, ok := ef.(motan.Status); ok {
					statusFilters = append(statusFilters, sf)
				}
			}
		}
	}
	fep.StatusFilters = statusFilters
	fep.Filter = lf
	vlog.Infof("MotanCluster add ep filters. url:%s, filters:%s", ep.GetURL().GetIdentity(), motan.GetEPFilterInfo(fep.Filter))
	return fep
}

func (m *MotanCluster) GetIdentity() string {
	id := m.identity.Load()
	if id == "" {
		// TODO: if using moving GC another object may use the same address as m,
		//       but now it's ok
		id = fmt.Sprintf("%p-%s", m, m.url.GetIdentity())
		m.identity.Store(id)
	}
	return id
}

func (m *MotanCluster) Destroy() {
	if !m.closed {
		m.notifyLock.Lock()
		defer m.notifyLock.Unlock()
		vlog.Infof("cluster %s will destroy.", m.url.GetIdentity())
		for _, r := range m.Registries {
			vlog.Infof("unsubscribe from registry %s .", r.GetURL().GetIdentity())
			r.Unsubscribe(m.url, m)
		}
		for _, e := range m.Refers {
			vlog.Infof("destroy endpoint %s .", e.GetURL().GetIdentity())
			e.Destroy()
		}
		m.closed = true
	}
}

func (m *MotanCluster) SetExtFactory(factory motan.ExtensionFactory) {
	m.extFactory = factory
}

func (m *MotanCluster) parseRegistry() (err error) {
	envReg := motan.GetDirectEnvRegistry(m.url)
	if envReg != nil { // If the direct url is specified by the env variable, other registries are ignored
		return m.parseFromEnvRegistry(envReg)
	}
	regs, ok := m.url.Parameters[motan.RegistryKey]
	if !ok {
		errInfo := fmt.Sprintf("registry not found! url %+v", m.url)
		err = errors.New(errInfo)
		vlog.Errorln(errInfo)
		return
	}
	arr := motan.TrimSplit(regs, ",")
	registries := make([]motan.Registry, 0, len(arr))
	for _, r := range arr {
		if registryURL, ok := m.Context.RegistryURLs[r]; ok {
			registry := m.extFactory.GetRegistry(registryURL)
			if registry == nil {
				continue
			}
			m.url.Group = getSubscribeGroup(m.url, registry)
			m.url.ClearCachedInfo()
			if _, ok := registry.(motan.DiscoverCommand); ok {
				registry = GetCommandRegistryWrapper(m, registry)
			}
			registry.Subscribe(m.url, m)
			registries = append(registries, registry)
			urls := registry.Discover(m.url)
			m.Notify(registryURL, urls)
		} else {
			err = errors.New("registry is invalid: " + r)
			vlog.Errorln("registry is invalid: " + r)
		}

	}
	m.Registries = registries
	return err
}

func (m *MotanCluster) parseFromEnvRegistry(reg *motan.URL) error {
	vlog.Infof("direct registry is found from env, will replace registry with %+v, cluster:%s", reg, m.url.GetIdentity())
	registry := m.extFactory.GetRegistry(reg)
	if registry == nil {
		return errors.New("env direct registry is nil. reg url: " + reg.GetIdentity())
	}
	registries := make([]motan.Registry, 0, 1)
	m.Registries = append(registries, registry)
	urls := registry.Discover(m.url)
	m.Notify(reg, urls)
	return nil
}

func (m *MotanCluster) initFilters() {
	clusterFilter, endpointFilters := motan.GetURLFilters(m.url, m.extFactory)
	if clusterFilter != nil {
		m.clusterFilter = clusterFilter
	}
	if len(endpointFilters) > 0 {
		m.Filters = endpointFilters
	}
	vlog.Infof("MotanCluster init filter. url:%+v, cluster filter:%#v, ep filter size:%d, ep filters:%#v", m.GetURL(), m.clusterFilter, len(m.Filters), m.Filters)
}

func (m *MotanCluster) NotifyAgentCommand(commandInfo string) {
	for _, reg := range m.Registries {
		if notifyRegistry, ok := reg.(motan.CommandNotifyListener); ok {
			notifyRegistry.NotifyCommand(m.url, AgentCmd, commandInfo)
		}
	}
}

const (
	clusterIdcPlaceHolder = "${idc}"
)

func getSubscribeGroup(url *motan.URL, registry motan.Registry) string {
	if strings.Index(url.Group, clusterIdcPlaceHolder) == -1 {
		return url.Group
	}

	// prepare groups for exact match
	preDetectGroups := make([]string, 0, 3)
	if *motan.IDC != "" {
		originGroup := strings.Replace(url.Group, clusterIdcPlaceHolder, *motan.IDC, 1)
		lowerGroup := strings.Replace(url.Group, clusterIdcPlaceHolder, strings.ToLower(*motan.IDC), 1)
		upperGroup := strings.Replace(url.Group, clusterIdcPlaceHolder, strings.ToUpper(*motan.IDC), 1)
		preDetectGroups = append(preDetectGroups, originGroup)
		appendPreDetectGroups := func(group string) {
			// avoid duplicate group
			for _, g := range preDetectGroups {
				if group == g {
					return
				}
			}
			preDetectGroups = append(preDetectGroups, group)
		}
		appendPreDetectGroups(lowerGroup)
		appendPreDetectGroups(upperGroup)
	}

	groupDetectURL := url.Copy()
	gr, canDiscoverGroup := registry.(motan.GroupDiscoverableRegistry)
	if !canDiscoverGroup {
		// registry can not discover groups, we just try prepared groups
		for _, g := range preDetectGroups {
			groupDetectURL.Group = g
			if len(registry.Discover(groupDetectURL)) > 0 {
				return g
			}
		}
		return url.Group
	}

	// registry can discover groups, first try prepared groups, if no match, try regex match
	groups := motan.GetAllGroups(gr)
	for _, g := range preDetectGroups {
		for _, group := range groups {
			groupDetectURL.Group = g
			if group == g && len(registry.Discover(groupDetectURL)) > 0 {
				return g
			}
		}
	}

	regex, err := regexp.Compile("^" + strings.Replace(url.Group, clusterIdcPlaceHolder, "(.*)", 1) + "$")
	if err != nil {
		vlog.Errorf("Unexpected url config %v", url)
		return url.Group
	}

	groupNodes := make(map[string][]*motan.URL)
	matchedGroups := make([]string, 0, 3)
	for _, group := range groups {
		if regex.Match([]byte(group)) {
			matchedGroups = append(matchedGroups, group)
			groupDetectURL.Group = group
			nodes := registry.Discover(groupDetectURL)
			if len(nodes) > 0 {
				groupNodes[group] = nodes
			}
		}
	}
	if len(groupNodes) != 0 {
		return getBestGroup(groupNodes)
	}
	if len(matchedGroups) == 0 {
		vlog.Errorf("No group found for %s", url.Group)
		return url.Group
	}
	group := matchedGroups[len(matchedGroups)-1]
	vlog.Warningf("Get all group nodes for %s failed, use a fallback group: %s", url.Group, group)
	return group
}

func getBestGroup(groupNodes map[string][]*motan.URL) string {
	var lastGroup string
	groupRtt := make(map[string]time.Duration, len(groupNodes))
	// TODO: if we do not have root privilege on linux, we need another policy to get network latency
	// For linux we need root privilege to do ping, but darwin no need
	pingPrivileged := true
	if runtime.GOOS == "darwin" {
		pingPrivileged = false
	}
	for group, nodes := range groupNodes {
		lastGroup = group
		pinger, err := motan.NewPinger(nodes[rand.Intn(len(nodes))].Host, 5, 1*time.Second, 1024, pingPrivileged)
		if err != nil {
			continue
		}
		pinger.Ping()
		if len(pinger.Rtts) < 3 {
			// node loss packets
			continue
		}

		var min, max, total time.Duration
		min = pinger.Rtts[0]
		max = pinger.Rtts[0]
		for _, rtt := range pinger.Rtts {
			if rtt < min {
				min = rtt
			}
			if rtt > max {
				max = rtt
			}
			total += rtt
		}
		groupRtt[group] = time.Duration(int64(total-min-max) / int64(len(pinger.Rtts)-2))
	}

	var minRtt time.Duration
	var bestGroup string
	for group, rtt := range groupRtt {
		if minRtt == 0 || minRtt > rtt {
			// First rtt group or the rtt is less than former
			minRtt = rtt
			bestGroup = group
		}
	}
	if bestGroup == "" {
		vlog.Warningf("Detect best group failed, use fallback group %s", lastGroup)
		bestGroup = lastGroup
	}
	return bestGroup
}
