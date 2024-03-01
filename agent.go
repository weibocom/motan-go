package motan

import (
	"flag"
	"fmt"
	"github.com/weibocom/motan-go/endpoint"
	vlog "github.com/weibocom/motan-go/log"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/valyala/fasthttp"
	"github.com/weibocom/motan-go/cluster"
	cfg "github.com/weibocom/motan-go/config"
	motan "github.com/weibocom/motan-go/core"
	mhttp "github.com/weibocom/motan-go/http"
	"github.com/weibocom/motan-go/metrics"
	mpro "github.com/weibocom/motan-go/protocol"
	"github.com/weibocom/motan-go/registry"
	mserver "github.com/weibocom/motan-go/server"
)

const (
	defaultPort             = 9981
	defaultReverseProxyPort = 9982
	defaultHTTPProxyPort    = 9983
	defaultManagementPort   = 8002
	defaultPidFile          = "./agent.pid"
	defaultAgentGroup       = "default_agent_group"
	defaultRuntimeDir       = "./agent_runtime"
	defaultStatusSnap       = "status"
)

var (
	initParamLock             sync.Mutex
	setAgentLock              sync.Mutex
	notFoundProviderCount     int64 = 0
	defaultInitClusterTimeout int64 = 10000 //ms
)

type Agent struct {
	ConfigFile   string
	extFactory   motan.ExtensionFactory
	Context      *motan.Context
	onAfterStart []func(a *Agent)
	recover      bool

	agentServer motan.Server

	clusterMap     *motan.CopyOnWriteMap
	httpClusterMap *motan.CopyOnWriteMap
	status         int64
	agentURL       *motan.URL
	logdir         string
	port           int
	mport          int
	eport          int
	hport          int
	pidfile        string
	runtimedir     string

	serviceExporters  *motan.CopyOnWriteMap
	serviceMap        *motan.CopyOnWriteMap
	agentPortServer   map[int]motan.Server
	serviceRegistries *motan.CopyOnWriteMap
	httpProxyServer   *mserver.HTTPProxyServer

	manageHandlers map[string]http.Handler
	envHandlers    map[string]map[string]http.Handler

	svcLock      sync.Mutex
	clsLock      sync.Mutex
	registryLock sync.Mutex

	configurer *DynamicConfigurer

	commandHandlers []CommandHandler
}

type CommandHandler interface {
	CanServe(a *Agent, l *AgentListener, registryURL *motan.URL, cmdInfo string) bool
	Serve() (currentCommandInfo string)
}

type serviceMapItem struct {
	url     *motan.URL
	cluster *cluster.MotanCluster
}

func NewAgent(extfactory motan.ExtensionFactory) *Agent {
	var agent *Agent
	if extfactory == nil {
		fmt.Println("agent using default extensionFactory.")
		agent = &Agent{extFactory: GetDefaultExtFactory()}
	} else {
		agent = &Agent{extFactory: extfactory}
	}
	agent.clusterMap = motan.NewCopyOnWriteMap()
	agent.httpClusterMap = motan.NewCopyOnWriteMap()
	agent.serviceExporters = motan.NewCopyOnWriteMap()
	agent.agentPortServer = make(map[int]motan.Server)
	agent.serviceRegistries = motan.NewCopyOnWriteMap()
	agent.manageHandlers = make(map[string]http.Handler)
	agent.envHandlers = make(map[string]map[string]http.Handler)
	agent.serviceMap = motan.NewCopyOnWriteMap()
	return agent
}

func (a *Agent) initProxyServiceURL(url *motan.URL) {
	export := url.GetParam(motan.ExportKey, "")
	url.Protocol, url.Port, _ = motan.ParseExportInfo(export)
	url.Host = motan.GetLocalIP()
	application := url.GetParam(motan.ApplicationKey, "")
	if application == "" {
		application = a.agentURL.GetParam(motan.ApplicationKey, "")
		url.PutParam(motan.ApplicationKey, application)
	}
	url.ClearCachedInfo()
}

func (a *Agent) OnAfterStart(f func(a *Agent)) {
	a.onAfterStart = append(a.onAfterStart, f)
}

func (a *Agent) RegisterCommandHandler(f CommandHandler) {
	a.commandHandlers = append(a.commandHandlers, f)
}

func (a *Agent) GetDynamicRegistryInfo() *RegistrySnapInfoStorage {
	return a.configurer.getRegistryInfo()
}

func (a *Agent) callAfterStart() {
	time.AfterFunc(time.Second*5, func() {
		for _, f := range a.onAfterStart {
			f(a)
		}
	})
}

// RuntimeDir acquires the agent runtime working directory
func (a *Agent) RuntimeDir() string {
	return a.runtimedir
}

// GetAgentServer get Agent server
func (a *Agent) GetAgentServer() motan.Server {
	return a.agentServer
}

func (a *Agent) SetAllServicesAvailable() {
	a.availableAllServices()
	atomic.StoreInt64(&a.status, http.StatusOK)
	a.saveStatus()
}

func (a *Agent) SetAllServicesUnavailable() {
	a.unavailableAllServices()
	atomic.StoreInt64(&a.status, http.StatusServiceUnavailable)
	a.saveStatus()
}

func (a *Agent) StartMotanAgent() {
	a.StartMotanAgentFromConfig(nil)
}

func (a *Agent) StartMotanAgentFromConfig(config *cfg.Config) {
	if !flag.Parsed() {
		flag.Parse()
	}
	a.recover = *motan.Recover

	if config != nil {
		a.Context = &motan.Context{Config: config}
	} else {
		a.Context = &motan.Context{ConfigFile: a.ConfigFile}
	}

	a.Context.Initialize()

	if a.Context.Config == nil {
		fmt.Println("init agent context fail. ConfigFile:", a.Context.ConfigFile)
		return
	}
	fmt.Println("init agent context success.")
	a.initParam()
	a.SetSanpshotConf()
	a.initAgentURL()
	// start metrics reporter early, here agent context has already initialized
	metrics.StartReporter(a.Context)
	a.registerStatusSampler()
	a.initStatus()
	a.initClusters()
	a.startServerAgent()
	a.initHTTPClusters()
	a.startHTTPAgent()
	a.configurer = NewDynamicConfigurer(a)
	go a.startMServer()
	go a.registerAgent()
	go a.startRegistryFailback()
	f, err := os.Create(a.pidfile)
	if err != nil {
		vlog.Errorf("create file %s fail.", a.pidfile)
	} else {
		defer f.Close()
		f.WriteString(strconv.Itoa(os.Getpid()))
	}
	if atomic.LoadInt64(&a.status) == http.StatusOK {
		// recover form a unexpected case
		a.availableAllServices()
	}
	vlog.Infoln("Motan agent is starting...")
	a.startAgent()
}

func (a *Agent) startRegistryFailback() {
	vlog.Infoln("start agent failback")
	ticker := time.NewTicker(registry.DefaultFailbackInterval * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		a.registryLock.Lock()
		a.serviceRegistries.Range(func(k, v interface{}) bool {
			if vv, ok := v.(motan.RegistryStatusManager); ok {
				statusMap := vv.GetRegistryStatus()
				for _, j := range statusMap {
					curStatus := atomic.LoadInt64(&a.status)
					if curStatus == http.StatusOK && j.Status == motan.RegisterFailed {
						vlog.Infoln(fmt.Sprintf("detect agent register fail, do register again, service: %s", j.Service.GetIdentity()))
						v.(motan.Registry).Available(j.Service)
					} else if curStatus == http.StatusServiceUnavailable && j.Status == motan.UnregisterFailed {
						vlog.Infoln(fmt.Sprintf("detect agent unregister fail, do unregister again, service: %s", j.Service.GetIdentity()))
						v.(motan.Registry).Unavailable(j.Service)
					}
				}
			}
			return true
		})
		a.registryLock.Unlock()
	}

}

func (a *Agent) GetRegistryStatus() []map[string]*motan.RegistryStatus {
	a.registryLock.Lock()
	defer a.registryLock.Unlock()
	var res []map[string]*motan.RegistryStatus
	a.serviceRegistries.Range(func(k, v interface{}) bool {
		if vv, ok := v.(motan.RegistryStatusManager); ok {
			statusMap := vv.GetRegistryStatus()
			res = append(res, statusMap)
		}
		return true
	})
	return res
}

func (a *Agent) registerStatusSampler() {
	metrics.RegisterStatusSampleFunc("memory", func() int64 {
		p, _ := process.NewProcess(int32(os.Getpid()))
		memInfo, err := p.MemoryInfo()
		if err != nil {
			return 0
		}
		return int64(memInfo.RSS >> 20)
	})
	metrics.RegisterStatusSampleFunc("cpu", func() int64 {
		p, _ := process.NewProcess(int32(os.Getpid()))
		cpuPercent, err := p.CPUPercent()
		if err != nil {
			return 0
		}
		return int64(cpuPercent)
	})
	metrics.RegisterStatusSampleFunc("goroutine_count", func() int64 {
		return int64(runtime.NumGoroutine())
	})
	metrics.RegisterStatusSampleFunc("not_found_provider_count", func() int64 {
		return atomic.SwapInt64(&notFoundProviderCount, 0)
	})
}

func (a *Agent) initStatus() {
	if a.recover {
		a.recoverStatus()
		// here we add the metrics for recover
		application := a.agentURL.GetParam(motan.ApplicationKey, metrics.DefaultStatApplication)
		keys := []string{metrics.DefaultStatRole, application, "abnormal_exit"}
		metrics.AddCounterWithKeys(metrics.DefaultStatGroup, "", metrics.DefaultStatService,
			keys, ".total_count", 1)
	} else {
		atomic.StoreInt64(&a.status, http.StatusServiceUnavailable)
	}
}

func (a *Agent) saveStatus() {
	statSnapFile := a.runtimedir + string(filepath.Separator) + defaultStatusSnap
	err := ioutil.WriteFile(statSnapFile, []byte(strconv.Itoa(http.StatusOK)), 0644)
	if err != nil {
		vlog.Errorln("Save status error: " + err.Error())
		return
	}
}

func (a *Agent) recoverStatus() {
	a.status = http.StatusServiceUnavailable
	statSnapFile := a.runtimedir + string(filepath.Separator) + defaultStatusSnap
	bytes, err := ioutil.ReadFile(statSnapFile)
	if err != nil {
		vlog.Warningln("Read status snapshot error: " + err.Error())
		return
	}
	code, err := strconv.Atoi(string(bytes))
	if err != nil {
		vlog.Errorln("Convert status code error: " + err.Error())
		return
	}
	if code == http.StatusOK {
		a.status = http.StatusOK
	}
}

func (a *Agent) initParam() {
	initParamLock.Lock()
	defer initParamLock.Unlock()
	section, err := a.Context.Config.GetSection("motan-agent")
	if err != nil {
		fmt.Println("get config of \"motan-agent\" fail! err " + err.Error())
	}
	logDir := ""
	isFound := false
	for _, j := range os.Args {
		if j == "-log_dir" {
			isFound = true
			break
		}
	}
	if isFound {
		logDir = flag.Lookup("log_dir").Value.String()
	} else if section != nil && section["log_dir"] != nil {
		logDir = section["log_dir"].(string)
	}
	if logDir == "" {
		logDir = "."
	}
	initLog(logDir, section)
	registerSwitchers(a.Context)

	processPoolSize := 0
	if section != nil && section["processPoolSize"] != nil {
		processPoolSize = section["processPoolSize"].(int)
	}
	if processPoolSize > 0 {
		mserver.SetProcessPoolSize(processPoolSize)
	}

	port := *motan.Port
	if port == 0 && section != nil && section["port"] != nil {
		port = section["port"].(int)
	}
	if port == 0 {
		port = defaultPort
	}

	mPort := *motan.Mport
	if mPort == 0 {
		if envMPort, ok := os.LookupEnv("mport"); ok {
			if envMPortInt, err := strconv.Atoi(envMPort); err == nil {
				mPort = envMPortInt
			}
		} else if section != nil && section["mport"] != nil {
			mPort = section["mport"].(int)
		}
	}

	if mPort == 0 {
		mPort = defaultManagementPort
	}

	ePort := *motan.Eport
	if ePort == 0 && section != nil && section["eport"] != nil {
		ePort = section["eport"].(int)
	}
	if ePort == 0 {
		ePort = defaultReverseProxyPort
	}

	hPort := *motan.Hport
	if hPort == 0 && section != nil && section["hport"] != nil {
		hPort = section["hport"].(int)
	}
	if hPort == 0 {
		hPort = defaultHTTPProxyPort
	}

	pidFile := *motan.Pidfile
	if pidFile == "" && section != nil && section["pidfile"] != nil {
		pidFile = section["pidfile"].(string)
	}
	if pidFile == "" {
		pidFile = defaultPidFile
	}

	runtimeDir := ""
	if section != nil && section["runtime_dir"] != nil {
		runtimeDir = section["runtime_dir"].(string)
	}
	if runtimeDir == "" {
		runtimeDir = defaultRuntimeDir
	}

	err = os.MkdirAll(runtimeDir, 0775)
	if err != nil {
		panic("Init runtime directory error: " + err.Error())
	}
	asyncInit := true
	if section != nil && section[motan.MotanEpAsyncInit] != nil {
		if ai, ok := section[motan.MotanEpAsyncInit].(bool); ok {
			asyncInit = ai
			vlog.Infof("%s is set to %s", motan.MotanEpAsyncInit, strconv.FormatBool(ai))
		} else {
			vlog.Warningf("illegal %s input, input should be bool", motan.MotanEpAsyncInit)
		}
	}
	endpoint.SetMotanEPDefaultAsynInit(asyncInit)
	vlog.Infof("agent port:%d, manage port:%d, pidfile:%s, logdir:%s, runtimedir:%s", port, mPort, pidFile, logDir, runtimeDir)
	a.logdir = logDir
	a.port = port
	a.eport = ePort
	a.hport = hPort
	a.mport = mPort
	a.pidfile = pidFile
	a.runtimedir = runtimeDir
}

func (a *Agent) initHTTPClusters() {
	for id, url := range a.Context.HTTPClientURLs {
		if application := url.GetParam(motan.ApplicationKey, ""); application == "" {
			url.PutParam(motan.ApplicationKey, a.agentURL.GetParam(motan.ApplicationKey, ""))
		}
		httpCluster := cluster.NewHTTPCluster(url, true, a.Context, a.extFactory)
		if httpCluster == nil {
			vlog.Errorf("Create http cluster %s failed", id)
			continue
		}
		// here the domain has value
		a.httpClusterMap.Store(url.GetParam(mhttp.DomainKey, ""), httpCluster)
	}
}

func (a *Agent) startHTTPAgent() {
	// reuse configuration of agent
	url := a.agentURL.Copy()
	url.Port = a.hport
	a.httpProxyServer = mserver.NewHTTPProxyServer(url)
	a.httpProxyServer.Open(false, true, &httpClusterGetter{a: a}, &agentMessageHandler{agent: a})
	vlog.Infof("Start http forward proxy server on port %d", a.hport)
}

type httpClusterGetter struct {
	a *Agent
}

func (h *httpClusterGetter) GetHTTPCluster(host string) *cluster.HTTPCluster {
	if c, ok := h.a.httpClusterMap.Load(host); ok {
		return c.(*cluster.HTTPCluster)
	}
	return nil
}

func (a *Agent) reloadClusters(ctx *motan.Context) {
	a.clsLock.Lock()
	defer a.clsLock.Unlock()

	a.Context = ctx

	serviceItemKeep := make(map[string]bool)
	clusterMap := make(map[interface{}]interface{})
	serviceMap := make(map[interface{}]interface{})
	var allRefersURLs []*motan.URL
	if a.configurer != nil {
		//keep all dynamic refers
		for _, url := range a.configurer.subscribeNodes {
			allRefersURLs = append(allRefersURLs, url)
		}
	}
	for _, v := range a.Context.RefersURLs {
		allRefersURLs = append(allRefersURLs, v)
	}
	for _, url := range allRefersURLs {
		if url.Parameters[motan.ApplicationKey] == "" {
			url.Parameters[motan.ApplicationKey] = a.agentURL.Parameters[motan.ApplicationKey]
		}

		service := url.Path
		mapKey := getClusterKey(url.Group, url.GetStringParamsWithDefault(motan.VersionKey, motan.DefaultReferVersion), url.Protocol, url.Path)

		// find exists old serviceMap
		var serviceMapValue serviceMapItem
		if v, exists := a.serviceMap.Load(service); exists {
			vItems := v.([]serviceMapItem)

			for _, vItem := range vItems {
				urlExtInfo := url.ToExtInfo()
				if urlExtInfo == vItem.url.ToExtInfo() {
					serviceItemKeep[urlExtInfo] = true
					serviceMapValue = vItem
					break
				}
			}
		}

		// new serviceMap & cluster
		if serviceMapValue.url == nil {
			vlog.Infoln("hot create service:" + url.ToExtInfo())
			c := cluster.NewCluster(a.Context, a.extFactory, url, true)
			serviceMapValue = serviceMapItem{
				url:     url,
				cluster: c,
			}
		}
		clusterMap[mapKey] = serviceMapValue.cluster

		var serviceMapItemArr []serviceMapItem
		if v, exists := serviceMap[service]; exists {
			serviceMapItemArr = v.([]serviceMapItem)
		}
		serviceMapItemArr = append(serviceMapItemArr, serviceMapValue)
		serviceMap[url.Path] = serviceMapItemArr
	}

	oldServiceMap := a.serviceMap.Swap(serviceMap)
	a.clusterMap.Swap(clusterMap)

	// diff and destroy service
	for _, v := range oldServiceMap {
		vItems := v.([]serviceMapItem)
		for _, item := range vItems {
			if _, ok := serviceItemKeep[item.url.ToExtInfo()]; !ok {
				vlog.Infoln("hot destroy service:" + item.url.ToExtInfo())
				item.cluster.Destroy()
			}
		}
	}
}

func (a *Agent) initClusters() {
	initTimeout := a.Context.AgentURL.GetIntValue(motan.InitClusterTimeoutKey, defaultInitClusterTimeout)
	timer := time.NewTimer(time.Millisecond * time.Duration(initTimeout))
	wg := sync.WaitGroup{}
	wg.Add(len(a.Context.RefersURLs))
	for _, url := range a.Context.RefersURLs {
		// concurrently initialize cluster
		go func(u *motan.URL) {
			defer wg.Done()
			defer motan.HandlePanic(nil)
			a.initCluster(u)
		}(url)
	}
	finishChan := make(chan struct{})
	go func() {
		wg.Wait()
		finishChan <- struct{}{}
	}()
	select {
	case <-timer.C:
		vlog.Infof("agent init cluster timeout(%dms), do not wait(rest cluster keep doing initialization backend)", initTimeout)
	case <-finishChan:
		defer timer.Stop()
		vlog.Infoln("agent cluster init complete")
	}
}

func (a *Agent) initCluster(url *motan.URL) {
	if url.Parameters[motan.ApplicationKey] == "" {
		url.Parameters[motan.ApplicationKey] = a.agentURL.Parameters[motan.ApplicationKey]
	}

	c := cluster.NewCluster(a.Context, a.extFactory, url, true)
	item := serviceMapItem{
		url:     url,
		cluster: c,
	}
	service := url.Path
	a.serviceMap.SafeDoFunc(func() {
		var serviceMapItemArr []serviceMapItem
		if v, exists := a.serviceMap.Load(service); exists {
			serviceMapItemArr = v.([]serviceMapItem)
			serviceMapItemArr = append(serviceMapItemArr, item)
		} else {
			serviceMapItemArr = []serviceMapItem{item}
		}
		a.serviceMap.UnsafeStore(url.Path, serviceMapItemArr)
	})
	mapKey := getClusterKey(url.Group, url.GetStringParamsWithDefault(motan.VersionKey, motan.DefaultReferVersion), url.Protocol, url.Path)
	a.clsLock.Lock() // Mutually exclusive with the reloadClusters method
	defer a.clsLock.Unlock()
	a.clusterMap.Store(mapKey, c)
}

func (a *Agent) SetSanpshotConf() {
	section, err := a.Context.Config.GetSection("motan-agent")
	if err != nil {
		vlog.Infoln("get config of \"motan-agent\" fail! err " + err.Error())
	}
	var snapshotDir string
	if section != nil && section["snapshot_dir"] != nil {
		snapshotDir = section["snapshot_dir"].(string)
	}
	if snapshotDir == "" {
		snapshotDir = registry.DefaultSnapshotDir
	}
	registry.SetSnapshotConf(registry.DefaultSnapshotInterval, snapshotDir)
}

func (a *Agent) initAgentURL() {
	agentURL := a.Context.AgentURL
	if application, ok := agentURL.Parameters[motan.ApplicationKey]; ok {
		agentURL.Group = application // agent's application is same with agent group.
	} else {
		agentURL.Parameters[motan.ApplicationKey] = agentURL.Group
	}
	if agentURL.Group == "" {
		agentURL.Group = defaultAgentGroup
		agentURL.Parameters[motan.ApplicationKey] = defaultAgentGroup
	}
	if agentURL.Path == "" {
		agentURL.Path = agentURL.Group
	}

	if mportstr, ok := agentURL.Parameters["mport"]; ok {
		mport, err := strconv.Atoi(mportstr)
		if err == nil {
			agentURL.Port = mport
		}
	}

	agentURL.Parameters[motan.NodeTypeKey] = "agent"
	a.agentURL = agentURL
	vlog.Infof("Agent URL inited %s", a.agentURL.GetIdentity())
}

func (a *Agent) startAgent() {
	url := a.agentURL.Copy()
	url.Port = a.port
	handler := &agentMessageHandler{agent: a}
	server := &mserver.MotanServer{URL: url}
	server.SetMessageHandler(handler)
	vlog.Infof("Motan agent is started. port:%d", a.port)
	fmt.Println("Motan agent start.")
	a.agentServer = server
	a.callAfterStart()
	err := server.Open(true, true, handler, a.extFactory)
	if err != nil {
		vlog.Fatalf("start agent fail. port :%d, err: %v", a.port, err)
	}
	fmt.Println("Motan agent start fail!")
}

func (a *Agent) registerAgent() {
	vlog.Infoln("start agent registry.")
	if reg, exit := a.agentURL.Parameters[motan.RegistryKey]; exit {
		agentURL := a.agentURL.Copy()
		if agentURL.Host == "" {
			agentURL.Host = motan.GetLocalIP()
		}
		if registryURL, regexit := a.Context.RegistryURLs[reg]; regexit {
			registry := a.extFactory.GetRegistry(registryURL)
			if registry != nil {
				vlog.Infof("agent register in registry:%s, agent url:%s", registry.GetURL().GetIdentity(), agentURL.GetIdentity())
				registry.Register(agentURL)
				//TODO 503, heartbeat
				if commandRegisry, ok := registry.(motan.DiscoverCommand); ok {
					listener := &AgentListener{agent: a}
					commandRegisry.SubscribeCommand(agentURL, listener)
					commandInfo := commandRegisry.DiscoverCommand(agentURL)
					listener.NotifyCommand(registryURL, cluster.AgentCmd, commandInfo)
					vlog.Infof("agent subscribe command. init command: %s", commandInfo)
				}
			}
		} else {
			vlog.Warningf("can not find agent registry in conf, so do not register. agent url:%s", agentURL.GetIdentity())
		}
	}
}

type agentMessageHandler struct {
	agent *Agent
}

func (a *agentMessageHandler) clusterCall(request motan.Request, ck string, motanCluster *cluster.MotanCluster) (res motan.Response) {
	// fill default request info
	fillDefaultReqInfo(request, motanCluster.GetURL())
	res = motanCluster.Call(request)
	if res == nil {
		vlog.Warningf("motanCluster Call return nil. cluster:%s", ck)
		res = getDefaultResponse(request.GetRequestID(), "motanCluster Call return nil. cluster:"+ck)
	}
	return res
}

func (a *agentMessageHandler) httpCall(request motan.Request, ck string, httpCluster *cluster.HTTPCluster) (res motan.Response) {
	start := time.Now()
	originalService := request.GetServiceName()
	useHTTP := false
	defer func() {
		if useHTTP {
			// TODO: here we just record the request use http, rpc request has its own access log,
			//       maybe we should record it at one space
			vlog.Infof("http-rpc %s,%s,%d,%d,%t,%v",
				originalService, request.GetMethod(), request.GetRequestID(), time.Since(start)/1000000,
				res.GetException() == nil, res.GetException())
		}
	}()
	if service, ok := httpCluster.CanServe(request.GetMethod()); ok {
		// if the client can use rpc, do it
		fillDefaultReqInfo(request, httpCluster.GetURL())
		request.SetAttachment(mpro.MPath, service)
		if motanRequest, ok := request.(*motan.MotanRequest); ok {
			motanRequest.ServiceName = service
		}
		res = httpCluster.Call(request)
		if res == nil {
			vlog.Warningf("httpCluster Call return nil. cluster:%s", ck)
			return getDefaultResponse(request.GetRequestID(), "httpCluster Call return nil. cluster:"+ck)
		}
	}
	// has response and response not a no endpoint exception
	// here nil res represent http cluster can not serve this method
	if res != nil && (res.GetException() == nil || res.GetException().ErrCode != motan.ENoEndpoints) {
		return res
	}
	// no rpc service or rpc with no endpoints
	useHTTP = true
	err := request.ProcessDeserializable(nil)
	if err != nil {
		return getDefaultResponse(request.GetRequestID(), "bad request: "+err.Error())
	}
	httpRequest := fasthttp.AcquireRequest()
	httpResponse := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(httpRequest)
	defer fasthttp.ReleaseResponse(httpResponse)
	httpRequest.Header.Del("Host")
	httpRequest.SetHost(originalService)
	httpRequest.URI().SetPath(request.GetMethod())
	err = mhttp.MotanRequestToFasthttpRequest(request, httpRequest, "GET")
	if err != nil {
		return getDefaultResponse(request.GetRequestID(), "bad motan-http request: "+err.Error())
	}
	err = a.agent.httpProxyServer.GetHTTPClient().Do(httpRequest, httpResponse)
	if err != nil {
		return getDefaultResponse(request.GetRequestID(), "do http request failed : "+err.Error())
	}
	httpMotanResp := mhttp.AcquireHttpMotanResponse()
	httpMotanResp.RequestID = request.GetRequestID()
	res = httpMotanResp
	mhttp.FasthttpResponseToMotanResponse(res, httpResponse)
	return res
}

// fill default reqeust info such as application, group..
func fillDefaultReqInfo(r motan.Request, url *motan.URL) {
	if r.GetRPCContext(true).IsMotanV1 {
		if r.GetAttachment(motan.ApplicationKey) == "" {
			application := url.GetParam(motan.ApplicationKey, "")
			if application != "" {
				r.SetAttachment(motan.ApplicationKey, application)
			}
		}
		if r.GetAttachment(motan.GroupKey) == "" {
			r.SetAttachment(motan.GroupKey, url.Group)
		}
	} else {
		if r.GetAttachment(mpro.MSource) == "" {
			if app := r.GetAttachment(motan.ApplicationKey); app != "" {
				r.SetAttachment(mpro.MSource, app)
			} else {
				application := url.GetParam(motan.ApplicationKey, "")
				if application != "" {
					r.SetAttachment(mpro.MSource, application)
				}
			}
		}
		if r.GetAttachment(mpro.MGroup) == "" {
			r.SetAttachment(mpro.MGroup, url.Group)
		}
	}
}

func (a *agentMessageHandler) Call(request motan.Request) (res motan.Response) {
	c, ck, err := a.findCluster(request)
	if err == nil {
		res = a.clusterCall(request, ck, c)
	} else if httpCluster := a.agent.httpClusterMap.LoadOrNil(request.GetServiceName()); httpCluster != nil {
		// if normal cluster not found we try http cluster, here service of request represent domain
		res = a.httpCall(request, ck, httpCluster.(*cluster.HTTPCluster))
	} else {
		vlog.Warningf("cluster not found. cluster: %s, request id:%d", err.Error(), request.GetRequestID())
		res = getDefaultResponse(request.GetRequestID(), "cluster not found. cluster: "+err.Error())
	}
	if res.GetRPCContext(true).RemoteAddr != "" { // set response remote addr
		res.SetAttachment(motan.XForwardedForLower, res.GetRPCContext(true).RemoteAddr)
	}
	return res
}

func (a *agentMessageHandler) findCluster(request motan.Request) (c *cluster.MotanCluster, key string, err error) {
	service := request.GetServiceName()
	if service == "" {
		err = fmt.Errorf("empty service is not supported. service: %s", service)
		return
	}
	serviceItemArrI, exists := a.agent.serviceMap.Load(service)
	if !exists {
		err = fmt.Errorf("cluster not found. service: %s", service)
		return
	}
	clusters := serviceItemArrI.([]serviceMapItem)
	if len(clusters) == 1 {
		//TODO: add strict mode to avoid incorrect group call
		c = clusters[0].cluster
		return
	}
	group := request.GetAttachment(mpro.MGroup)
	if group == "" {
		err = fmt.Errorf("multiple clusters are matched with service: %s, but the group is empty", service)
		return
	}
	version := request.GetAttachment(mpro.MVersion)
	protocol := request.GetAttachment(mpro.MProxyProtocol)
	for _, j := range clusters {
		if j.url.IsMatch(service, group, protocol, version) {
			c = j.cluster
			return
		}
	}
	err = fmt.Errorf("no cluster matches the request; info: {service: %s, group: %s, protocol: %s, version: %s}", service, group, protocol, version)
	return
}

func (a *agentMessageHandler) AddProvider(p motan.Provider) error {
	return nil
}

func (a *agentMessageHandler) RmProvider(p motan.Provider) {}

func (a *agentMessageHandler) GetProvider(serviceName string) motan.Provider {
	return nil
}

func (a *Agent) startServerAgent() {
	globalContext := a.Context
	for _, url := range globalContext.ServiceURLs {
		a.initProxyServiceURL(url)
		a.doExportService(url)
	}
}

func (a *Agent) availableAllServices() {
	a.registryLock.Lock()
	defer a.registryLock.Unlock()
	a.serviceRegistries.Range(func(k, v interface{}) bool {
		v.(motan.Registry).Available(nil)
		return true
	})
}

func (a *Agent) unavailableAllServices() {
	a.registryLock.Lock()
	defer a.registryLock.Unlock()
	a.serviceRegistries.Range(func(k, v interface{}) bool {
		v.(motan.Registry).Unavailable(nil)
		return true
	})
}

func (a *Agent) doExportService(url *motan.URL) {
	a.svcLock.Lock()
	defer a.svcLock.Unlock()

	globalContext := a.Context
	exporter := &mserver.DefaultExporter{}
	provider := a.extFactory.GetProvider(url)
	if provider == nil {
		vlog.Errorf("Didn't have a %s provider, url:%+v", url.Protocol, url)
		return
	}
	motan.CanSetContext(provider, globalContext)
	motan.Initialize(provider)
	provider = mserver.WrapWithFilter(provider, a.extFactory, globalContext)
	exporter.SetProvider(provider)
	server := a.agentPortServer[url.Port]
	if server == nil {
		server = a.extFactory.GetServer(url)
		handler := &serverAgentMessageHandler{}
		motan.Initialize(handler)
		handler.AddProvider(provider)
		err := server.Open(false, true, handler, a.extFactory)
		if err != nil {
			vlog.Fatalf("start server agent fail. port :%d, err: %v", url.Port, err)
		}
		a.agentPortServer[url.Port] = server
	} else if canShareChannel(*url, *server.GetURL()) {
		server.GetMessageHandler().AddProvider(provider)
	} else {
		vlog.Errorf("service can't find a share channel , url:%v", url)
		return
	}
	err := exporter.Export(server, a.extFactory, globalContext)
	if err != nil {
		vlog.Errorf("service export fail! url:%v, err:%v", url, err)
		return
	}

	a.serviceExporters.Store(url.GetIdentityWithRegistry(), exporter)
	vlog.Infof("service export success. url:%v", url)
	for _, r := range exporter.Registries {
		rid := r.GetURL().GetIdentityWithRegistry()
		if _, ok := a.serviceRegistries.Load(rid); !ok {
			a.serviceRegistries.Store(rid, r)
		}
	}
}

type serverAgentMessageHandler struct {
	providers *motan.CopyOnWriteMap
}

func (sa *serverAgentMessageHandler) Initialize() {
	sa.providers = motan.NewCopyOnWriteMap()
}

func getServiceKey(group, path string) string {
	return group + "_" + path
}

func (sa *serverAgentMessageHandler) Call(request motan.Request) (res motan.Response) {
	defer motan.HandlePanic(func() {
		res = motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "provider call panic", ErrType: motan.ServiceException})
		vlog.Errorf("provider call panic. req:%s", motan.GetReqInfo(request))
	})
	// todo: add GetGroup() method in Request
	group := request.GetAttachment(mpro.MGroup)
	if group == "" { // compatible with motan v1
		group = request.GetAttachment(motan.GroupKey)
	}
	serviceKey := getServiceKey(group, request.GetServiceName())
	if p := sa.providers.LoadOrNil(serviceKey); p != nil {
		p := p.(motan.Provider)
		res = p.Call(request)
		res.GetRPCContext(true).GzipSize = int(p.GetURL().GetIntValue(motan.GzipSizeKey, 0))
		return res
	}
	vlog.Errorf("not found provider for %s", motan.GetReqInfo(request))
	atomic.AddInt64(&notFoundProviderCount, 1)
	return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "not found provider for " + serviceKey, ErrType: motan.ServiceException})
}

func (sa *serverAgentMessageHandler) AddProvider(p motan.Provider) error {
	sa.providers.Store(getServiceKey(p.GetURL().Group, p.GetPath()), p)
	return nil
}

func (sa *serverAgentMessageHandler) RmProvider(p motan.Provider) {
	sa.providers.Delete(getServiceKey(p.GetURL().Group, p.GetPath()))
}

func (sa *serverAgentMessageHandler) GetProvider(serviceName string) motan.Provider {
	return nil
}

func getClusterKey(group, version, protocol, path string) string {
	// TODO: remove when cedrus is all instead
	if protocol == "cedrus" {
		protocol = "motan2"
	}
	return group + "_" + version + "_" + protocol + "_" + path
}

func initLog(logDir string, section map[interface{}]interface{}) {
	if section != nil && section["log_async"] != nil {
		logAsync := strconv.FormatBool(section["log_async"].(bool))
		if logAsync != "" {
			_ = flag.Set("log_async", logAsync)
		}
	}

	if section != nil && section["log_structured"] != nil {
		logStructured := strconv.FormatBool(section["log_structured"].(bool))
		if logStructured != "" {
			_ = flag.Set("log_structured", logStructured)
		}
	}
	if section != nil && section["rotate_per_hour"] != nil {
		rotatePerHour := strconv.FormatBool(section["rotate_per_hour"].(bool))
		if rotatePerHour != "" {
			_ = flag.Set("rotate_per_hour", rotatePerHour)
		}
	}
	if section != nil && section["log_level"] != nil {
		logLevel := section["log_level"].(string)
		if logLevel != "" {
			_ = flag.Set("log_level", logLevel)
		}
	}
	if section != nil && section["log_filter_caller"] != nil {
		logFilterCaller := strconv.FormatBool(section["log_filter_caller"].(bool))
		if logFilterCaller != "" {
			_ = flag.Set("log_filter_caller", logFilterCaller)
		}
	}
	if section != nil && section["log_buffer_size"] != nil {
		logBufferSize := strconv.Itoa(section["log_buffer_size"].(int))
		if logBufferSize != "" {
			_ = flag.Set("log_buffer_size", logBufferSize)
		}
	}
	if logDir == "stdout" {
		return
	}
	fmt.Printf("use log dir:%s\n", logDir)
	_ = flag.Set("log_dir", logDir)
	vlog.LogInit(nil)
}

func registerSwitchers(c *motan.Context) {
	switchers, _ := c.Config.GetSection(motan.SwitcherSection)
	s := motan.GetSwitcherManager()
	for n, v := range switchers {
		s.Register(n.(string), v.(bool))
	}
}

func getDefaultResponse(requestid uint64, errmsg string) *motan.MotanResponse {
	return motan.BuildExceptionResponse(requestid, &motan.Exception{ErrCode: 400, ErrMsg: errmsg, ErrType: motan.ServiceException})
}

type AgentListener struct {
	agent              *Agent
	CurrentCommandInfo string
	processStatus      bool //is command process finish
}

func (a *AgentListener) NotifyCommand(registryURL *motan.URL, commandType int, commandInfo string) {
	vlog.Infof("agentlistener command notify:%s", commandInfo)
	// command repeated, just ignore it.

	if commandInfo == a.CurrentCommandInfo {
		vlog.Infof("agentlistener ignore repeated command notify:%s", commandInfo)
		return
	}
	defer motan.HandlePanic(nil)
	defer func() {
		a.CurrentCommandInfo = commandInfo
	}()
	if len(a.agent.commandHandlers) > 0 {
		canServeExists := false
		// there are defined some command handlers
		for _, h := range a.agent.commandHandlers {
			if ok := h.CanServe(a.agent, a, registryURL, commandInfo); ok {
				canServeExists = true
				commandInfo = h.Serve()
			}
		}
		// there are handlers had processed the command, so we should return here, prevent default processing.
		if canServeExists {
			return
		}
	}

	a.agent.clusterMap.Range(func(k, v interface{}) bool {
		cls := v.(*cluster.MotanCluster)
		for _, registry := range cls.Registries {
			if cr, ok := registry.(motan.CommandNotifyListener); ok {
				cr.NotifyCommand(registryURL, cluster.AgentCmd, commandInfo)
			}
		}
		return true
	})
}

func (a *AgentListener) GetIdentity() string {
	return a.agent.agentURL.GetIdentity()
}

func (a *Agent) RegisterManageHandler(path string, handler http.Handler) {
	if path != "" && handler != nil {
		a.manageHandlers[path] = handler // override
	}
}

func (a *Agent) RegisterEnvHandlers(envStr string, handlers map[string]http.Handler) {
	if envStr != "" && handlers != nil {
		a.envHandlers[envStr] = handlers // override
	}
}

func (a *Agent) startMServer() {
	handlers := make(map[string]http.Handler, 16)
	for k, v := range GetDefaultManageHandlers() {
		handlers[k] = v
	}
	for k, v := range a.manageHandlers {
		handlers[k] = v
	}
	// register env handlers
	extHandelrs := os.Getenv(motan.HandlerEnvironmentName)
	for _, k := range strings.Split(extHandelrs, ",") {
		if v, ok := a.envHandlers[strings.TrimSpace(k)]; ok {
			for kk, vv := range v {
				handlers[kk] = vv
			}

		}
	}
	for k, v := range handlers {
		a.mhandle(k, v)
	}

	var managementListener net.Listener
	if managementUnixSockAddr := a.agentURL.GetParam(motan.ManagementUnixSockKey, ""); managementUnixSockAddr != "" {
		listener, err := motan.ListenUnixSock(managementUnixSockAddr)
		if err != nil {
			vlog.Warningf("start listen manage unix sock fail! err:%s\n", err.Error())
			return
		}
		managementListener = listener
	} else if managementRangePort := a.agentURL.GetParam(motan.ManagementPortRangeKey, ""); managementRangePort != "" {
		startAndPortStr := motan.TrimSplit(managementRangePort, "-")
		if len(startAndPortStr) < 2 {
			vlog.Infof("illegal management port range %s", managementRangePort)
			return
		}
		startPort, err := strconv.ParseInt(startAndPortStr[0], 10, 64)
		if err != nil {
			vlog.Infof("illegal management port range %s with wrong start port %s", managementRangePort, startAndPortStr[0])
			return
		}
		endPort, err := strconv.ParseInt(startAndPortStr[1], 10, 64)
		if err != nil {
			vlog.Infof("illegal management port range %s with wrong end port %s", managementRangePort, startAndPortStr[1])
			return
		}
		for port := int(startPort); port <= int(endPort); port++ {
			listener, err := net.Listen("tcp", ":"+strconv.Itoa(port))
			if err != nil {
				continue
			}
			a.mport = port
			managementListener = motan.TCPKeepAliveListener{TCPListener: listener.(*net.TCPListener)}
			break
		}
		if managementListener == nil {
			vlog.Warningf("start management server failed for port range %s", startAndPortStr)
			return
		}
	} else {
		listener, err := net.Listen("tcp", ":"+strconv.Itoa(a.mport))
		if err != nil {
			vlog.Infof("listen manage port %d failed:%s", a.mport, err.Error())
			return
		}
		managementListener = motan.TCPKeepAliveListener{TCPListener: listener.(*net.TCPListener)}
	}

	vlog.Infof("start listen manage for address: %s", managementListener.Addr().String())
	err := http.Serve(managementListener, nil)
	if err != nil {
		vlog.Warningf("start listen manage port fail! port:%d, err:%s", a.mport, err.Error())
	}
}

func (a *Agent) mhandle(k string, h http.Handler) {
	defer func() {
		if err := recover(); err != nil {
			vlog.Warningf("manageHandler register fail. maybe the pattern '%s' already registered", k)
		}
	}()
	if sa, ok := h.(SetAgent); ok {
		setAgentLock.Lock()
		sa.SetAgent(a)
		setAgentLock.Unlock()
	}
	http.HandleFunc(k, func(w http.ResponseWriter, r *http.Request) {
		if !PermissionCheck(r) {
			w.Write([]byte("need permission!"))
			return
		}
		defer func() {
			if err := recover(); err != nil {
				fmt.Fprintf(w, "process request err: %s\n", err)
			}
		}()
		h.ServeHTTP(w, r)
	})
	vlog.Infof("add manage server handle path:%s", k)
}

// backend server heartbeat downgrade
func (a *Agent) setBackendServerHeartbeat(enable bool) {
	for _, s := range a.agentPortServer {
		s.SetHeartbeat(enable)
	}
}

func (a *Agent) BackendStatusChanged(alive bool) {
	a.setBackendServerHeartbeat(alive)
}

func (a *Agent) getConfigData() []byte {
	data, err := yaml.Marshal(a.Context.Config.GetOriginMap())
	if err != nil {
		return []byte(err.Error())
	}
	return data
}

func urlExist(url *motan.URL, urls map[string]*motan.URL) bool {
	for _, u := range urls {
		if url.GetIdentity() == u.GetIdentity() {
			return true
		}
	}
	return false
}

func (a *Agent) SubscribeService(url *motan.URL) error {
	if urlExist(url, a.Context.RefersURLs) {
		return fmt.Errorf("url exist, ignore subscribe, url: %s", url.GetIdentity())
	}
	a.initCluster(url)
	return nil
}

func (a *Agent) ExportService(url *motan.URL) error {
	if urlExist(url, a.Context.ServiceURLs) {
		return fmt.Errorf("url exist, ignore export. url: %s", url.GetIdentityWithRegistry())
	}
	a.doExportService(url)
	return nil
}

func (a *Agent) UnexportService(url *motan.URL) error {
	if urlExist(url, a.Context.ServiceURLs) {
		return nil
	}

	a.svcLock.Lock()
	defer a.svcLock.Unlock()

	if exporter := a.serviceExporters.Delete(url.GetIdentity()); exporter != nil {
		exporter.(motan.Exporter).Unexport()
	}
	return nil
}
