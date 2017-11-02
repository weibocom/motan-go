package motan

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	cluster "github.com/weibocom/motan-go/cluster"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	mpro "github.com/weibocom/motan-go/protocol"
	registry "github.com/weibocom/motan-go/registry"
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
	extFactory motan.ExtentionFactory
	Context    *motan.Context

	agentServer motan.Server

	clustermap map[string]*cluster.MotanCluster
	status     int
	agentURL   *motan.URL
	logdir     string
	port       int
	mport      int
	pidfile    string

	agentPortService map[int]motan.Exporter
	agentPortServer  map[int]motan.Server
}

func NewAgent(extfactory motan.ExtentionFactory) *Agent {
	var agent *Agent
	if extfactory == nil {
		fmt.Println("agent using default extentionFactory.")
		agent = &Agent{extFactory: GetDefaultExtFactory()}
	} else {
		agent = &Agent{extFactory: extfactory}
	}
	agent.clustermap = make(map[string]*cluster.MotanCluster)
	agent.agentPortService = make(map[int]motan.Exporter)
	agent.agentPortServer = make(map[int]motan.Server)
	agent.status = http.StatusOK
	return agent
}

func (a *Agent) StartMotanAgent() {
	if !flag.Parsed() {
		flag.Parse()
	}
	a.initContext()
	a.initParam()
	a.initAgentURL()
	a.initClusters()
	a.startServerAgent()
	vlog.Infof("Agent URL inited %s\n", a.agentURL.GetIdentity())

	go a.startMServer()
	f, err := os.Create(a.pidfile)
	if err != nil {
		vlog.Errorf("create file %s fail.\n", a.pidfile)
	} else {
		defer f.Close()
		f.WriteString(strconv.Itoa(os.Getpid()))
	}
	go a.registerAgent()
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

	vlog.Infof("agent port:%s, manage port:%s, pidfile:%s, logdir:%s\n", port, mport, pidfile, logdir)
	a.logdir = logdir
	a.port = port
	a.mport = mport
	a.pidfile = pidfile
}

func (a *Agent) initContext() {
	a.Context = &motan.Context{ConfigFile: a.ConfigFile}
	a.Context.Initialize()
	vlog.Infoln("init agent context success.")
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
	registry.SetSanpshotConf(registry.DefaultSnapshotInterval, snapshotDir)
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
	vlog.Infoln("start agent regitstry.")
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
			vlog.Warningf("can not find agent registry in conf, so do not register. agent url:%s\n", a.agentURL)
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
		exporter := &mserver.DefaultExporter{}
		provider := a.extFactory.GetProvider(url)
		if provider == nil {
			vlog.Errorf("Didn't have a %s provider, url:%+v\n", url.Protocol, url)
			return
		}
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
		}
		err := exporter.Export(server, a.extFactory, globalContext)
		if err != nil {
			vlog.Errorf("service export fail! url:%v, err:%v\n", url, err)
		} else {
			vlog.Infof("service export success. url:%v\n", url)
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
	p := sa.providers[request.GetServiceName()]
	if p != nil {
		return p.Call(request)
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
	return fmt.Sprintf("%s_%s_%s_%s_", group, version, protocol, path)
}

func initLog(logdir string) {
	fmt.Printf("use log dir:%s\n", logdir)
	flag.Set("log_dir", logdir)
	vlog.FlushInterval = 1 * time.Second
	vlog.LogInit(nil)
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

func (a *Agent) startMServer() {
	http.HandleFunc("/", a.rootHandler)
	http.HandleFunc("/503", a.statusSetHandler)
	http.HandleFunc("/200", a.statusSetHandler)
	http.HandleFunc("/getConfig", a.getConfigHandler)
	http.HandleFunc("/getReferService", a.getReferServiceHandler)
	vlog.Infof("start listen manage port %s ...", a.mport)
	http.ListenAndServe(":"+strconv.Itoa(a.mport), nil)
}

type rpcService struct {
	Name   string `json:"name"`
	Status bool   `json:"status"`
}

type body struct {
	Service []rpcService `json:"service"`
}

type jsonRetData struct {
	Code int  `json:"code"`
	Body body `json:"body"`
}

// return agent server status, e.g. 200 or 503
func (a *Agent) rootHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(a.status)
	w.Write([]byte(http.StatusText(a.status)))
}

func (a *Agent) getConfigHandler(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadFile(*motan.CfgFile)
	if err != nil {
		w.Write([]byte("error."))
	} else {
		w.Write(data)
	}
}

func (a *Agent) getReferServiceHandler(w http.ResponseWriter, r *http.Request) {

	mbody := body{Service: []rpcService{}}
	for _, cls := range a.clustermap {
		rpc := cls.GetURL().Path
		available := cls.IsAvailable()
		mbody.Service = append(mbody.Service, rpcService{Name: rpc, Status: available})
	}
	retData := &jsonRetData{Code: 200, Body: mbody}
	if data, err := json.Marshal(&retData); err == nil {
		w.Write(data)
	} else {
		w.Write([]byte("error."))
	}
}

//change agent server status.
func (a *Agent) statusSetHandler(w http.ResponseWriter, r *http.Request) {
	switch r.RequestURI {
	case "/200":
		a.status = http.StatusOK
	case "/503":
		a.status = http.StatusServiceUnavailable
	}
	w.Write([]byte("ok."))
}
