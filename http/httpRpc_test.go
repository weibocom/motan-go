package http_test

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go"
	"github.com/weibocom/motan-go/config"
	motancore "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/protocol"
	"math/rand"
	"net/http"
	"os"
	"testing"
	"time"
)

const serverConf = `
motan-agent:
  port: 9985 # agent rpc forward proxy port.
  eport: 9986 # service export port when as a reverse proxy
  mport: 8003 # agent manage port
  hport: 9987 # agent http forward proxy port
  log_dir: stdout
  registry: "direct-registry" # registry id for registering agent info
  application: "agent-test" # agent identify. for agent command notify and so on


http-locations: # client server 公用的配置，用来确定用哪个upstream，以及url重写相关的东西
  _: # domain name
    - match: /
      type: start
      upstream: _

motan-service: # server端配置
  test.domain:
    registry: direct-registry
    group: test.domain
    domain: _
    path: test
    export: "motan2:9989"
    provider: http
    serialization: simple
    proxyAddress: localhost:18001
    filter: "accessLog,metrics"
    requestTimeout: 2000
`

func TestRequestResponse(t *testing.T) {
	arguments := map[string]string{ // 参数 string map
		"source": "3206318534",
		"count":  "3000",
		"uid":    "3608383407",
	}
	resp, err := callTimeOut("127.0.0.1", "9989", "test", "test.domain", 30000, "", "/", []interface{}{arguments})
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	attachments := map[string]string{
		"HTTP_QueryString": "lalala",
		"Host":             "127.0.0.1",
		"HTTP_Method":      "POST",
	}
	resp, err = callTimeOutWithAttachment("127.0.0.1", "9989", "test", "test.domain", 30000, "", "/", []interface{}{arguments}, attachments)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	// other type of arguments
	argumentString := "argument"
	resp, err = callTimeOutWithAttachment("127.0.0.1", "9989", "test", "test.domain", 30000, "", "/", []interface{}{argumentString}, attachments)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	argumentByte := []byte{123}
	resp, err = callTimeOutWithAttachment("127.0.0.1", "9989", "test", "test.domain", 30000, "", "/", []interface{}{argumentByte}, attachments)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	argumentInt := 12345
	resp, err = callTimeOutWithAttachment("127.0.0.1", "9989", "test", "test.domain", 30000, "", "/", []interface{}{argumentInt}, attachments)
	assert.NotNil(t, err)
	assert.Nil(t, resp)
	// wrong number of argument
	resp, err = callTimeOutWrongArgumentCount("127.0.0.1", "9989", "test", "test.domain", 30000, "", "/", []interface{}{argumentString}, attachments)
	assert.NotNil(t, err)
	assert.Nil(t, resp)
}

func TestMain(m *testing.M) {
	go fastHttpServer()
	time.Sleep(time.Second * 1)
	serverAgent := motan.NewAgent(motan.GetDefaultExtFactory())
	serverConfig, _ := config.NewConfigFromReader(bytes.NewReader([]byte(serverConf)))
	go serverAgent.StartMotanAgentFromConfig(serverConfig)
	time.Sleep(time.Second * 3)
	code := m.Run()
	os.Exit(code)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "hello world")
}

func fastHttpServer() {
	http.HandleFunc("/", indexHandler)
	http.ListenAndServe(":18001", nil)
}

func callTimeOut(address string, port string, path string, group string, timeoutMilliSecond int, serviceName string, method string, argument ...[]interface{}) (interface{}, error) {
	var ep motancore.EndPoint
	clientExt := motan.GetDefaultExtFactory()
	info := fmt.Sprintf("motan2://%s:%s/%s?serialization=simple&maxRequestTimeout=30000&group=%s", address, port, path, group)
	u := motancore.FromExtInfo(info)
	ep = clientExt.GetEndPoint(u)
	if ep == nil {
		return nil, fmt.Errorf("get end point error")
	}
	ep.SetSerialization(motancore.GetSerialization(u, clientExt))
	motancore.Initialize(ep)
	request := &motancore.MotanRequest{}
	request.RequestID = rand.Uint64()
	request.ServiceName = path
	request.Method = method

	request.Attachment = motancore.NewStringMap(motancore.DefaultAttachmentSize)
	if timeoutMilliSecond > 0 {
		request.Attachment.Store(protocol.MTimeout, fmt.Sprintf("%d", timeoutMilliSecond))
	}
	if len(argument) > 0 {
		request.Arguments = argument[0]
	}
	resp := ep.Call(request)
	exception := resp.GetException()
	if exception != nil {
		errMsg := fmt.Sprintf("error: errCode: %d, errorMsg: %s, errType: %d", exception.ErrCode, exception.ErrMsg, exception.ErrType)
		err := fmt.Errorf(errMsg)
		return nil, err
	}
	returnValue := resp.GetValue()
	if returnValue != nil {
		return returnValue, nil
	}
	return nil, fmt.Errorf("wrong arguments")
}

func callTimeOutWithAttachment(address string, port string, path string, group string, timeoutMilliSecond int, serviceName string, method string, argument []interface{}, attachment map[string]string) (interface{}, error) {
	var ep motancore.EndPoint
	clientExt := motan.GetDefaultExtFactory()
	info := fmt.Sprintf("motan2://%s:%s/%s?serialization=simple&maxRequestTimeout=30000&group=%s", address, port, path, group)
	u := motancore.FromExtInfo(info)
	ep = clientExt.GetEndPoint(u)
	if ep == nil {
		return nil, fmt.Errorf("get end point error")
	}
	ep.SetSerialization(motancore.GetSerialization(u, clientExt))
	motancore.Initialize(ep)
	request := &motancore.MotanRequest{}
	request.RequestID = rand.Uint64()
	request.ServiceName = path
	request.Method = method

	request.Attachment = motancore.NewStringMap(motancore.DefaultAttachmentSize)
	if timeoutMilliSecond > 0 {
		request.Attachment.Store(protocol.MTimeout, fmt.Sprintf("%d", timeoutMilliSecond))
	}
	for i, j := range attachment {
		request.Attachment.Store(i, j)
	}

	request.Arguments = argument

	resp := ep.Call(request)
	exception := resp.GetException()
	if exception != nil {
		errMsg := fmt.Sprintf("error: errCode: %d, errorMsg: %s, errType: %d", exception.ErrCode, exception.ErrMsg, exception.ErrType)
		err := fmt.Errorf(errMsg)
		return nil, err
	}
	returnValue := resp.GetValue()
	if returnValue != nil {
		return returnValue, nil
	}
	return nil, fmt.Errorf("wrong arguments")
}

func callTimeOutWrongArgumentCount(address string, port string, path string, group string, timeoutMilliSecond int, serviceName string, method string, argument []interface{}, attachment map[string]string) (interface{}, error) {
	var ep motancore.EndPoint
	clientExt := motan.GetDefaultExtFactory()
	info := fmt.Sprintf("motan2://%s:%s/%s?serialization=simple&maxRequestTimeout=30000&group=%s", address, port, path, group)
	u := motancore.FromExtInfo(info)
	ep = clientExt.GetEndPoint(u)
	if ep == nil {
		return nil, fmt.Errorf("get end point error")
	}
	ep.SetSerialization(motancore.GetSerialization(u, clientExt))
	motancore.Initialize(ep)
	request := &motancore.MotanRequest{}
	request.RequestID = rand.Uint64()
	request.ServiceName = path
	request.Method = method

	request.Attachment = motancore.NewStringMap(motancore.DefaultAttachmentSize)
	if timeoutMilliSecond > 0 {
		request.Attachment.Store(protocol.MTimeout, fmt.Sprintf("%d", timeoutMilliSecond))
	}
	for i, j := range attachment {
		request.Attachment.Store(i, j)
	}

	request.Arguments = []interface{}{argument, argument}

	resp := ep.Call(request)
	exception := resp.GetException()
	if exception != nil {
		errMsg := fmt.Sprintf("error: errCode: %d, errorMsg: %s, errType: %d", exception.ErrCode, exception.ErrMsg, exception.ErrType)
		err := fmt.Errorf(errMsg)
		return nil, err
	}
	returnValue := resp.GetValue()
	if returnValue != nil {
		return returnValue, nil
	}
	return nil, fmt.Errorf("wrong arguments")
}
