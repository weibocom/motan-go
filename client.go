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
	clients    map[string]*Client

	csync  sync.Mutex
	inited bool
}

type Client struct {
	url        *motan.URL
	cluster    *cluster.MotanCluster
	extFactory motan.ExtentionFactory
}

func (c *Client) Call(method string, args []interface{}, reply interface{}) error {
	req := c.BuildRequest(method, args)
	return c.BaseCall(req, reply)
}

func (c *Client) BaseCall(req motan.Request, reply interface{}) error {
	rc := req.GetRPCContext(true)
	rc.ExtFactory = c.extFactory
	rc.Reply = reply
	res := c.cluster.Call(req)
	if res.GetException() != nil {
		return errors.New(res.GetException().ErrMsg)
	}
	return nil
}

func (c *Client) Go(method string, args []interface{}, reply interface{}, done chan *motan.AsyncResult) *motan.AsyncResult {
	req := c.BuildRequest(method, args)
	return c.BaseGo(req, reply, done)
}

func (c *Client) BaseGo(req motan.Request, reply interface{}, done chan *motan.AsyncResult) *motan.AsyncResult {
	result := &motan.AsyncResult{}
	if done == nil || cap(done) == 0 {
		done = make(chan *motan.AsyncResult, 5)
	}
	result.Done = done
	rc := req.GetRPCContext(true)
	rc.ExtFactory = c.extFactory
	rc.Result = result
	rc.AsyncCall = true
	rc.Result.Reply = reply
	res := c.cluster.Call(req)
	if res.GetException() != nil {
		result.Error = errors.New(res.GetException().ErrMsg)
		result.Done <- result
	}
	return result
}

func (c *Client) BuildRequest(method string, args []interface{}) motan.Request {
	req := &motan.MotanRequest{Method: method, ServiceName: c.url.Path, Arguments: args, Attachment: motan.NewStringMap(motan.DefaultAttachmentSize)}
	version := c.url.GetParam(motan.VersionKey, "")
	if version != "" {
		req.SetAttachment(mpro.MVersion, version)
	}
	module := c.url.GetParam(motan.ModuleKey, "")
	if module != "" {
		req.SetAttachment(mpro.MModule, module)
	}
	application := c.url.GetParam(motan.ApplicationKey, "")
	if application != "" {
		req.SetAttachment(mpro.MSource, application)
	}
	req.SetAttachment(mpro.MGroup, c.url.Group)

	return req
}

func GetClientContext(confFile string) *MCContext {
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

		m.clients = make(map[string]*Client, 32)
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

	for key, url := range m.context.RefersURLs {
		c := cluster.NewCluster(url, false)
		c.SetExtFactory(m.extFactory)
		c.Context = m.context
		c.InitCluster()
		m.clients[key] = &Client{url: url, cluster: c, extFactory: m.extFactory}
	}
}

func (m *MCContext) GetClient(clientid string) *Client {
	return m.clients[clientid]
}

func (m *MCContext) GetRefer(service string) interface{} {
	// TODO 对client的封装，可以根据idl自动生成代码时支持
	return nil
}
