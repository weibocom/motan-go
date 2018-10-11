package motan

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/weibocom/motan-go/cluster"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	mpro "github.com/weibocom/motan-go/protocol"
	"github.com/weibocom/motan-go/registry"
	mserver "github.com/weibocom/motan-go/server"
)

const (
	defaultPort       = 9981
	defaultMport      = 8002
	defaultPidFile    = "./agent.pid"
	defaultAgentGroup = "default_agent_group"
)

type Agent struct {
	ConfigFile string
	extFactory motan.ExtensionFactory
	Context    *motan.Context

	agentServer motan.Server

	clustermap map[string]*cluster.MotanCluster
	status     int
	agentURL   *motan.URL
	logdir     string
	port       int
	mport      int
	pidfile    string

	agentPortService  map[int]motan.Exporter
	agentPortServer   map[int]motan.Server
	serviceRegistries map[string]motan.Registry // all registries used for services

	manageHandlers map[string]http.Handler
}

func NewAgent(extfactory motan.ExtensionFactory) *Agent {
	var agent *Agent
	if extfactory == nil {
		fmt.Println("agent using default extensionFactory.")
		agent = &Agent{extFactory: GetDefaultExtFactory()}
	} else {
		agent = &Agent{extFactory: extfactory}
	}
	agent.clustermap = make(map[string]*cluster.MotanCluster)
	agent.agentPortService = make(map[int]motan.Exporter)
	agent.agentPortServer = make(map[int]motan.Server)
	agent.serviceRegistries = make(map[string]motan.Registry)
	agent.status = http.StatusOK
	agent.manageHandlers = make(map[string]http.Handler)
	return agent
}

func (a *Agent) StartMotanAgent() {
	if !flag.Parsed() {
		flag.Parse()
	}
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
	a.initClusters()
	a.startServerAgent()
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

	pidfile := *motan.Pidfile
	if pidfile == "" && section != nil && section["pidfile"] != nil {
		pidfile = section["pidfile"].(string)
	}
	if pidfile == "" {
		pidfile = defaultPidFile
	}

	vlog.Infof("agent port:%d, manage port:%d, pidfile:%s, logdir:%s\n", port, mport, pidfile, logdir)
	a.logdir = logdir
	a.port = port
	a.mport = mport
	a.pidfile = pidfile
}

func (a *Agent) initClusters() {
	var mapKey string
	for _, url := range a.Context.RefersURLs {
		if url.Parameters[motan.ApplicationKey] == "" {
			url.Parameters[motan.ApplicationKey] = a.agentURL.Parameters[motan.ApplicationKey]
		}
		mapKey = getClusterKey(url.Group, url.GetStringParamsWithDefault(motan.VersionKey, "0.1"), url.Protocol, url.Path)
		c := cluster.NewCluster(url, true)
		c.SetExtFactory(a.extFactory)
		c.Context = a.Context
		c.InitCluster()
		a.clustermap[mapKey] = c
	}
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
	if motanCluster := a.agent.clustermap[ck]; motanCluster != nil {
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
		export := url.GetParam(motan.ExportKey, "")
		url.Protocol, url.Port, _ = motan.ParseExportInfo(export)
		url.Host = motan.GetLocalIP()
		application := url.GetParam(motan.ApplicationKey, "")
		if application == "" {
			application = a.agentURL.GetParam(motan.ApplicationKey, "")
			url.PutParam(motan.ApplicationKey, application)
		}
		url.ClearCachedInfo()
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
		} else {
			vlog.Infof("service export success. url:%s\n", url.GetIdentity())
			for _, r := range exporter.Registrys {
				rid := r.GetURL().GetIdentity()
				if _, ok := a.serviceRegistries[rid]; !ok {
					a.serviceRegistries[rid] = r
				}
			}
		}
	}

}

type serverAgentMessageHandler struct {
	providers map[string]motan.Provider
}

func (sa *serverAgentMessageHandler) Initialize() {
	sa.providers = make(map[string]motan.Provider)
}

func (sa *serverAgentMessageHandler) Call(request motan.Request) (res motan.Response) {
	defer motan.HandlePanic(func() {
		res = motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "provider call panic", ErrType: motan.ServiceException})
		vlog.Errorf("provider call panic. req:%s\n", motan.GetReqInfo(request))
	})
	p := sa.providers[request.GetServiceName()]
	if p != nil {
		res = p.Call(request)
		res.GetRPCContext(true).GzipSize = int(p.GetURL().GetIntValue(motan.GzipSizeKey, 0))
		return res
	}
	vlog.Errorf("not found provider for %s\n", motan.GetReqInfo(request))
	return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "not found provider for " + request.GetServiceName(), ErrType: motan.ServiceException})
}

func (sa *serverAgentMessageHandler) AddProvider(p motan.Provider) error {
	sa.providers[p.GetPath()] = p
	return nil
}

func (sa *serverAgentMessageHandler) RmProvider(p motan.Provider) {}

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
		for _, cls := range a.agent.clustermap {
			for _, registry := range cls.Registrys {
				if cr, ok := registry.(motan.CommandNotifyListener); ok {
					cr.NotifyCommand(registryURL, cluster.AgentCmd, commandInfo)
				}
			}
		}

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
	data, _ := ioutil.ReadFile(*motan.CfgFile)
	return data
}
