package motan

import (
	"errors"
	"flag"
	"fmt"
	"github.com/weibocom/motan-go/cluster"
	"github.com/weibocom/motan-go/config"
	motan "github.com/weibocom/motan-go/core"
	vlog "github.com/weibocom/motan-go/log"
	mpro "github.com/weibocom/motan-go/protocol"
	"sync"
)

var (
	clientContextMap   = make(map[string]*MCContext, 8)
	clientContextMutex sync.Mutex
)

type MCContext struct {
	config     *config.Config
	context    *motan.Context
	extFactory motan.ExtensionFactory
	clients    map[string]*Client

	csync  sync.Mutex
	inited bool
}

type Client struct {
	url        *motan.URL
	cluster    *cluster.MotanCluster
	extFactory motan.ExtensionFactory
}

func NewClient(url *motan.URL, cluster *cluster.MotanCluster, extFactory motan.ExtensionFactory) *Client {
	return &Client{
		url:        url,
		cluster:    cluster,
		extFactory: extFactory,
	}
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
	rc.Reply = reply
	go func() {
		res := c.cluster.Call(req)
		if res.GetException() != nil {
			result.Error = errors.New(res.GetException().ErrMsg)
		}
		result.Done <- result
	}()
	return result
}

func (c *Client) BuildRequest(method string, args []interface{}) motan.Request {
	req := &motan.MotanRequest{Method: method, ServiceName: c.url.Path, Arguments: args, Attachment: motan.NewStringMap(motan.DefaultAttachmentSize)}
	version := c.url.GetParam(motan.VersionKey, "")
	req.SetAttachment(mpro.MVersion, version)
	module := c.url.GetParam(motan.ModuleKey, "")
	req.SetAttachment(mpro.MModule, module)
	application := c.url.GetParam(motan.ApplicationKey, "")
	if application == "" {
		application = c.cluster.Context.ClientURL.GetParam(motan.ApplicationKey, "")
	}
	req.SetAttachment(mpro.MSource, application)
	req.SetAttachment(mpro.MGroup, c.url.Group)
	req.SetAttachment(mpro.MPath, req.GetServiceName())
	return req
}

func NewClientContextFromConfig(conf *config.Config) (mc *MCContext) {
	mc = &MCContext{config: conf}
	motan.Initialize(mc)
	section, err := mc.context.Config.GetSection("motan-client")
	if err != nil {
		fmt.Println("get config of \"motan-client\" fail! err " + err.Error())
	}

	logDir := ""
	if section != nil && section["log_dir"] != nil {
		logDir = section["log_dir"].(string)
	}
	if logDir == "" {
		logDir = "."
	}
	initLog(logDir, section)
	registerSwitchers(mc.context)
	return mc
}

func GetClientContext(confFile string) *MCContext {
	return GetClientContextWithFlagParse(confFile, true)
}

func GetClientContextWithFlagParse(confFile string, parseFlag bool) *MCContext {
	if parseFlag && !flag.Parsed() {
		flag.Parse()
	}
	clientContextMutex.Lock()
	defer clientContextMutex.Unlock()
	mc := clientContextMap[confFile]
	if mc == nil {
		conf, err := config.NewConfigFromFile(confFile)
		if err != nil {
			vlog.Errorf("parse config file fail, error: %s, file: %s", err, confFile)
			return nil
		}
		mc = NewClientContextFromConfig(conf)
		clientContextMap[confFile] = mc
	}
	return mc
}

func (m *MCContext) Initialize() {
	m.csync.Lock()
	defer m.csync.Unlock()
	if !m.inited {
		m.context = motan.NewContextFromConfig(m.config, "", "")
		m.clients = make(map[string]*Client, 32)
		m.inited = true
	}
}

func (m *MCContext) Start(extfactory motan.ExtensionFactory) {
	m.csync.Lock()
	defer m.csync.Unlock()
	m.extFactory = extfactory
	if m.extFactory == nil {
		m.extFactory = GetDefaultExtFactory()
	}

	for key, url := range m.context.RefersURLs {
		c := cluster.NewCluster(m.context, m.extFactory, url, false)
		m.clients[key] = NewClient(url, c, m.extFactory)
	}
}

func (m *MCContext) GetClient(clientid string) *Client {
	return m.clients[clientid]
}

func (m *MCContext) GetRefer(service string) interface{} {
	// TODO 对client的封装，可以根据idl自动生成代码时支持
	return nil
}

func (m *MCContext) GetContext() *motan.Context {
	return m.context
}
