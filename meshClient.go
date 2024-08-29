package motan

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/weibocom/motan-go/cluster"
	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
	mhttp "github.com/weibocom/motan-go/http"
	"github.com/weibocom/motan-go/protocol"
)

const (
	DefaultMeshRequestTimeout = 5 * time.Second
	DefaultMeshAddress        = "127.0.0.1:9981"
	DefaultMeshSerialization  = "simple"
)
const meshDirectRegistryKey = "mesh-registry"

type MeshClient struct {
	requestTimeout time.Duration
	application    string
	address        string
	serialization  string
	cluster        *cluster.MotanCluster
	httpClient     *fasthttp.Client
}

func NewMeshClient() *MeshClient {
	return &MeshClient{}
}

func (c *MeshClient) SetAddress(address string) {
	c.address = address
}

func (c *MeshClient) SetRequestTimeout(requestTimeout time.Duration) {
	c.requestTimeout = requestTimeout
}

func (c *MeshClient) SetSerialization(serialization string) {
	c.serialization = serialization
}

func (c *MeshClient) SetApplication(application string) {
	c.application = application
}

func (c *MeshClient) Initialize() {
	if c.requestTimeout == 0 {
		c.requestTimeout = DefaultMeshRequestTimeout
	}
	if c.address == "" {
		c.address = DefaultMeshAddress
	}
	if c.serialization == "" {
		c.serialization = DefaultMeshSerialization
	}
	c.httpClient = &fasthttp.Client{}
	clusterURL := &core.URL{}
	clusterURL.Protocol = endpoint.Motan2
	clusterURL.PutParam(core.TimeOutKey, strconv.Itoa(int(c.requestTimeout/time.Millisecond)))
	clusterURL.PutParam(core.ApplicationKey, c.application)
	clusterURL.PutParam(core.ErrorCountThresholdKey, "0")
	clusterURL.PutParam(core.RegistryKey, meshDirectRegistryKey)
	clusterURL.PutParam(core.ConnectRetryIntervalKey, "5000")
	clusterURL.PutParam(core.SerializationKey, c.serialization)
	clusterURL.PutParam(core.AsyncInitConnection, "false")
	meshRegistryURL := &core.URL{}
	meshRegistryURL.Protocol = "direct"
	meshRegistryURL.PutParam(core.AddressKey, c.address)
	context := &core.Context{}
	context.RegistryURLs = make(map[string]*core.URL)
	context.RegistryURLs[meshDirectRegistryKey] = meshRegistryURL
	c.cluster = cluster.NewCluster(context, GetDefaultExtFactory(), clusterURL, false)
}

func (c *MeshClient) Destroy() {
	c.cluster.Destroy()
}

func (c *MeshClient) BuildRequestWithGroup(service string, method string, args []interface{}, group string) core.Request {
	request := &core.MotanRequest{Method: method, ServiceName: service, Arguments: args, Attachment: core.NewStringMap(core.DefaultAttachmentSize)}
	request.RequestID = endpoint.GenerateRequestID()
	request.SetAttachment(protocol.MSource, c.application)
	request.SetAttachment(protocol.MGroup, group)
	request.SetAttachment(protocol.MPath, request.GetServiceName())
	return request
}

func (c *MeshClient) BuildRequest(service string, method string, args []interface{}) core.Request {
	return c.BuildRequestWithGroup(service, method, args, "")
}

func (c *MeshClient) Call(service string, method string, args []interface{}, reply interface{}) error {
	request := c.BuildRequest(service, method, args)
	response := c.BaseCall(request, reply)
	if response.GetException() != nil {
		return errors.New(response.GetException().ErrMsg)
	}
	return nil
}

func (c *MeshClient) BaseCall(request core.Request, reply interface{}) core.Response {
	rc := request.GetRPCContext(true)
	rc.Reply = reply
	response := c.cluster.Call(request)
	// none http call direct response
	if strings.Index(request.GetMethod(), "/") == -1 {
		return response
	}
	// http request fallback to http call if mesh can not connect
	if response.GetException() == nil || (response.GetException().ErrCode != core.ENoChannel && response.GetException().ErrCode != core.ENoEndpoints) {
		return response
	}
	httpRequest := fasthttp.AcquireRequest()
	httpResponse := fasthttp.AcquireResponse()
	// do not release http response
	defer fasthttp.ReleaseRequest(httpRequest)
	httpRequest.Header.Del("Host")
	httpRequest.SetHost(request.GetServiceName())
	httpRequest.URI().SetPath(request.GetMethod())
	err := mhttp.MotanRequestToFasthttpRequest(request, httpRequest, "GET")
	if err != nil {
		return getDefaultResponse(request.GetRequestID(), "bad motan-http request: "+err.Error())
	}
	err = c.httpClient.Do(httpRequest, httpResponse)
	if err != nil {
		return getDefaultResponse(request.GetRequestID(), "do http request failed : "+err.Error())
	}
	response = &core.MotanResponse{RequestID: request.GetRequestID()}
	mhttp.FasthttpResponseToMotanResponse(response, httpResponse)
	if replyPointer, ok := rc.Reply.(*[]byte); ok {
		*replyPointer = response.GetValue().([]byte)
	}
	if replyPointer, ok := rc.Reply.(*string); ok {
		*replyPointer = string(response.GetValue().([]byte))
	}
	return response
}
