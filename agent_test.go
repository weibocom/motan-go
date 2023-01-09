package motan

import (
	"bytes"
	"flag"
	"fmt"
	_ "fmt"
	"github.com/weibocom/motan-go/config"
	vlog "github.com/weibocom/motan-go/log"
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
	"testing"
	"time"

	gtest "github.com/snail007/gmc/util/testing"
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

func Test_unixClientCall1(t *testing.T) {
	t.Parallel()
	if gtest.RunProcess(t, func() {
		startServer(t, "helloService", 22991)
		time.Sleep(time.Second * 3)
		// start client mesh
		ext := GetDefaultExtFactory()
		os.Remove("agent.sock")
		config, _ := config.NewConfigFromReader(bytes.NewReader([]byte(`motan-agent:
  mport: 12500
  unixSock: agent.sock
  log_dir: "stdout"
  snapshot_dir: "./snapshot"
  application: "testing"

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
      serialization: breeze`)))
		agent := NewAgent(ext)
		go agent.StartMotanAgentFromConfig(config)
		time.Sleep(time.Second * 3)
		c1 := NewMeshClient()
		c1.SetAddress("unix://./agent.sock")
		c1.Initialize()
		req := c1.BuildRequestWithGroup("helloService", "Hello", []interface{}{"jack"}, "hello")
		resp := c1.BaseCall(req, nil)
		if resp.GetException() != nil {
			return
		}
		if "Hello jack from motan server" != resp.GetValue() {
			return
		}
		fmt.Println("check_pass")
	}) {
		return
	}
	out, _, err := gtest.NewProcess(t).Verbose(true).Wait()
	assert.Nil(t, err)
	assert.Contains(t, out, "check_pass")
}
func Test_unixClientCall2(t *testing.T) {
	t.Parallel()
	if gtest.RunProcess(t, func() {
		startServer(t, "helloService", 22992)
		time.Sleep(time.Second * 3)
		// start client mesh
		ext := GetDefaultExtFactory()
		os.Remove("agent2.sock")
		config1, _ := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  mport: 12501
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
      protocol: motanV1Compatible
      registry: direct
      serialization: breeze
`)))
		agent := NewAgent(ext)
		go agent.StartMotanAgentFromConfig(config1)
		time.Sleep(time.Second * 3)

		ext1 := GetDefaultExtFactory()
		cfg, _ := config.NewConfigFromReader(bytes.NewReader([]byte(`
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
    protocol: motanV1Compatible
    group: hello
    path: helloService
    requestTimeout: 3000
`)))
		mccontext := NewClientContextFromConfig(cfg)
		mccontext.Start(ext1)
		mclient := mccontext.GetClient("test-refer")
		var reply string
		err := mclient.Call("Hello", []interface{}{"jack"}, &reply)
		if err != nil {
			return
		}
		if "Hello jack from motan server" != reply {
			return
		}
		fmt.Println("check_pass")
	}) {
		return
	}
	out, _, err := gtest.NewProcess(t).Verbose(true).Wait()
	assert.Nil(t, err)
	assert.Contains(t, out, "check_pass")
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
	a := NewAgent(nil)
	a.Context = &core.Context{Config: conf}
	logFilterCallerFalseConfig, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  log_filter_caller: false
`)))
	conf.Merge(logFilterCallerFalseConfig)
	section, err := conf.GetSection("motan-agent")
	assert.Nil(err)
	assert.Equal(false, section["log_filter_caller"].(bool))
	a.initParam()

	logFilterCallerTrueConfig, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  log_filter_caller: true
`)))
	assert.Nil(err)
	conf.Merge(logFilterCallerTrueConfig)
	section, err = conf.GetSection("motan-agent")
	assert.Nil(err)
	assert.Equal(true, section["log_filter_caller"].(bool))
	a.initParam()

	logDirConfig, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  log_dir: "./test/abcd"
`)))
	assert.Nil(err)
	conf.Merge(logDirConfig)
	section, err = conf.GetSection("motan-agent")
	assert.Nil(err)
	a.initParam()
	assert.Equal(a.logdir, "./test/abcd")
	os.Args = append(os.Args, "-log_dir", "./test/cdef")
	_ = flag.Set("log_dir", "./test/cdef")
	a.initParam()
	assert.Equal(a.logdir, "./test/cdef")
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
	a.initParam()
	assert.Equal(a.mport, 8003)

	mportConfigENV, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  mport: 8003
`)))
	assert.Nil(err)
	conf.Merge(mportConfigENV)
	section, err = conf.GetSection("motan-agent")
	assert.Nil(err)
	err = os.Setenv("mport", "8006")
	a.initParam()
	assert.Equal(a.mport, 8006)

	mportConfigENVParam, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  mport: 8003
`)))
	assert.Nil(err)
	conf.Merge(mportConfigENVParam)
	section, err = conf.GetSection("motan-agent")
	assert.Nil(err)
	err = os.Setenv("mport", "8006")
	_ = flag.Set("mport", "8007")
	a.initParam()
	assert.Equal(a.mport, 8007)
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
	agent := NewAgent(nil)
	agent.agentURL = &core.URL{Parameters: make(map[string]string)}
	urlTest := &core.URL{Parameters: make(map[string]string)}
	urlTest.Group = "test1"
	agent.initCluster(urlTest)
	agentHandler := &agentMessageHandler{agent: agent}

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
		agent.initCluster(v)
	}

	//test init cluster with one path and one groups in clusterMap
	temp := agent.clusterMap.LoadOrNil(getClusterKey("test1", "0.1", "", ""))
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
		{"test0", "", "", "", "No refers for request"},
		{"test-1", "111", "222", "333", "cluster not found. cluster:test-1"},
		{"test3", "", "", "", "empty group is not supported"},
		{"test", "g2", "", "", "No refers for request"},
		{"test", "g1", "", "", "empty protocol is not supported"},
		{"test", "g1", "motan2", "", "No refers for request"},
		{"test", "g1", "motan", "", "empty version is not supported"},
		{"test", "g1", "http", "1.3", "No refers for request"},
		{"test", "g1", "http", "1.2", "less condition to select cluster"},
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

	agent.reloadClusters(ctx)
	assert.Equal(t, agent.serviceMap.Len(), 1, "hot-load serviceMap helloService2 length error")

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
	agent.serviceMap.Store("test6", []serviceMapItem{
		{url: dynamicURLs["test6"], cluster: nil},
	})
	agent.configurer = NewDynamicConfigurer(agent)
	agent.configurer.subscribeNodes = dynamicURLs
	ctx.RefersURLs = reloadUrls
	agent.reloadClusters(ctx)
	assert.Equal(t, agent.serviceMap.Len(), 3, "hot-load serviceMap except length error")

	for _, v := range []struct {
		service  string
		group    string
		protocol string
		version  string
		except   string
	}{
		{"test3", "111", "222", "333", "cluster not found. cluster:test3"},
		{"test4", "", "", "", "empty group is not supported"},
		{"test5", "", "", "", "No refers for request"},
		{"helloService2", "", "", "", "cluster not found. cluster:helloService2"},
	} {
		request = newRequest(v.service, "")
		request.SetAttachment(mpro.MGroup, v.group)
		request.SetAttachment(mpro.MProxyProtocol, v.protocol)
		request.SetAttachment(mpro.MVersion, v.version)
		ret = agentHandler.Call(request)
		assert.True(t, strings.Contains(ret.GetException().ErrMsg, v.except))
	}
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

func Test_unixHTTPClientCall2(t *testing.T) {
	t.Parallel()
	//gtest.DebugRunProcess(t)
	if gtest.RunProcess(t, func() {
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
		// start client mesh
		ext := GetDefaultExtFactory()
		config1, _ := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-agent:
  port: 12821
  mport: 12503
  eport: 23281
  htport: 23282
  log_dir: "stdout"
  snapshot_dir: "./snapshot"
  application: "testing"

motan-registry:
  direct:
    protocol: direct

motan-service:
    test01:
      protocol: motan2
      group: hello
      path: helloService
      registry: direct
      serialization: simple
      provider: http
      proxyAddress: unix://./http2.sock
      enableRewrite: false
      export: motan2:23281
`)))
		agent := NewAgent(ext)
		go agent.StartMotanAgentFromConfig(config1)
		time.Sleep(time.Second * 3)
		c1 := NewMeshClient()
		c1.SetAddress("127.0.0.1:23281")
		c1.Initialize()
		var reply []byte
		req := c1.BuildRequestWithGroup("helloService", "/unixclient", []interface{}{}, "hello")
		req.SetAttachment("HTTP_HOST", "test.com")
		resp := c1.BaseCall(req, &reply)
		if resp.GetException() != nil {
			return
		}
		if "okay" != string(reply) {
			return
		}
		fmt.Println("check_pass")
	}) {
		return
	}
	out, _, err := gtest.NewProcess(t).Verbose(true).Wait()
	assert.Nil(t, err)
	assert.Contains(t, out, "check_pass")
}
