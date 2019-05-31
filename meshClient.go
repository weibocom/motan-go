package motan

import (
	"errors"
	"strconv"
	"time"

	"github.com/weibocom/motan-go/cluster"
	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
	"github.com/weibocom/motan-go/protocol"
)

const DefaultMeshRequestTimeout = 5 * time.Second
const DefaultMeshAddress = "127.0.0.1:9981"
const meshDirectRegistryKey = "mesh-registry"

type MeshClient struct {
	requestTimeout time.Duration
	application    string
	address        string
	serialization  string
	cluster        *cluster.MotanCluster
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
		c.serialization = "simple"
	}
	clusterURL := &core.URL{}
	clusterURL.Protocol = endpoint.Motan2
	clusterURL.PutParam(core.TimeOutKey, strconv.Itoa(int(c.requestTimeout/time.Millisecond)))
	clusterURL.PutParam(core.ApplicationKey, c.application)
	clusterURL.PutParam(core.ErrorCountThresholdKey, "0")
	clusterURL.PutParam(core.RegistryKey, meshDirectRegistryKey)
	meshRegistryURL := &core.URL{}
	meshRegistryURL.Protocol = "direct"
	meshRegistryURL.PutParam(core.AddressKey, c.address)
	context := &core.Context{}
	context.RegistryURLs = make(map[string]*core.URL)
	context.RegistryURLs[meshDirectRegistryKey] = meshRegistryURL
	c.cluster = cluster.NewCluster(context, GetDefaultExtFactory(), clusterURL, false)
}

func (c *MeshClient) BuildRequestWithGroup(service string, method string, args []interface{}, group string) core.Request {
	request := &core.MotanRequest{Method: method, ServiceName: service, Arguments: args, Attachment: core.NewStringMap(core.DefaultAttachmentSize)}
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
	return c.cluster.Call(request)
}
