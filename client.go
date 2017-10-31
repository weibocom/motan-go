package motan

import (
	"errors"
	"flag"
	"fmt"
	"sync"

	cluster "github.com/weibocom/motan-go/cluster"
	motan "github.com/weibocom/motan-go/core"
	mpro "github.com/weibocom/motan-go/protocol"
)

var (
	clientContextMap   = make(map[string]*MCContext, 8)
	clientContextMutex sync.Mutex
)

type MCContext struct {
	confFile   string
	context    *motan.Context
	extFactory motan.ExtentionFactory
	clients    map[string]*MotanClient

	csync  sync.Mutex
	inited bool
}

type MotanClient struct {
	url        *motan.Url
	cluster    *cluster.MotanCluster
	extFactory motan.ExtentionFactory
}

func (m *MotanClient) Call(method string, args interface{}, reply interface{}) error {
	req := m.buildRequest(method, args)
	rc := req.GetRpcContext(true)
	rc.ExtFactory = m.extFactory
	rc.Reply = reply
	res := m.cluster.Call(req)
	if res.GetException() != nil {
		return errors.New(res.GetException().ErrMsg)
	} else {
		return nil
	}
}

func (m *MotanClient) Go(method string, args interface{}, reply interface{}, done chan *motan.AsyncResult) *motan.AsyncResult {
	req := m.buildRequest(method, args)
	result := &motan.AsyncResult{}
	if done == nil || cap(done) == 0 {
		done = make(chan *motan.AsyncResult, 5)
	}
	result.Done = done
	rc := req.GetRpcContext(true)
	rc.ExtFactory = m.extFactory
	rc.Result = result
	rc.AsyncCall = true
	rc.Result.Reply = reply
	res := m.cluster.Call(req)
	if res.GetException() != nil {
		result.Error = errors.New(res.GetException().ErrMsg)
		result.Done <- result
	}
	return result
}

func (m *MotanClient) buildRequest(method string, args interface{}) motan.Request {
	req := &motan.MotanRequest{Method: method, ServiceName: m.url.Path, Arguments: []interface{}{args}, Attachment: make(map[string]string, 16)}
	version := m.url.GetParam(motan.VersionKey, "")
	if version != "" {
		req.Attachment[mpro.M_version] = version
	}
	module := m.url.GetParam(motan.ModuleKey, "")
	if module != "" {
		req.Attachment[mpro.M_module] = module
	}
	application := m.url.GetParam(motan.ApplicationKey, "")
	if application != "" {
		req.Attachment[mpro.M_source] = application
	}
	req.Attachment[mpro.M_group] = m.url.Group

	return req
}

func GetMotanClientContext(confFile string) *MCContext {
	if !flag.Parsed() {
		flag.Parse()
	}
	clientContextMutex.Lock()
	defer clientContextMutex.Unlock()
	mc := clientContextMap[confFile]
	if mc == nil {
		mc = &MCContext{confFile: confFile}
		clientContextMap[confFile] = mc
		motan.Initialize(mc)
		section, err := mc.context.Config.GetSection("motan-client")
		if err != nil {
			fmt.Println("get config of \"motan-client\" fail! err " + err.Error())
		}

		logdir := ""
		if section != nil && section["log_dir"] != nil {
			logdir = section["log_dir"].(string)
		}
		if logdir == "" {
			logdir = "."
		}
		initLog(logdir)
	}
	return mc
}

func (m *MCContext) Initialize() {
	m.csync.Lock()
	defer m.csync.Unlock()
	if !m.inited {
		m.context = &motan.Context{ConfigFile: m.confFile}
		m.context.Initialize()

		m.clients = make(map[string]*MotanClient, 32)
		m.inited = true
	}
}

func (m *MCContext) Start(extfactory motan.ExtentionFactory) {
	m.csync.Lock()
	defer m.csync.Unlock()
	m.extFactory = extfactory
	if m.extFactory == nil {
		m.extFactory = GetDefaultExtFactory()
	}

	for key, url := range m.context.RefersUrls {
		c := cluster.NewCluster(url, false)
		c.SetExtFactory(m.extFactory)
		c.Context = m.context
		c.InitCluster()
		m.clients[key] = &MotanClient{url: url, cluster: c, extFactory: m.extFactory}
	}
}

func (m *MCContext) GetClient(clientid string) *MotanClient {
	return m.clients[clientid]
}

func (m *MCContext) GetRefer(service string) interface{} {
	// TODO 对client的封装，可以根据idl自动生成代码时支持
	return nil
}
