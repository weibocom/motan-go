package http_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/weibocom/motan-go"
	"github.com/weibocom/motan-go/config"
	motancore "github.com/weibocom/motan-go/core"
	http2 "github.com/weibocom/motan-go/http"
	"github.com/weibocom/motan-go/protocol"
	"math/rand"
	"net/http"
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


http-locations: 
  _: # domain name
    - match: /
      type: start
      upstream: _

motan-service: 
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

var (
	httpRequestHeader http.Header
)

type APITestSuite struct {
	suite.Suite
}

func (s *APITestSuite) SetupTest() {
	go fastHttpServer()
	time.Sleep(time.Second * 1)
	serverAgent := motan.NewAgent(motan.GetDefaultExtFactory())
	serverConfig, _ := config.NewConfigFromReader(bytes.NewReader([]byte(serverConf)))
	go serverAgent.StartMotanAgentFromConfig(serverConfig)
	time.Sleep(time.Second * 3)
}

func TestAPITestSuite(t *testing.T) {
	suite.Run(t, new(APITestSuite))
}

func (s *APITestSuite) TestRequestResponse() {
	arguments := map[string]string{
		"source": "3206318534",
		"count":  "3000",
		"uid":    "3608383407",
	}
	resp, err := callTimeOut("127.0.0.1", "9989", "test", "test.domain", 30000, "", "/", []interface{}{arguments})
	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), resp)
	attachments := map[string]string{
		"HTTP_QueryString": "lalala",
		"Host":             "127.0.0.1",
		"HTTP_Method":      "POST",
	}
	resp, err = callTimeOutWithAttachment("127.0.0.1", "9989", "test", "test.domain", 30000, "", "/", []interface{}{arguments}, attachments)
	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), resp)
	// other type of arguments
	argumentString := "argument"
	resp, err = callTimeOutWithAttachment("127.0.0.1", "9989", "test", "test.domain", 30000, "", "/", []interface{}{argumentString}, attachments)
	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), resp)
	argumentByte := []byte{123}
	resp, err = callTimeOutWithAttachment("127.0.0.1", "9989", "test", "test.domain", 30000, "", "/", []interface{}{argumentByte}, attachments)
	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), resp)
	argumentInt := 12345
	resp, err = callTimeOutWithAttachment("127.0.0.1", "9989", "test", "test.domain", 30000, "", "/", []interface{}{argumentInt}, attachments)
	assert.NotNil(s.T(), err)
	assert.Nil(s.T(), resp)
	// wrong number of argument
	resp, err = callTimeOutWrongArgumentCount("127.0.0.1", "9989", "test", "test.domain", 30000, "", "/", []interface{}{argumentString}, attachments)
	assert.NotNil(s.T(), err)
	assert.Nil(s.T(), resp)
	// json content-type request
	bodyBytes,_ := json.Marshal(arguments)
	attachments[http2.HeaderContentType] = "application/json"
	resp, err = callTimeOutWithAttachment("127.0.0.1", "9989", "test", "test.domain", 30000, "", "/", []interface{}{bodyBytes}, attachments)
	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), resp)
	assert.Equal(s.T(), "application/json", httpRequestHeader.Get(http2.HeaderContentType))
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	httpRequestHeader = r.Header
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
