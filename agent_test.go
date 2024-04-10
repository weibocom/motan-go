package motan

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	_ "fmt"
	"github.com/weibocom/motan-go/config"
	"github.com/weibocom/motan-go/endpoint"
	vlog "github.com/weibocom/motan-go/log"
	"github.com/weibocom/motan-go/registry"
	"github.com/weibocom/motan-go/serialize"
	"github.com/weibocom/motan-go/server"
	_ "github.com/weibocom/motan-go/server"
	_ "golang.org/x/net/context"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/core"
	mhttp "github.com/weibocom/motan-go/http"
	mpro "github.com/weibocom/motan-go/protocol"
)

const (
	goNum      = 5
	requestNum = 100
)

var proxyClient *http.Client
var meshClient *MeshClient
var agent *Agent

var (
	testRegistryFailSwitcher int64 = 0
)

func Test_unixClientCall1(t *testing.T) {
	//t.Parallel()
	startServer(t, "helloService", 22991)
	time.Sleep(time.Second * 3)
	// start client mesh
	ext := GetDefaultExtFactory()
	os.Remove("agent.sock")
	clientAgentConfig, _ := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  mport: 12500
  port: 13821
  eport: 13281
  htport: 24282
  unixSock: agent.sock
  log_dir: "stdout"
  snapshot_dir: "./snapshot"
  application: "testing"
  processPoolSize: 100

motan-registry:
  direct:
    protocol: direct
    address: 127.0.0.1:22991

motan-refer:
    recom-engine-refer:
      group: hello
      path: helloService
      protocol: motan2
      registry: direct
      asyncInitConnection: false
      serialization: breeze`)))
	targetAgent := NewAgent(ext)
	go targetAgent.StartMotanAgentFromConfig(clientAgentConfig)
	time.Sleep(time.Second * 3)
	core.SetMport(0)
	c1 := NewMeshClient()
	c1.SetAddress("unix://./agent.sock")
	c1.Initialize()
	req := c1.BuildRequestWithGroup("helloService", "Hello", []interface{}{"jack"}, "hello")
	resp := c1.BaseCall(req, nil)
	assert.Nil(t, resp.GetException())
	assert.Equal(t, "Hello jack from motan server", resp.GetValue())
	assert.Equal(t, 100, server.GetProcessPoolSize())
}

func Test_envHandler(t *testing.T) {
	//t.Parallel()
	time.Sleep(time.Second * 3)
	// start client mesh
	ext := GetDefaultExtFactory()
	os.Remove("agent.sock")
	config, _ := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  mport: 13500
  port: 14821
  eport: 14281
  htport: 25282

motan-registry:
  direct:
    protocol: direct
    address: 127.0.0.1:22991

motan-refer:
    recom-engine-refer:
      group: hello
      path: helloService
      protocol: motan2
      registry: direct
      asyncInitConnection: false
      serialization: breeze`)))
	clientAgent := NewAgent(ext)
	clientAgent.RegisterEnvHandlers("testHandler", map[string]http.Handler{
		"/test/test": testHandler(),
	})
	os.Setenv(core.HandlerEnvironmentName, "testHandler")
	go clientAgent.StartMotanAgentFromConfig(config)
	time.Sleep(time.Second * 3)
	core.SetMport(0)
	client := http.Client{
		Timeout: time.Second,
	}
	resp, err := client.Get("http://127.0.0.1:13500/test/test")
	assert.Nil(t, err)
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err)
	assert.Equal(t, string(b), "OK")
	os.Unsetenv(core.HandlerEnvironmentName)
}

func testHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	}
}

func Test_unixClientCall2(t *testing.T) {
	//t.Parallel()
	startServer(t, "helloService", 22992)
	time.Sleep(time.Second * 3)
	// start client mesh
	ext := GetDefaultExtFactory()
	os.Remove("agent2.sock")
	clientAgentconfig1, _ := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  mport: 12501
  port: 12921
  mport: 12903
  eport: 12981
  htport: 23982
  unixSock: ./agent2.sock
  log_dir: "stdout"
  snapshot_dir: "./snapshot"
  application: "testing"

motan-registry:
  direct:
    protocol: direct
    address: 127.0.0.1:22992

motan-refer:
    test-refer:
      group: hello
      path: helloService
      protocol: motan2
      registry: direct
      serialization: breeze
      asyncInitConnection: false
`)))
	clientAgent := NewAgent(ext)
	go clientAgent.StartMotanAgentFromConfig(clientAgentconfig1)
	time.Sleep(time.Second * 3)
	core.SetMport(0)

	ext1 := GetDefaultExtFactory()
	clientConfig, _ := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-client:
  log_dir: stdout
  application: client-test
motan-registry:
  local:
    protocol: direct
    address: unix://./agent2.sock
motan-refer:
  test-refer:
    registry: local
    serialization: breeze
    protocol: motan2
    group: hello
    path: helloService
    requestTimeout: 3000
    asyncInitConnection: false
`)))
	mccontext := NewClientContextFromConfig(clientConfig)
	mccontext.Start(ext1)
	mclient := mccontext.GetClient("test-refer")
	var reply string
	err := mclient.Call("Hello", []interface{}{"jack"}, &reply)
	assert.Nil(t, err)
	assert.Equal(t, "Hello jack from motan server", reply)
}

func TestMain(m *testing.M) {
	core.RegistLocalProvider("LocalTestService", &LocalTestServiceProvider{})
	cfgFile := filepath.Join("testdata", "agent.yaml")
	go func() {
		var addr = ":9090"
		handler := &http.ServeMux{}
		handler.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
			request.ParseForm()
			writer.Write([]byte(request.URL.String()))
		})
		http.ListenAndServe(addr, handler)
	}()
	go func() {
		agent = NewAgent(nil)
		agent.ConfigFile = cfgFile
		agent.StartMotanAgent()
	}()
	time.Sleep(time.Second * 3)
	core.SetMport(0)
	proxyURL, _ := url.Parse("http://localhost:9983")
	proxyClient = &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	time.Sleep(1 * time.Second)
	meshClient = NewMeshClient()
	meshClient.SetRequestTimeout(time.Second)
	meshClient.Initialize()
	for i := 0; i < 100; i++ {
		resp, err := proxyClient.Get("http://test.domain/tst/test")
		if err != nil {
			continue
		}
		if resp.StatusCode != 200 {
			continue
		}
		time.Sleep(1 * time.Second)
		break
	}
	os.Exit(m.Run())
}

func Test_initParam(t *testing.T) {
	assert := assert.New(t)
	conf, err := config.NewConfigFromFile(filepath.Join("testdata", "agent.yaml"))
	assert.Nil(err)
	testAgent := NewAgent(nil)
	testAgent.Context = &core.Context{Config: conf}
	logFilterCallerFalseConfig, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  log_filter_caller: false
`)))
	conf.Merge(logFilterCallerFalseConfig)
	section, err := conf.GetSection("motan-agent")
	assert.Nil(err)
	assert.Equal(false, section["log_filter_caller"].(bool))
	testAgent.initParam()

	logFilterCallerTrueConfig, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  log_filter_caller: true
`)))
	assert.Nil(err)
	conf.Merge(logFilterCallerTrueConfig)
	section, err = conf.GetSection("motan-agent")
	assert.Nil(err)
	assert.Equal(true, section["log_filter_caller"].(bool))
	testAgent.initParam()

	logDirConfig, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  log_dir: "./test/abcd"
`)))
	assert.Nil(err)
	conf.Merge(logDirConfig)
	section, err = conf.GetSection("motan-agent")
	assert.Nil(err)
	testAgent.initParam()
	assert.Equal(testAgent.logdir, "./test/abcd")
	os.Args = append(os.Args, "-log_dir", "./test/cdef")
	_ = flag.Set("log_dir", "./test/cdef")
	testAgent.initParam()
	assert.Equal(testAgent.logdir, "./test/cdef")
	// test export log dir
	assert.Equal(vlog.GetLogDir(), "./test/cdef")

	mportConfig, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  mport: 8003
`)))
	assert.Nil(err)
	conf.Merge(mportConfig)
	section, err = conf.GetSection("motan-agent")
	assert.Nil(err)
	testAgent.initParam()
	assert.Equal(testAgent.mport, 8003)

	mportConfigENV, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  mport: 8003
`)))
	assert.Nil(err)
	conf.Merge(mportConfigENV)
	section, err = conf.GetSection("motan-agent")
	assert.Nil(err)
	err = os.Setenv("mport", "8006")
	testAgent.initParam()
	assert.Equal(testAgent.mport, 8006)

	mportConfigENVParam, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  mport: 8003
`)))
	assert.Nil(err)
	conf.Merge(mportConfigENVParam)
	section, err = conf.GetSection("motan-agent")
	assert.Nil(err)
	//err = os.Setenv("mport", "8006")
	//_ = flag.Set("mport", "8007")
	//testAgent.initParam()
	//assert.Equal(testAgent.mport, 8007)
	os.Unsetenv("mport")
}

func TestHTTPProxyBodySize(t *testing.T) {
	body := bytes.NewReader(make([]byte, 1000))
	resp, _ := proxyClient.Post("http://test.domain/tst/test", "application/octet-stream", body)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body = bytes.NewReader(make([]byte, 1001))
	resp, _ = proxyClient.Post("http://test.domain/tst/test", "application/octet-stream", body)
	resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHTTPProxy(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(goNum)
	requests := make(chan int, requestNum)
	for i := 0; i < goNum; i++ {
		go func() {
			defer wg.Done()
			for req := range requests {
				suffix := "test" + strconv.Itoa(req)
				resp, err := proxyClient.Get("http://test.domain/tst/test?index=" + suffix)
				if err != nil {
					continue
				}
				bytes, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					resp.Body.Close()
					continue
				}
				resp.Body.Close()
				if !strings.HasSuffix(string(bytes), suffix) {
					t.Errorf("wrong response")
				}
			}
		}()
	}
	for i := 0; i < requestNum; i++ {
		requests <- i
	}
	close(requests)
	wg.Wait()
}

func TestRpcToHTTPProxy(t *testing.T) {
	service := "test.domain"
	request := meshClient.BuildRequest(service, "/tst/xxxx/111", []interface{}{map[string]string{"a": "a"}})
	var reply []byte
	meshClient.BaseCall(request, &reply)
	assert.Equal(t, "/2/tst/xxxx/111?a=a", string(reply))
	request.SetAttachment(mhttp.QueryString, "b=b")
	request.SetAttachment(mhttp.Method, "POST")
	meshClient.BaseCall(request, &reply)
	assert.Equal(t, "/2/tst/xxxx/111?b=b", string(reply))

	wg := &sync.WaitGroup{}
	wg.Add(goNum)
	requests := make(chan int, requestNum)
	for i := 0; i < goNum; i++ {
		go func() {
			defer wg.Done()
			for req := range requests {
				suffix := "test" + strconv.Itoa(req)
				request := meshClient.BuildRequest(service, "/tst/test", []interface{}{map[string]string{"index": suffix}})
				request.SetAttachment(mhttp.Method, "GET")
				var reply []byte
				meshClient.BaseCall(request, &reply)
				if !strings.HasSuffix(string(reply), suffix) {
					t.Errorf("wrong response")
				}
			}
		}()
	}
	for i := 0; i < requestNum; i++ {
		requests <- i
	}
	close(requests)
	wg.Wait()
}

func TestLocalEndpoint(t *testing.T) {
	var reply string
	meshClient.Call("LocalTestService", "hello", []interface{}{"service"}, &reply)
	assert.Equal(t, "hello service", reply)
}

func TestAgent_RuntimeDir(t *testing.T) {
	assert.NotEmpty(t, agent.RuntimeDir())
}

func TestAgent_InitCall(t *testing.T) {
	//init
	targetAgent := NewAgent(nil)
	targetAgent.agentURL = &core.URL{Parameters: make(map[string]string)}
	urlTest := &core.URL{Parameters: make(map[string]string)}
	urlTest.Group = "test1"
	urlTest.Parameters[core.AsyncInitConnection] = "false"
	targetAgent.initCluster(urlTest)
	agentHandler := &agentMessageHandler{agent: targetAgent}

	for _, v := range []*core.URL{
		{Parameters: map[string]string{core.VersionKey: ""}, Path: "test3", Group: "", Protocol: ""},
		{Parameters: map[string]string{core.VersionKey: ""}, Path: "test3", Group: "", Protocol: ""},
		{Parameters: map[string]string{core.VersionKey: "1.0"}, Path: "test", Group: "g1", Protocol: "motan2"},
		{Parameters: map[string]string{core.VersionKey: "1.0"}, Path: "test", Group: "g1", Protocol: "motan"},
		{Parameters: map[string]string{core.VersionKey: "1.1"}, Path: "test", Group: "g2", Protocol: "motan"},
		{Parameters: map[string]string{core.VersionKey: "1.2"}, Path: "test", Group: "g1", Protocol: "motan"},
		{Parameters: map[string]string{core.VersionKey: "1.2"}, Path: "test", Group: "g1", Protocol: "http"},
		{Parameters: map[string]string{core.VersionKey: "1.2"}, Path: "test", Group: "g1", Protocol: "http"},
		{Parameters: map[string]string{core.VersionKey: "1.3"}, Path: "test", Group: "g1", Protocol: "http"},
		{Parameters: map[string]string{core.VersionKey: "1.3"}, Path: "test0", Group: "g0", Protocol: "http"},
	} {
		targetAgent.initCluster(v)
	}

	//test init cluster with one path and one groups in clusterMap
	temp := targetAgent.clusterMap.LoadOrNil(getClusterKey("test1", "1.0", "", ""))
	assert.NotNil(t, temp, "init cluster with one path and two groups in clusterMap fail")

	//test agentHandler call with group
	request := &core.MotanRequest{Attachment: core.NewStringMap(10)}
	request.SetAttachment(mpro.MGroup, "test1")
	ret := agentHandler.Call(request)
	assert.True(t, strings.Contains(ret.GetException().ErrMsg, "empty service is not supported"))

	//test agentHandler call without group
	request.SetAttachment(mpro.MGroup, "")
	ret = agentHandler.Call(request)
	assert.True(t, strings.Contains(ret.GetException().ErrMsg, "empty service is not supported"))

	//test agentHandler call without group
	request.SetAttachment(mpro.MGroup, "")
	ret = agentHandler.Call(request)
	assert.True(t, strings.Contains(ret.GetException().ErrMsg, "empty service is not supported"))

	for _, v := range []struct {
		service  string
		group    string
		protocol string
		version  string
		except   string
	}{
		// only input service，and there is only one cluster，findCluster would return successfully
		{"test0", "", "", "", "No refers for request"},
		{"test0", "g0", "", "", "No refers for request"},
		{"test0", "g0", "http", "", "No refers for request"},
		{"test0", "g0", "", "1.3", "No refers for request"},
		{"test-1", "111", "222", "333", "cluster not found"},
		{"test", "g2", "", "", "No refers for request"},
		{"test", "g1", "motan2", "", "No refers for request"},
		{"test", "g1", "http", "1.3", "No refers for request"},
		{"test", "b", "c", "d", "no cluster matches the request"},
		// one service matches multiple clusters, without passing group
		{"test", "", "c", "d", "multiple clusters are matched with service"},
	} {
		request.ServiceName = v.service
		request.SetAttachment(mpro.MGroup, v.group)
		request.SetAttachment(mpro.MProxyProtocol, v.protocol)
		request.SetAttachment(mpro.MVersion, v.version)
		ret = agentHandler.Call(request)
		assert.True(t, strings.Contains(ret.GetException().ErrMsg, v.except))
	}

	// test hot reload clusters normal
	startServer(t, "helloService2", 64533)

	helloService := core.FromExtInfo("motan2://127.0.0.1:64533/helloService2?serialization=simple")
	assert.NotNil(t, helloService, "hello hot-reload service start fail")

	ctx := &core.Context{ConfigFile: filepath.Join("testdata", "agent-reload.yaml")}
	ctx.Initialize()
	assert.NotNil(t, ctx, "hot-reload config file context initialize fail")

	// wait ha
	time.Sleep(time.Second * 1)

	targetAgent.reloadClusters(ctx)
	assert.Equal(t, targetAgent.serviceMap.Len(), 1, "hot-load serviceMap helloService2 length error")

	request = newRequest("helloService2", "hello", "Ray")
	motanResponse := agentHandler.Call(request)
	var responseBody []byte
	err := motanResponse.ProcessDeserializable(&responseBody)
	assert.Nil(t, err, err)
	assert.Equal(t, "Hello Ray from motan server", string(responseBody), "hot-reload service response error")

	// test hot reload clusters except
	reloadUrls := map[string]*core.URL{
		"test4-0": {Parameters: map[string]string{core.VersionKey: ""}, Path: "test4", Group: "g1", Protocol: ""},
		"test4-1": {Parameters: map[string]string{core.VersionKey: ""}, Path: "test4", Group: "g2", Protocol: ""},
		"test5":   {Parameters: map[string]string{core.VersionKey: "1.0"}, Path: "test5", Group: "g1", Protocol: "motan"},
	}
	dynamicURLs := map[string]*core.URL{
		"test6": {Parameters: map[string]string{core.VersionKey: ""}, Path: "test6", Group: "g1", Protocol: ""},
	}
	targetAgent.serviceMap.Store("test6", []serviceMapItem{
		{url: dynamicURLs["test6"], cluster: nil},
	})
	targetAgent.configurer = NewDynamicConfigurer(targetAgent)
	targetAgent.configurer.subscribeNodes = dynamicURLs
	ctx.RefersURLs = reloadUrls
	targetAgent.reloadClusters(ctx)
	assert.Equal(t, targetAgent.serviceMap.Len(), 3, "hot-load serviceMap except length error")

	for _, v := range []struct {
		service  string
		group    string
		protocol string
		version  string
		except   string
	}{
		{"test3", "111", "222", "333", "cluster not found. service: test3"},
		{"test5", "", "", "", "No refers for request"},
		{"helloService2", "", "", "", "cluster not found. service: helloService2"},
	} {
		request = newRequest(v.service, "")
		request.SetAttachment(mpro.MGroup, v.group)
		request.SetAttachment(mpro.MProxyProtocol, v.protocol)
		request.SetAttachment(mpro.MVersion, v.version)
		ret = agentHandler.Call(request)
		assert.True(t, strings.Contains(ret.GetException().ErrMsg, v.except))
	}
}

func TestNotFoundProvider(t *testing.T) {
	notFoundService := "notFoundService"
	request := meshClient.BuildRequest(notFoundService, "test", []interface{}{})
	epUrl := &core.URL{
		Protocol: endpoint.Motan2,
		Host:     "127.0.0.1",
		Port:     9982,
		Path:     notFoundService,
		Group:    "test",
		Parameters: map[string]string{
			core.TimeOutKey:              "3000",
			core.ApplicationKey:          "testep",
			core.ConnectRetryIntervalKey: "5000",
			core.ErrorCountThresholdKey:  "0",
			core.AsyncInitConnection:     "false",
		},
	}
	ext := GetDefaultExtFactory()
	ep := ext.GetEndPoint(epUrl)
	assert.NotNil(t, ep)
	ep.SetSerialization(ext.GetSerialization(serialize.Simple, serialize.SimpleNumber))
	core.Initialize(ep)
	ep.SetProxy(true)
	resp := ep.Call(request)
	assert.NotNil(t, resp.GetException())
	assert.Contains(t, core.ProviderNotExistPrefix+"test_notFoundService", resp.GetException().ErrMsg)
	assert.Equal(t, int64(1), atomic.LoadInt64(&notFoundProviderCount))
	ep.Destroy()
}

func newRequest(serviceName string, method string, argments ...interface{}) *core.MotanRequest {
	request := &core.MotanRequest{
		RequestID:   rand.Uint64(),
		ServiceName: serviceName,
		Method:      method,
		Attachment:  core.NewStringMap(core.DefaultAttachmentSize),
	}
	request.Arguments = argments
	return request
}

type LocalTestServiceProvider struct {
	url *core.URL
}

func (l *LocalTestServiceProvider) GetRuntimeInfo() map[string]interface{} {
	return map[string]interface{}{}
}

func (l *LocalTestServiceProvider) SetService(s interface{}) {
}

func (l *LocalTestServiceProvider) GetURL() *core.URL {
	return l.url
}

func (l *LocalTestServiceProvider) SetURL(url *core.URL) {
	l.url = url
}

func (l *LocalTestServiceProvider) IsAvailable() bool {
	return true
}

func (l *LocalTestServiceProvider) Call(request core.Request) core.Response {
	var requestStr string
	err := request.ProcessDeserializable([]interface{}{&requestStr})
	if err != nil {
		return core.BuildExceptionResponse(request.GetRequestID(), &core.Exception{
			ErrCode: 500,
			ErrMsg:  err.Error(),
			ErrType: core.ServiceException,
		})
	}
	return &core.MotanResponse{
		RequestID:   request.GetRequestID(),
		Value:       request.GetMethod() + " " + requestStr,
		Exception:   nil,
		ProcessTime: 0,
		Attachment:  nil,
		RPCContext:  nil,
	}
}

func (l *LocalTestServiceProvider) Destroy() {
}

func (l *LocalTestServiceProvider) GetPath() string {
	return l.url.Path
}

func Test_unixHTTPClientCall(t *testing.T) {
	//t.Parallel()
	go func() {
		http.HandleFunc("/unixclient", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("okay"))
		})
		os.Remove("http2.sock")
		addr, _ := net.ResolveUnixAddr("unix", "http2.sock")
		l, err := net.ListenUnix("unix", addr)
		if err != nil {
			panic(err)
		}
		err = http.Serve(l, nil)
		if err != nil {
			panic(err)
		}
	}()
	// start unix server mesh
	ext := GetDefaultExtFactory()
	serverAgentConfig1, _ := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  port: 12822
  mport: 12504
  eport: 23282
  htport: 23283
  log_dir: "stdout"
  snapshot_dir: "./snapshot"
  application: "testing"

motan-registry:
  direct:
    protocol: direct

motan-service:
    test01:
      protocol: motan2
      provider: http
      proxyAddress: unix://./http2.sock
      group: hello
      path: helloService
      registry: direct
      serialization: simple
      enableRewrite: false
      export: motan2:23282
`)))
	serverAgent := NewAgent(ext)
	go serverAgent.StartMotanAgentFromConfig(serverAgentConfig1)
	time.Sleep(time.Second * 3)
	core.SetMport(0)

	c1 := NewMeshClient()
	c1.SetAddress("127.0.0.1:23282")
	c1.Initialize()
	var reply []byte
	req := c1.BuildRequestWithGroup("helloService", "/unixclient", []interface{}{}, "hello")
	req.SetAttachment("http_Host", "test.com")
	resp := c1.BaseCall(req, &reply)
	assert.Nil(t, resp.GetException())
	assert.Equal(t, "okay", string(reply))
}

func Test_unixRPCClientCall(t *testing.T) {
	//t.Parallel()
	os.Remove("server.sock")
	startServer(t, "helloService", 0, "server.sock")
	time.Sleep(time.Second * 3)
	// start client mesh
	// start unix server mesh
	ext := GetDefaultExtFactory()
	serverMeshConfig, _ := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  port: 12821
  mport: 12503
  eport: 12281
  htport: 23282
  log_dir: "stdout"
  snapshot_dir: "./snapshot"
  application: "testing"
  motanEpAsyncInit: false

motan-registry:
  direct:
    protocol: direct

motan-service:
    test01:
      protocol: motan2
      provider: motan2
      group: hello
      path: helloService
      registry: direct
      serialization: simple
      proxy.host: unix://./server.sock
      export: motan2:12281
`)))
	serverAgent := NewAgent(ext)
	go serverAgent.StartMotanAgentFromConfig(serverMeshConfig)
	time.Sleep(time.Second * 3)
	core.SetMport(0)

	c1 := NewMeshClient()
	c1.SetAddress("127.0.0.1:12281")
	c1.Initialize()
	var reply []byte
	req := c1.BuildRequestWithGroup("helloService", "Hello", []interface{}{"jack"}, "hello")
	resp := c1.BaseCall(req, &reply)
	assert.Nil(t, resp.GetException())
	assert.Equal(t, "Hello jack from motan server", string(reply))

	// test not found provider endpoint circuit breaker and runtime info
	reqNotFound := c1.BuildRequestWithGroup("helloServiceNotFound", "Hello", []interface{}{"jack"}, "helloNotFound")
	respNotFound := c1.BaseCall(reqNotFound, &reply)
	assert.NotNil(t, respNotFound.GetException())
	assert.Equal(t, core.ProviderNotExistPrefix+"helloNotFound_helloServiceNotFound", respNotFound.GetException().ErrMsg)
}

func Test_changeDefaultMotanEpAsyncInit(t *testing.T) {
	serviceAgentConfigTemplate := `
motan-agent:
  port: 12821
  mport: 12503
  eport: 12281
  htport: 23282
  log_dir: "stdout"
  motanEpAsyncInit: %s
  snapshot_dir: "./snapshot"
  application: "testing"

motan-registry:
  direct:
    protocol: direct

motan-service:
    test01:
      protocol: motan2
      provider: motan2
      group: hello
      path: helloService
      registry: direct
      serialization: simple
      proxy.host: unix://./server.sock
      export: motan2:12281
`
	ext := GetDefaultExtFactory()
	config1, _ := config.NewConfigFromReader(bytes.NewReader([]byte(fmt.Sprintf(serviceAgentConfigTemplate, "false"))))
	serverAgent := NewAgent(ext)
	serverAgent.Context = &core.Context{Config: config1}
	serverAgent.Context.Initialize()
	serverAgent.initParam()
	assert.Equal(t, endpoint.GetDefaultMotanEPAsynInit(), false)

	config2, _ := config.NewConfigFromReader(bytes.NewReader([]byte(fmt.Sprintf(serviceAgentConfigTemplate, "true"))))
	serverAgent = NewAgent(ext)
	serverAgent.Context = &core.Context{Config: config2}
	serverAgent.Context.Initialize()
	serverAgent.initParam()
	assert.Equal(t, endpoint.GetDefaultMotanEPAsynInit(), true)
}

func Test_agentRegistryFailback(t *testing.T) {
	currentFailbackInterval := registry.GetFailbackInterval()
	newInterval := uint32(5000)
	registry.SetFailbackInterval(newInterval)

	serverAgentTemplate := `
motan-agent:
  port: 12829
  mport: 12604
  eport: 12282
  hport: 23283

motan-registry:
  direct:
    protocol: direct
  test:
    protocol: test-registry

motan-service:
  test01:
    protocol: motan2
    provider: motan2
    group: hello
    path: helloService
    registry: test
    serialization: simple
    export: motan2:12282
    check: true
`
	extFactory := GetDefaultExtFactory()
	extFactory.RegistExtRegistry("test-registry", func(url *core.URL) core.Registry {
		return &testRegistry{url: url}
	})
	config1, err := config.NewConfigFromReader(bytes.NewReader([]byte(serverAgentTemplate)))
	assert.Nil(t, err)
	serverAgent := NewAgent(extFactory)
	go serverAgent.StartMotanAgentFromConfig(config1)
	time.Sleep(time.Second * 3)
	core.SetMport(0)

	setRegistryFailSwitcher(true)
	m := serverAgent.GetRegistryStatus()
	assert.Equal(t, len(m), 1)
	for _, mm := range m[0] {
		if mm.Service.Path == "helloService" {
			assert.Equal(t, mm.Status, core.NotRegister)
		}
	}
	agentStatus := getCurAgentStatus(12604)
	assert.Equal(t, agentStatus, core.NotRegister)
	serverAgent.SetAllServicesAvailable()
	m = serverAgent.GetRegistryStatus()
	for _, mm := range m[0] {
		if mm.Service.Path == "helloService" {
			assert.Equal(t, mm.Status, core.RegisterFailed)
		}
	}
	agentStatus = getCurAgentStatus(12604)
	assert.Equal(t, agentStatus, core.RegisterFailed)
	setRegistryFailSwitcher(false)
	time.Sleep(time.Duration(newInterval) * time.Millisecond)
	m = serverAgent.GetRegistryStatus()
	assert.Equal(t, len(m), 1)
	for _, mm := range m[0] {
		if mm.Service.Path == "helloService" {
			assert.Equal(t, mm.Status, core.RegisterSuccess)
		}
	}
	agentStatus = getCurAgentStatus(12604)
	assert.Equal(t, agentStatus, core.RegisterSuccess)
	setRegistryFailSwitcher(true)
	serverAgent.SetAllServicesUnavailable()
	m = serverAgent.GetRegistryStatus()
	assert.Equal(t, len(m), 1)
	for _, mm := range m[0] {
		if mm.Service.Path == "helloService" {
			assert.Equal(t, mm.Status, core.UnregisterFailed)
		}
	}
	agentStatus = getCurAgentStatus(12604)
	assert.Equal(t, agentStatus, core.UnregisterFailed)
	setRegistryFailSwitcher(false)
	time.Sleep(time.Duration(newInterval) * time.Millisecond)
	m = serverAgent.GetRegistryStatus()
	assert.Equal(t, len(m), 1)
	for _, mm := range m[0] {
		if mm.Service.Path == "helloService" {
			assert.Equal(t, mm.Status, core.UnregisterSuccess)
		}
	}
	agentStatus = getCurAgentStatus(12604)
	assert.Equal(t, agentStatus, core.UnregisterSuccess)
	registry.SetFailbackInterval(currentFailbackInterval)
}

type testRegistry struct {
	url                 *core.URL
	namingServiceStatus *core.CopyOnWriteMap
	registeredServices  map[string]*core.URL
}

func (t *testRegistry) GetRuntimeInfo() map[string]interface{} {
	return map[string]interface{}{}
}

func (t *testRegistry) Initialize() {
	t.registeredServices = make(map[string]*core.URL)
	t.namingServiceStatus = core.NewCopyOnWriteMap()
}

func (t *testRegistry) GetName() string {
	return "test-registry"
}

func (t *testRegistry) GetURL() *core.URL {
	return t.url
}

func (t *testRegistry) SetURL(url *core.URL) {
	t.url = url
}

func (t *testRegistry) Subscribe(url *core.URL, listener core.NotifyListener) {
}

func (t *testRegistry) Unsubscribe(url *core.URL, listener core.NotifyListener) {
}

func (t *testRegistry) Discover(url *core.URL) []*core.URL {
	return nil
}

func (t *testRegistry) Register(serverURL *core.URL) {
	t.registeredServices[serverURL.GetIdentity()] = serverURL
	t.namingServiceStatus.Store(serverURL.GetIdentity(), &core.RegistryStatus{
		Status:   core.NotRegister,
		Service:  serverURL,
		Registry: t,
		IsCheck:  registry.IsCheck(serverURL),
	})
}

func (t *testRegistry) UnRegister(serverURL *core.URL) {
	delete(t.registeredServices, serverURL.GetIdentity())
	t.namingServiceStatus.Delete(serverURL.GetIdentity())
}

func (t *testRegistry) Available(serverURL *core.URL) {
	if getRegistryFailSwitcher() {
		for _, u := range t.registeredServices {
			t.namingServiceStatus.Store(u.GetIdentity(), &core.RegistryStatus{
				Status:   core.RegisterFailed,
				Registry: t,
				Service:  u,
				ErrMsg:   "error",
				IsCheck:  registry.IsCheck(u),
			})
		}
	} else {
		for _, u := range t.registeredServices {
			t.namingServiceStatus.Store(u.GetIdentity(), &core.RegistryStatus{
				Status:   core.RegisterSuccess,
				Registry: t,
				Service:  u,
				IsCheck:  registry.IsCheck(u),
			})
		}
	}
}

func (t *testRegistry) Unavailable(serverURL *core.URL) {
	if getRegistryFailSwitcher() {
		for _, u := range t.registeredServices {
			t.namingServiceStatus.Store(u.GetIdentity(), &core.RegistryStatus{
				Status:   core.UnregisterFailed,
				Registry: t,
				Service:  u,
				ErrMsg:   "error",
				IsCheck:  registry.IsCheck(u),
			})
		}
	} else {
		for _, u := range t.registeredServices {
			t.namingServiceStatus.Store(u.GetIdentity(), &core.RegistryStatus{
				Status:   core.UnregisterSuccess,
				Registry: t,
				Service:  u,
				IsCheck:  registry.IsCheck(u),
			})
		}
	}
}

func (t *testRegistry) GetRegisteredServices() []*core.URL {
	return nil
}

func (t *testRegistry) StartSnapshot(conf *core.SnapshotConf) {
}

func (t *testRegistry) GetRegistryStatus() map[string]*core.RegistryStatus {
	res := make(map[string]*core.RegistryStatus)
	t.namingServiceStatus.Range(func(k, v interface{}) bool {
		res[k.(string)] = v.(*core.RegistryStatus)
		return true
	})
	return res
}

func setRegistryFailSwitcher(b bool) {
	if b {
		atomic.StoreInt64(&testRegistryFailSwitcher, 1)
	} else {
		atomic.StoreInt64(&testRegistryFailSwitcher, 0)
	}
}

func getRegistryFailSwitcher() bool {
	return atomic.LoadInt64(&testRegistryFailSwitcher) == 1
}

func getCurAgentStatus(port int64) string {
	type (
		Result struct {
			Status         string      `json:"status"`
			RegistryStatus interface{} `json:"registryStatus"`
		}
	)
	client := http.Client{
		Timeout: time.Second * 3,
	}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/registry/status", port))
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err.Error()
	}
	res := Result{}
	err = json.Unmarshal(b, &res)
	if err != nil {
		return err.Error()
	}
	return res.Status
}

func TestRuntimeHandler(t *testing.T) {
	resp, err := http.Get("http://127.0.0.1:8002/runtime/info")
	assert.Nil(t, err)
	var runtimeInfo map[string]interface{}
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err)
	err = resp.Body.Close()
	assert.Nil(t, err)
	err = json.Unmarshal(bodyBytes, &runtimeInfo)
	assert.Nil(t, err)

	for _, s := range []string{
		core.RuntimeInstanceTypeKey,
		core.RuntimeExportersKey,
		core.RuntimeClustersKey,
		core.RuntimeHttpClustersKey,
		core.RuntimeExtensionFactoryKey,
		core.RuntimeServersKey,
		core.RuntimeBasicKey,
	} {
		info, ok := runtimeInfo[s]
		assert.True(t, ok)
		assert.NotNil(t, info)
		t.Logf("key: %s", s)
	}
}
