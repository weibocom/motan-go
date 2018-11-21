package motan

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/weibocom/motan-go/cluster"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	mpro "github.com/weibocom/motan-go/protocol"
	"github.com/weibocom/motan-go/registry"
	mserver "github.com/weibocom/motan-go/server"
	"gopkg.in/yaml.v2"
)

const (
	defaultPort       = 9981
	defaultEport      = 9982
	defaultMport      = 8002
	defaultPidFile    = "./agent.pid"
	defaultAgentGroup = "default_agent_group"
	defaultRuntimeDir = "./agent_runtime"
	defaultStatusSnap = "status"
)

type Agent struct {
	ConfigFile string
	extFactory motan.ExtensionFactory
	Context    *motan.Context
	recover    bool

	agentServer motan.Server

	clustermap *motan.CopyOnWriteMap
	status     int
	agentURL   *motan.URL
	logdir     string
	port       int
	mport      int
	eport      int
	pidfile    string
	runtimedir string

	serviceExporters  *motan.CopyOnWriteMap
	agentPortServer   map[int]motan.Server
	serviceRegistries *motan.CopyOnWriteMap

	manageHandlers map[string]http.Handler

	svcLock sync.Mutex
	clsLock sync.Mutex

	configurer *DynamicConfigurer
}

func NewAgent(extfactory motan.ExtensionFactory) *Agent {
	var agent *Agent
	if extfactory == nil {
		fmt.Println("agent using default extensionFactory.")
		agent = &Agent{extFactory: GetDefaultExtFactory()}
	} else {
		agent = &Agent{extFactory: extfactory}
	}
	agent.clustermap = motan.NewCopyOnWriteMap()
	agent.serviceExporters = motan.NewCopyOnWriteMap()
	agent.agentPortServer = make(map[int]motan.Server)
	agent.serviceRegistries = motan.NewCopyOnWriteMap()
	agent.manageHandlers = make(map[string]http.Handler)
	return agent
}

func (a *Agent) initProxyURL(url *motan.URL) {
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

func (a *Agent) StartMotanAgent() {
	if !flag.Parsed() {
		flag.Parse()
	}
	a.recover = *motan.Recover
	a.Context = &motan.Context{ConfigFile: a.ConfigFile}
	a.Context.Initialize()
	if a.Context.Config == nil {
		fmt.Println("init agent context fail. ConfigFile:", a.Context.ConfigFile)
		return
	}
	fmt.Println("init agent context success.")
	a.initParam()
	a.SetSanpshotConf()
	a.initAgentURL()
	a.initStatus()
	a.initClusters()
	a.startServerAgent()
	a.configurer = NewDynamicConfigurer(a)
	go a.startMServer()
	go a.registerAgent()
	f, err := os.Create(a.pidfile)
	if err != nil {
		vlog.Errorf("create file %s fail.\n", a.pidfile)
	} else {
		defer f.Close()
		f.WriteString(strconv.Itoa(os.Getpid()))
	}
	vlog.Infoln("Motan agent is starting...")
	a.startAgent()
}

func (a *Agent) initStatus() {
	if a.recover {
		a.recoverStatus()
	} else {
		a.status = http.StatusServiceUnavailable
	}
}

func (a *Agent) saveStatus() {
	statSnapFile := a.runtimedir + string(filepath.Separator) + defaultStatusSnap
	err := ioutil.WriteFile(statSnapFile, []byte(strconv.Itoa(int(http.StatusOK))), 0644)
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
	section, err := a.Context.Config.GetSection("motan-agent")
	if err != nil {
		fmt.Println("get config of \"motan-agent\" fail! err " + err.Error())
	}
	logdir := ""
	if section != nil && section["log_dir"] != nil {
		logdir = section["log_dir"].(string)
	}
	if logdir == "" {
		logdir = "."
	}
	initLog(logdir)
	registerSwitchers(a.Context)

	port := *motan.Port
	if port == 0 && section != nil && section["port"] != nil {
		port = section["port"].(int)
	}
	if port == 0 {
		port = defaultPort
	}

	mport := *motan.Mport
	if mport == 0 && section != nil && section["mport"] != nil {
		mport = section["mport"].(int)
	}
	if mport == 0 {
		mport = defaultMport
	}

	eport := *motan.Eport
	if eport == 0 && section != nil && section["eport"] != nil {
		eport = section["eport"].(int)
	}
	if eport == 0 {
		eport = defaultEport
	}

	pidfile := *motan.Pidfile
	if pidfile == "" && section != nil && section["pidfile"] != nil {
		pidfile = section["pidfile"].(string)
	}
	if pidfile == "" {
		pidfile = defaultPidFile
	}

	runtimedir := ""
	if section != nil && section["runtime_dir"] != nil {
		runtimedir = section["runtime_dir"].(string)
	}
	if runtimedir == "" {
		runtimedir = defaultRuntimeDir
	}

	err = os.MkdirAll(runtimedir, 0775)
	if err != nil {
		panic("Init runtime directory error: " + err.Error())
	}

	vlog.Infof("agent port:%d, manage port:%d, pidfile:%s, logdir:%s, runtimedir:%s\n", port, mport, pidfile, logdir, runtimedir)
	a.logdir = logdir
	a.port = port
	a.eport = eport
	a.mport = mport
	a.pidfile = pidfile
	a.runtimedir = runtimedir
}

func (a *Agent) initClusters() {
	for _, url := range a.Context.RefersURLs {
		a.initCluster(url)
	}
}

func (a *Agent) initCluster(url *motan.URL) {
	a.clsLock.Lock()
	defer a.clsLock.Unlock()

	if url.Parameters[motan.ApplicationKey] == "" {
		url.Parameters[motan.ApplicationKey] = a.agentURL.Parameters[motan.ApplicationKey]
	}
	mapKey := getClusterKey(url.Group, url.GetStringParamsWithDefault(motan.VersionKey, "0.1"), url.Protocol, url.Path)
	c := cluster.NewCluster(a.Context, a.extFactory, url, true)
	a.clustermap.Store(mapKey, c)
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
	if agentURL.Host == "" {
		agentURL.Host = motan.GetLocalIP()
	}

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
	vlog.Infof("Agent URL inited %s\n", a.agentURL.GetIdentity())
}

func (a *Agent) startAgent() {
	url := &motan.URL{Port: a.port}
	handler := &agentMessageHandler{agent: a}
	server := &mserver.MotanServer{URL: url}
	server.SetMessageHandler(handler)
	vlog.Infof("Motan agent is started. port:%d\n", a.port)
	fmt.Println("Motan agent start.")
	err := server.Open(true, true, handler, a.extFactory)
	if err != nil {
		vlog.Fatalf("start agent fail. port :%d, err: %v\n", a.port, err)
	}
	a.agentServer = server
	fmt.Println("Motan agent start fail!")
}

func (a *Agent) registerAgent() {
	vlog.Infoln("start agent registry.")
	if reg, exit := a.agentURL.Parameters[motan.RegistryKey]; exit {
		if registryURL, regexit := a.Context.RegistryURLs[reg]; regexit {
			registry := a.extFactory.GetRegistry(registryURL)
			if registry != nil {
				vlog.Infof("agent register in registry:%s, agent url:%s\n", registry.GetURL().GetIdentity(), a.agentURL.GetIdentity())
				registry.Register(a.agentURL)
				//TODO 503, heartbeat
				if commandRegisry, ok := registry.(motan.DiscoverCommand); ok {
					listener := &AgentListener{agent: a}
					commandRegisry.SubscribeCommand(a.agentURL, listener)
					commandInfo := commandRegisry.DiscoverCommand(a.agentURL)
					listener.NotifyCommand(registryURL, cluster.AgentCmd, commandInfo)
					vlog.Infof("agent subscribe command. init command: %s\n", commandInfo)
				}
			}
		} else {
			vlog.Warningf("can not find agent registry in conf, so do not register. agent url:%s\n", a.agentURL.GetIdentity())
		}
	}
}

type agentMessageHandler struct {
	agent *Agent
}

func (a *agentMessageHandler) Call(request motan.Request) (res motan.Response) {
	version := "0.1"
	if request.GetAttachment(mpro.MVersion) != "" {
		version = request.GetAttachment(mpro.MVersion)
	}
	ck := getClusterKey(request.GetAttachment(mpro.MGroup), version, request.GetAttachment(mpro.MProxyProtocol), request.GetAttachment(mpro.MPath))
	if motanCluster := a.agent.clustermap.LoadOrNil(ck); motanCluster != nil {
		motanCluster := motanCluster.(*cluster.MotanCluster)
		if request.GetAttachment(mpro.MSource) == "" {
			application := motanCluster.GetURL().GetParam(motan.ApplicationKey, "")
			if application == "" {
				application = a.agent.agentURL.GetParam(motan.ApplicationKey, "")
			}
			request.SetAttachment(mpro.MSource, application)
		}
		res = motanCluster.Call(request)
		if res == nil {
			vlog.Warningf("motanCluster Call return nil. cluster:%s\n", ck)
			res = getDefaultResponse(request.GetRequestID(), "motanCluster Call return nil. cluster:"+ck)
		}
	} else {
		res = getDefaultResponse(request.GetRequestID(), "cluster not found. cluster:"+ck)
		vlog.Warningf("[Error]cluster not found. cluster: %s, request id:%d\n", ck, request.GetRequestID())
	}
	return res
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
		a.initProxyURL(url)
		a.doExportService(url)
	}
}

func (a *Agent) doExportService(url *motan.URL) {
	a.svcLock.Lock()
	defer a.svcLock.Unlock()

	globalContext := a.Context
	exporter := &mserver.DefaultExporter{}
	provider := a.extFactory.GetProvider(url)
	if provider == nil {
		vlog.Errorf("Didn't have a %s provider, url:%+v\n", url.Protocol, url)
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
			vlog.Fatalf("start server agent fail. port :%d, err: %v\n", url.Port, err)
		}
		a.agentPortServer[url.Port] = server
	} else if canShareChannel(*url, *server.GetURL()) {
		server.GetMessageHandler().AddProvider(provider)
	}
	err := exporter.Export(server, a.extFactory, globalContext)
	if err != nil {
		vlog.Errorf("service export fail! url:%v, err:%v\n", url, err)
		return
	}

	a.serviceExporters.Store(url.GetIdentity(), exporter)
	vlog.Infof("service export success. url:%v\n", url)
	for _, r := range exporter.Registries {
		rid := r.GetURL().GetIdentity()
		if _, ok := a.serviceRegistries.Load(rid); !ok {
			a.serviceRegistries.Store(rid, r)
		}
	}

	if a.status == http.StatusOK {
		exporter.Available()
	}
}

type serverAgentMessageHandler struct {
	providers *motan.CopyOnWriteMap
}

func (sa *serverAgentMessageHandler) Initialize() {
	sa.providers = motan.NewCopyOnWriteMap()
}

func (sa *serverAgentMessageHandler) Call(request motan.Request) (res motan.Response) {
	defer motan.HandlePanic(func() {
		res = motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "provider call panic", ErrType: motan.ServiceException})
		vlog.Errorf("provider call panic. req:%s\n", motan.GetReqInfo(request))
	})
	if p := sa.providers.LoadOrNil(request.GetServiceName()); p != nil {
		p := p.(motan.Provider)
		res = p.Call(request)
		res.GetRPCContext(true).GzipSize = int(p.GetURL().GetIntValue(motan.GzipSizeKey, 0))
		return res
	}
	vlog.Errorf("not found provider for %s\n", motan.GetReqInfo(request))
	return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "not found provider for " + request.GetServiceName(), ErrType: motan.ServiceException})
}

func (sa *serverAgentMessageHandler) AddProvider(p motan.Provider) error {
	sa.providers.Store(p.GetPath(), p)
	return nil
}

func (sa *serverAgentMessageHandler) RmProvider(p motan.Provider) {
	sa.providers.Delete(p.GetPath())
}

func (sa *serverAgentMessageHandler) GetProvider(serviceName string) motan.Provider {
	return nil
}

func getClusterKey(group, version, protocol, path string) string {
	return group + "_" + version + "_" + protocol + "_" + path
}

func initLog(logdir string) {
	fmt.Printf("use log dir:%s\n", logdir)
	flag.Set("log_dir", logdir)
	vlog.FlushInterval = 1 * time.Second
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
	vlog.Infof("agentlistener command notify:%s\n", commandInfo)
	//TODO notify according cluster
	if commandInfo != a.CurrentCommandInfo {
		a.CurrentCommandInfo = commandInfo
		a.agent.clustermap.Range(func(k, v interface{}) bool {
			cls := v.(*cluster.MotanCluster)
			for _, registry := range cls.Registries {
				if cr, ok := registry.(motan.CommandNotifyListener); ok {
					cr.NotifyCommand(registryURL, cluster.AgentCmd, commandInfo)
				}
			}
			return true
		})
	}
}

func (a *AgentListener) GetIdentity() string {
	return a.agent.agentURL.GetIdentity()
}

func (a *Agent) RegisterManageHandler(path string, handler http.Handler) {
	if path != "" && handler != nil {
		a.manageHandlers[path] = handler // override
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
	for k, v := range handlers {
		a.mhandle(k, v)
	}

	vlog.Infof("start listen manage port %d ...\n", a.mport)
	err := http.ListenAndServe(":"+strconv.Itoa(a.mport), nil)
	if err != nil {
		fmt.Printf("start listen manage port fail! port:%d, err:%s\n", a.mport, err.Error())
		vlog.Warningf("start listen manage port fail! port:%d, err:%s\n", a.mport, err.Error())
	}
}

func (a *Agent) mhandle(k string, h http.Handler) {
	defer func() {
		if err := recover(); err != nil {
			vlog.Warningf("manageHandler register fail. maybe the pattern '%s' already registered\n", k)
		}
	}()
	if sa, ok := h.(SetAgent); ok {
		sa.SetAgent(a)
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
	vlog.Infof("add manage server handle path:%s\n", k)
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
		return nil
	}
	a.initCluster(url)
	return nil
}

func (a *Agent) ExportService(url *motan.URL) error {
	if urlExist(url, a.Context.ServiceURLs) {
		return nil
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
