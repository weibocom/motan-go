package cluster

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

type MotanCluster struct {
	context    *motan.Context
	url        *motan.URL
	registries []motan.Registry
	haStrategy motan.HaStrategy

	filters          []motan.Filter
	clusterFilter    motan.ClusterFilter
	extFactory       motan.ExtensionFactory
	lock             sync.Mutex
	isCommandWorking *motan.AtomicBool
	available        bool
	closed           bool
	proxy            bool

	// one cluster can have some registries and groups
	// we can have master group and backup groups
	// the master group has only one, but backup groups can be many
	// when use backup group we need subscribe all groups
	groupEndpoints map[string]*registryEndpointSet
	loadBalance    *multiGroupLoadBalance
	listeners      map[string]*clusterGroupNodeChangeListener
}

//  GroupStrategy means multiple group load balance information
type GroupConfig struct {
	group  string
	weight int64
}

type registryEndpointSet struct {
	endpoints map[string][]motan.EndPoint
}

func newRegistryEndpointSet() *registryEndpointSet {
	return &registryEndpointSet{endpoints: make(map[string][]motan.EndPoint)}
}

func (s *registryEndpointSet) setEndpoints(registry *motan.URL, endpoints []motan.EndPoint) {
	s.endpoints[registry.GetIdentity()] = endpoints
}

func (s *registryEndpointSet) getEndpoints(registry *motan.URL) []motan.EndPoint {
	return s.endpoints[registry.GetIdentity()]
}

func (s *registryEndpointSet) removeEndpoints(registry *motan.URL) {
	delete(s.endpoints, registry.GetIdentity())
}

func (s *registryEndpointSet) getAllEndpoints() []motan.EndPoint {
	var endpoints []motan.EndPoint
	for _, v := range s.endpoints {
		endpoints = append(endpoints, v...)
	}
	return endpoints
}

type multiGroupLoadBalance struct {
	cluster         *MotanCluster
	groupConfigInfo GroupConfigInfo
	balances        map[string]motan.LoadBalance
}

func (lb *multiGroupLoadBalance) OnRefresh(endpoints []motan.EndPoint) {
}

func (lb *multiGroupLoadBalance) Select(request motan.Request) motan.EndPoint {
	groupConfigs := lb.groupConfigInfo.GroupConfigs
	if len(lb.groupConfigInfo.GroupConfigs) == 1 || lb.cluster.isCommandWorking.Get() {
		return lb.balances[groupConfigs[0].group].Select(request)
	}
	if lb.groupConfigInfo.UseBackupGroup {
		for _, group := range groupConfigs {
			if endPoint := lb.balances[group.group].Select(request); endPoint != nil {
				return endPoint
			}
		}
	}
	// TODO: more complex strategy for backup group
	return lb.balances[groupConfigs[rand.Intn(len(groupConfigs))].group].Select(request)
}

func (lb *multiGroupLoadBalance) SelectArray(request motan.Request) []motan.EndPoint {
	groupConfigs := lb.groupConfigInfo.GroupConfigs
	if len(lb.groupConfigInfo.GroupConfigs) == 1 || lb.cluster.isCommandWorking.Get() {
		return lb.balances[groupConfigs[0].group].SelectArray(request)
	}
	if lb.groupConfigInfo.UseBackupGroup {
		for _, group := range groupConfigs {
			if endPoints := lb.balances[group.group].SelectArray(request); len(endPoints) > 0 {
				return endPoints
			}
		}
	}
	// TODO: more complex strategy for backup group
	return lb.balances[groupConfigs[rand.Intn(len(groupConfigs))].group].SelectArray(request)
}

func (lb *multiGroupLoadBalance) SetWeight(weight string) {
}

func (lb *multiGroupLoadBalance) refreshGroup(group string) {
	endpoints := lb.cluster.getEndpointByGroup(group)
	if len(endpoints) > 0 {
		lb.balances[group].OnRefresh(endpoints)
	}
}

// for one group with all registry
type clusterGroupNodeChangeListener struct {
	subscribeURL *motan.URL
	cluster      *MotanCluster
	group        string
}

func (l *clusterGroupNodeChangeListener) GetIdentity() string {
	return l.subscribeURL.GetIdentity()
}

func (l *clusterGroupNodeChangeListener) processWeight(urls []*motan.URL) []*motan.URL {
	weight := ""
	if len(urls) > 0 && urls[len(urls)-1] != nil && urls[len(urls)-1].Protocol == "rule" {
		url := urls[len(urls)-1]
		weight = url.Parameters["weight"]
		urls = urls[:len(urls)-1]
	}
	l.cluster.loadBalance.balances[l.group].SetWeight(weight)
	return urls
}

func (l *clusterGroupNodeChangeListener) Notify(registryURL *motan.URL, urls []*motan.URL) {
	vlog.Infof("cluster %s receive notify size %d for group %s", l.cluster.GetIdentity(), len(urls), l.group)
	l.cluster.lock.Lock()
	defer l.cluster.lock.Unlock()
	if l.cluster.closed {
		return
	}

	masterGroup := l.cluster.loadBalance.groupConfigInfo.GroupConfigs[0].group
	// only master group need processWeight about command
	if masterGroup == l.group {
		urls = l.processWeight(urls)
	}
	// when use backup group, we should make backup groups's endpoint using lazy init
	lazyInit := "false"
	if masterGroup != l.group && l.cluster.loadBalance.groupConfigInfo.UseBackupGroup {
		lazyInit = "true"
	}

	endpoints := make([]motan.EndPoint, 0, len(urls))
	endpointsToRemove := make(map[string]motan.EndPoint)
	eps, ok := l.cluster.groupEndpoints[l.group]
	if !ok {
		vlog.Errorf("cluster %s receive notify with unknown registry %s for group %s", l.cluster.GetIdentity(), registryURL.GetIdentity(), l.group)
		return
	}
	for _, ep := range eps.getEndpoints(registryURL) {
		endpointsToRemove[ep.GetURL().GetIdentity()] = ep
	}

	for _, u := range urls {
		if u == nil {
			vlog.Errorln("cluster receive nil url!")
			continue
		}
		vlog.Infof("cluster %s with group %s received notify url:%s:%d", l.cluster.GetIdentity(), l.group, u.Host, u.Port)
		if !u.CanServe(l.subscribeURL) {
			vlog.Infof("cluster notify:can not use server:%+v", u)
			continue
		}
		var endpoint motan.EndPoint
		endpointID := u.GetIdentity()
		endpoint, ok := endpointsToRemove[endpointID]
		if ok {
			delete(endpointsToRemove, u.GetIdentity())
		}
		if endpoint != nil {
			endpoints = append(endpoints, endpoint)
			continue
		}

		endpointURL := u.Copy()
		endpointURL.MergeParams(l.subscribeURL.Parameters)
		endpointURL.PutParam(motan.LazyInitEndpointKey, lazyInit)
		endpoint = l.cluster.extFactory.GetEndPoint(endpointURL)
		if endpoint == nil {
			continue
		}

		endpoint.SetProxy(l.cluster.proxy)
		serialization := motan.GetSerialization(endpointURL, l.cluster.extFactory)
		if serialization == nil {
			vlog.Warningf("MotanCluster can not find Serialization in DefaultExtensionFactory! url:%+v", l.subscribeURL)
		} else {
			endpoint.SetSerialization(serialization)
		}
		motan.Initialize(endpoint)
		endpoint = l.cluster.addFilter(endpoint, l.cluster.filters)
		endpoints = append(endpoints, endpoint)
	}

	if len(endpoints) == 0 {
		if len(eps.endpoints) > 1 {
			eps.removeEndpoints(registryURL)
		} else {
			vlog.Infof("cluster %s notify endpoint is 0. notify ignored", l.subscribeURL.GetIdentity())
			return
		}
	}
	eps.setEndpoints(registryURL, endpoints)
	l.cluster.loadBalance.refreshGroup(l.group)
	for _, ep := range endpointsToRemove {
		ep.Destroy()
	}
}

func (m *MotanCluster) getEndpointByGroup(group string) []motan.EndPoint {
	if eps, ok := m.groupEndpoints[group]; ok {
		return eps.getAllEndpoints()
	}
	return nil
}

func (m *MotanCluster) IsAvailable() bool {
	return m.available
}

func (m *MotanCluster) Context() *motan.Context {
	return m.context
}

func NewCluster(context *motan.Context, extFactory motan.ExtensionFactory, url *motan.URL, proxy bool) *MotanCluster {
	cluster := &MotanCluster{
		context:    context,
		extFactory: extFactory,
		url:        url,
		proxy:      proxy,
	}
	cluster.isCommandWorking = motan.NewAtomicBool(false)
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

func (m *MotanCluster) initCluster() bool {
	//ha
	m.haStrategy = m.extFactory.GetHa(m.url)
	//lb
	m.loadBalance = &multiGroupLoadBalance{
		cluster:  m,
		balances: make(map[string]motan.LoadBalance),
	}
	m.groupEndpoints = make(map[string]*registryEndpointSet)
	m.listeners = make(map[string]*clusterGroupNodeChangeListener)
	//filter should initialize after HaStrategy
	m.initFilters()

	if m.clusterFilter == nil {
		m.clusterFilter = motan.GetLastClusterFilter()
	}
	if m.filters == nil {
		m.filters = make([]motan.Filter, 0)
	}
	//TODO whether has available refers
	m.available = true
	m.closed = false

	// parse registry and subscribe
	m.parseRegistry()

	vlog.Infof("init MotanCluster %s", m.GetIdentity())

	return true
}

func (m *MotanCluster) SetHaStrategy(haStrategy motan.HaStrategy) {
	m.haStrategy = haStrategy
}

func (m *MotanCluster) AddRegistry(registry motan.Registry) {
	m.registries = append(m.registries, registry)
}

func (m *MotanCluster) Registries() []motan.Registry {
	return m.registries
}

func (m *MotanCluster) addFilter(ep motan.EndPoint, filters []motan.Filter) motan.EndPoint {
	fep := &motan.FilterEndPoint{URL: ep.GetURL(), Caller: ep}
	statusFilters := make([]motan.Status, 0, len(filters))
	var lastFilter motan.EndPointFilter
	lastFilter = motan.GetLastEndPointFilter()
	for _, f := range filters {
		if filter := f.NewFilter(ep.GetURL()); filter != nil {
			if ef, ok := filter.(motan.EndPointFilter); ok {
				motan.CanSetContext(ef, m.context)
				ef.SetNext(lastFilter)
				lastFilter = ef
				if sf, ok := ef.(motan.Status); ok {
					statusFilters = append(statusFilters, sf)
				}
			}
		}
	}
	fep.StatusFilters = statusFilters
	fep.Filter = lastFilter
	return fep
}

func (m *MotanCluster) GetIdentity() string {
	return m.url.GetIdentity()
}

func (m *MotanCluster) Destroy() {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.closed {
		return
	}
	vlog.Infof("cluster %s will destroy.", m.url.GetIdentity())
	for _, r := range m.registries {
		vlog.Infof("unsubscribe from registry %s .", r.GetURL().GetIdentity())
		for _, listener := range m.listeners {
			r.Unsubscribe(listener.subscribeURL, listener)
		}
	}
	for _, registryEndpoints := range m.groupEndpoints {
		for _, endpoints := range registryEndpoints.endpoints {
			for _, e := range endpoints {
				vlog.Infof("destroy endpoint %s .", e.GetURL().GetIdentity())
				e.Destroy()
			}
		}
	}
	m.closed = true
}

func (m *MotanCluster) SetExtFactory(factory motan.ExtensionFactory) {
	m.extFactory = factory
}

func (m *MotanCluster) parseRegistry() (err error) {
	registryIDs, ok := m.url.Parameters[motan.RegistryKey]
	if !ok {
		errInfo := fmt.Sprintf("registry not found for cluster: %+v", m.url)
		err = errors.New(errInfo)
		vlog.Errorln(errInfo)
		panic(errInfo)
	}
	registryIDSet := motan.TrimSplit(registryIDs, ",")
	registries := make([]motan.Registry, 0, len(registryIDSet))
	originRegistries := make([]motan.Registry, 0, len(registryIDSet))
	for _, registryID := range registryIDSet {
		if registryURL, ok := m.context.RegistryURLs[registryID]; ok {
			registry := m.extFactory.GetRegistry(registryURL)
			if registry == nil {
				continue
			}
			originRegistries = append(originRegistries, registry)
			if _, ok := registry.(motan.DiscoverCommand); ok {
				registry = GetCommandRegistryWrapper(m, registry)
			}
			registries = append(registries, registry)
		} else {
			err = errors.New("registry is invalid: " + registryID)
			vlog.Errorln("registry is invalid: " + registryID)
		}
	}
	// no available registry to continue maybe not a good idea
	if len(registries) == 0 {
		panic("no available registry, error: " + err.Error())
	}
	groupConfigInfo := DetermineGroupConfig(m.url, originRegistries)
	groupConfigs := groupConfigInfo.GroupConfigs

	if len(groupConfigs) != 0 {
		m.loadBalance.groupConfigInfo = groupConfigInfo
	} else {
		groupConfigs = []GroupConfig{{group: m.url.Group}}
		m.loadBalance.groupConfigInfo = GroupConfigInfo{GroupConfigs: groupConfigs}
		vlog.Warningf("determineGroup find 0 available group, cluster: %v", m.url)
	}

	m.url.SetGroup(groupConfigs[0].group)
	for groupIndex, groupStrategy := range groupConfigs {
		group := groupStrategy.group
		subscribeURL := m.url.Copy()
		subscribeURL.Group = group
		listener := &clusterGroupNodeChangeListener{
			cluster:      m,
			subscribeURL: subscribeURL,
			group:        groupStrategy.group,
		}
		m.listeners[group] = listener
		m.groupEndpoints[group] = newRegistryEndpointSet()
		m.loadBalance.balances[group] = m.extFactory.GetLB(subscribeURL)
		for _, registry := range registries {
			// only the master group need to subscribe the commands
			if commandRegistry, ok := registry.(*CommandRegistryWrapper); ok {
				if groupIndex > 0 {
					registry = commandRegistry.registry
				}
			}
			registry.Subscribe(subscribeURL, listener)
			urls := registry.Discover(subscribeURL)
			listener.Notify(registry.GetURL(), urls)
		}
	}

	m.registries = registries
	return err
}

func (m *MotanCluster) initFilters() {
	clusterFilter, endpointFilters := motan.GetURLFilters(m.url, m.extFactory)
	if clusterFilter != nil {
		m.clusterFilter = clusterFilter
	}
	if len(endpointFilters) > 0 {
		m.filters = endpointFilters
	}
}

func (m *MotanCluster) NotifyAgentCommand(commandInfo string) {
	for _, reg := range m.registries {
		if notifyRegistry, ok := reg.(motan.CommandNotifyListener); ok {
			notifyRegistry.NotifyCommand(m.url, AgentCmd, commandInfo)
		}
	}
}

func (m *MotanCluster) Call(request motan.Request) (res motan.Response) {
	defer motan.HandlePanic(func() {
		res = motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "cluster call panic", ErrType: motan.ServiceException})
		vlog.Errorf("cluster call panic. req:%s", motan.GetReqInfo(request))
	})
	if m.available {
		return m.clusterFilter.Filter(m.haStrategy, m.loadBalance, request)
	}
	vlog.Infoln("cluster:" + m.GetIdentity() + "is not available!")
	return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "cluster not available, maybe caused by degrade", ErrType: motan.ServiceException})
}

const (
	clusterIdcPlaceHolder = "${idc}"
	backupGroupDelimiter  = ","
)

// DetermineGroupConfig redeclare DetermineGroupConfig
var DetermineGroupConfig = func(url *motan.URL, registries []motan.Registry) GroupConfigInfo {
	registry := registries[0]

	if strings.Contains(url.Group, backupGroupDelimiter) {
		if groupList := strings.Split(url.Group, backupGroupDelimiter); len(groupList) > 0 {
			var groupConfigList []GroupConfig
			for _, group := range groupList {
				groupConfigList = append(groupConfigList, GroupConfig{group: group})
			}
			return GroupConfigInfo{GroupConfigs: groupConfigList, UseBackupGroup: true}
		}
	}

	if strings.Index(url.Group, clusterIdcPlaceHolder) == -1 {
		return GroupConfigInfo{GroupConfigs: []GroupConfig{{group: url.Group}}}
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
				return GroupConfigInfo{GroupConfigs: []GroupConfig{{group: g}}}
			}
		}
		return GroupConfigInfo{GroupConfigs: []GroupConfig{{group: url.Group}}}
	}

	// registry can discover groups, first try prepared groups, if no match, try regex match
	groups := motan.GetAllGroups(gr)
	for _, g := range preDetectGroups {
		for _, group := range groups {
			groupDetectURL.Group = g
			if group == g && len(registry.Discover(groupDetectURL)) > 0 {
				return GroupConfigInfo{GroupConfigs: []GroupConfig{{group: g}}}
			}
		}
	}

	regex, err := regexp.Compile("^" + strings.Replace(url.Group, clusterIdcPlaceHolder, "(.*)", 1) + "$")
	if err != nil {
		vlog.Errorf("Unexpected url config %v", url)
		return GroupConfigInfo{GroupConfigs: []GroupConfig{{group: url.Group}}}
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
		dynamicGroupStrategy := DetermineDynamicGroupStrategy(url, groupNodes)
		groupConfigList := dynamicGroupStrategy.GetGroupConfigs()

		config := GroupConfigInfo{GroupConfigs: groupConfigList}
		sort.Sort(sort.Reverse(&config))
		return config
	}
	if len(matchedGroups) == 0 {
		vlog.Errorf("No group found for %s", url.Group)
		return GroupConfigInfo{GroupConfigs: []GroupConfig{{group: url.Group}}}
	}
	group := matchedGroups[len(matchedGroups)-1]
	vlog.Warningf("Get all group nodes for %s failed, use a fallback group: %s", url.Group, group)
	return GroupConfigInfo{GroupConfigs: []GroupConfig{{group: group}}}
}

type GroupConfigInfo struct {
	GroupConfigs   []GroupConfig
	UseBackupGroup bool
}

func (g *GroupConfigInfo) Len() int { return len(g.GroupConfigs) }
func (g *GroupConfigInfo) Less(i, j int) bool {
	return g.GroupConfigs[i].weight < g.GroupConfigs[j].weight
}
func (g *GroupConfigInfo) Swap(i, j int) {
	g.GroupConfigs[i], g.GroupConfigs[j] = g.GroupConfigs[j], g.GroupConfigs[i]
}

var DetermineDynamicGroupStrategy = func(url *motan.URL, groupNodes map[string][]*motan.URL) DynamicGroup {
	// TODO: multi dynamic group policy support
	return &PingDynamicGroup{groupNodes: groupNodes}
}

type DynamicGroup interface {
	GetGroupConfigs() []GroupConfig
}

type PingDynamicGroup struct {
	name       string
	groupNodes map[string][]*motan.URL
}

func (p *PingDynamicGroup) GetGroupConfigs() []GroupConfig {
	groupConfigList := make([]GroupConfig, 0, len(p.groupNodes))
	groupRtt := make(map[string]int64, len(p.groupNodes))
	// TODO: if we do not have root privilege on linux, we need another policy to get network latency
	// For linux we need root privilege to do ping, but darwin no need
	pingPrivileged := true
	if runtime.GOOS == "darwin" {
		pingPrivileged = false
	}

	var totalRtt int64 = 0
	for group, nodes := range p.groupNodes {
		pinger, err := motan.NewPinger(nodes[rand.Intn(len(nodes))].Host, 5, 1*time.Second, 1024, pingPrivileged)
		if err != nil {
			continue
		}
		pinger.Ping()
		if len(pinger.Rtts) < 3 {
			// node loss packets
			groupRtt[group] = 0
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
		avgRtt := int64(total-min-max) / int64(len(pinger.Rtts)-2)
		totalRtt += avgRtt
		groupRtt[group] = avgRtt
	}

	for group, rtt := range groupRtt {
		weight := totalRtt - rtt
		if rtt == 0 {
			weight = 0
		}
		groupConfigList = append(groupConfigList, GroupConfig{group: group, weight: weight})
	}

	return groupConfigList
}
