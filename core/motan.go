package core

import (
	"container/ring"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/weibocom/motan-go/log"
)

type taskHandler func()

var (
	refreshTaskPool = make(chan taskHandler, 100)
	requestPool     = sync.Pool{New: func() interface{} {
		return &MotanRequest{
			RPCContext: &RPCContext{},
			Arguments:  []interface{}{},
		}
	}}
	responsePool = sync.Pool{New: func() interface{} {
		return &MotanResponse{
			RPCContext: &RPCContext{},
		}
	}}
)

func init() {
	go func() {
		for handler := range refreshTaskPool {
			func() {
				defer HandlePanic(nil)
				handler()
			}()
		}
	}()
}

const (
	DefaultAttachmentSize = 16
	ProtocolLocal         = "local"
)

var (
	registryGroupInfoMaxCacheTime        = time.Hour
	registryGroupServiceInfoMaxCacheTime = time.Hour
)

//-----------interface-------------

// Name is a interface can get and set name. especially for extension implements
type Name interface {
	GetName() string
}

// Identity : get id
type Identity interface {
	GetIdentity() string
}

// WithURL : can set and get URL
type WithURL interface {
	GetURL() *URL
	SetURL(url *URL)
}

// Attachment : can get, set attachments.
type Attachment interface {
	GetAttachments() *StringMap
	GetAttachment(key string) string
	SetAttachment(key string, value string)
}

// Destroyable : can destroy ....
type Destroyable interface {
	Destroy()
}

// Cloneable : can clone itself, the return type interface{} must be the type which implement this interface
type Cloneable interface {
	Clone() interface{}
}

// Caller : can process a motan request. the call maybe process from remote by endpoint, maybe process by some kinds of provider
type Caller interface {
	RuntimeInfo
	WithURL
	Status
	Call(request Request) Response
	Destroyable
}

// Request : motan request
type Request interface {
	Attachment
	Cloneable
	GetServiceName() string // service name  e.g. request path.or interface name
	GetMethod() string
	GetMethodDesc() string
	GetArguments() []interface{}
	GetRequestID() uint64
	GetRPCContext(canCreate bool) *RPCContext
	ProcessDeserializable(toTypes []interface{}) error
}

type FinishHandler interface {
	Handle()
}

type FinishHandleFunc func()

func (f FinishHandleFunc) Handle() {
	f()
}

// Response : motan response
type Response interface {
	Attachment
	GetValue() interface{}
	GetException() *Exception
	GetRequestID() uint64
	GetProcessTime() int64
	SetProcessTime(time int64)
	GetRPCContext(canCreate bool) *RPCContext
	ProcessDeserializable(toType interface{}) error
}

// Status : for cluster or endpoint to check is available
type Status interface {
	IsAvailable() bool
}

// EndPoint : can process a remote rpc call
type EndPoint interface {
	Name
	Caller
	SetSerialization(s Serialization)
	SetProxy(proxy bool)
}

// HaStrategy : high availability strategy
type HaStrategy interface {
	Name
	WithURL
	Call(request Request, loadBalance LoadBalance) Response
}

// LoadBalance : loadBalance for cluster
type LoadBalance interface {
	OnRefresh(endpoints []EndPoint)

	Select(request Request) EndPoint

	SelectArray(request Request) []EndPoint

	SetWeight(weight string)
}

// WeightLoadBalance : weight loadBalance for cluster
type WeightLoadBalance interface {
	LoadBalance
	NotifyWeightChange()
}

// DiscoverService : discover service for cluster
type DiscoverService interface {
	Subscribe(url *URL, listener NotifyListener)

	Unsubscribe(url *URL, listener NotifyListener)

	Discover(url *URL) []*URL
}

type GroupDiscoverableRegistry interface {
	Registry
	DiscoverAllGroups() ([]string, error)
}

type ServiceDiscoverableRegistry interface {
	Registry
	DiscoverAllServices(group string) ([]string, error)
}

// DiscoverCommand : discover command for client or agent
type DiscoverCommand interface {
	SubscribeCommand(url *URL, listener CommandNotifyListener)
	UnSubscribeCommand(url *URL, listener CommandNotifyListener)
	DiscoverCommand(url *URL) string
}

// RegisterService : register service for rpc server
type RegisterService interface {
	Register(serverURL *URL)
	UnRegister(serverURL *URL)
	Available(serverURL *URL)
	Unavailable(serverURL *URL)
	GetRegisteredServices() []*URL
}

// SnapshotService : start registry snapshot
type SnapshotService interface {
	StartSnapshot(conf *SnapshotConf)
}

// Registry : can subscribe or register service
type Registry interface {
	RuntimeInfo
	Name
	WithURL
	DiscoverService
	RegisterService
	SnapshotService
}

type RegistryStatusManager interface {
	GetRegistryStatus() map[string]*RegistryStatus
}

type RegistryStatus struct {
	Status   string
	Service  *URL
	Registry RegisterService
	ErrMsg   string
	IsCheck  bool
}

// NotifyListener : NotifyListener
type NotifyListener interface {
	Identity
	Notify(registryURL *URL, urls []*URL)
}

// CommandNotifyListener : support command notify
type CommandNotifyListener interface {
	Identity
	NotifyCommand(registryURL *URL, commandType int, commandInfo string)
}

// Filter : filter request or response in a call processing
type Filter interface {
	RuntimeInfo
	Name
	// filter must be prototype
	NewFilter(url *URL) Filter
	HasNext() bool
	GetIndex() int
	GetType() int32
}

// EndPointFilter : filter for endpoint
type EndPointFilter interface {
	Filter
	SetNext(nextFilter EndPointFilter)
	GetNext() EndPointFilter
	//Filter for endpoint
	Filter(caller Caller, request Request) Response
}

// ClusterFilter : filter for cluster
type ClusterFilter interface {
	Filter
	SetNext(nextFilter ClusterFilter)
	GetNext() ClusterFilter
	//Filter for Cluster
	Filter(haStrategy HaStrategy, loadBalance LoadBalance, request Request) Response
}

// Server : rpc server which listen port and process request
type Server interface {
	WithURL
	Name
	Destroyable
	RuntimeInfo
	SetMessageHandler(mh MessageHandler)
	GetMessageHandler() MessageHandler
	Open(block bool, proxy bool, handler MessageHandler, extFactory ExtensionFactory) error
	SetHeartbeat(b bool)
}

// Exporter : export and manage a service. one exporter bind with a service
type Exporter interface {
	RuntimeInfo
	Export(server Server, extFactory ExtensionFactory, context *Context) error
	Unexport() error
	SetProvider(provider Provider)
	GetProvider() Provider
	Available()
	Unavailable()
	IsAvailable() bool
	WithURL
}

// Provider : service provider
type Provider interface {
	SetService(s interface{})
	Caller
	GetPath() string
}

// MessageHandler : handler message(request) for Server
type MessageHandler interface {
	Name
	RuntimeInfo
	Call(request Request) (res Response)
	AddProvider(p Provider) error
	RmProvider(p Provider)
	GetProvider(serviceName string) Provider
}

// Serialization : Serialization
type Serialization interface {
	GetSerialNum() int
	Serialize(v interface{}) ([]byte, error)
	DeSerialize(b []byte, v interface{}) (interface{}, error)
	SerializeMulti(v []interface{}) ([]byte, error)
	DeSerializeMulti(b []byte, v []interface{}) ([]interface{}, error)
}

// ExtensionFactory : can regiser and get all kinds of extension implements.
type ExtensionFactory interface {
	RuntimeInfo
	GetHa(url *URL) HaStrategy
	GetLB(url *URL) LoadBalance
	GetFilter(name string) Filter
	GetRegistry(url *URL) Registry
	GetEndPoint(url *URL) EndPoint
	GetProvider(url *URL) Provider
	GetServer(url *URL) Server
	GetMessageHandler(name string) MessageHandler
	GetSerialization(name string, id int) Serialization
	RegistExtFilter(name string, newFilter DefaultFilterFunc)
	RegistExtHa(name string, newHa NewHaFunc)
	RegistExtLb(name string, newLb NewLbFunc)
	RegistExtEndpoint(name string, newEndpoint NewEndpointFunc)
	RegistExtProvider(name string, newProvider NewProviderFunc)
	RegistExtRegistry(name string, newRegistry NewRegistryFunc)
	RegistExtServer(name string, newServer NewServerFunc)
	RegistryExtMessageHandler(name string, newMessage NewMessageHandlerFunc)
	RegistryExtSerialization(name string, id int, newSerialization NewSerializationFunc)
}

// Initializable :Initializable
type Initializable interface {
	Initialize()
}

// SetContext :SetContext
type SetContext interface {
	SetContext(context *Context)
}

// Initialize : Initialize if implement Initializable
func Initialize(s interface{}) {
	if init, ok := s.(Initializable); ok {
		init.Initialize()
	}
}

// CanSetContext :CanSetContext
func CanSetContext(s interface{}, context *Context) {
	if sc, ok := s.(SetContext); ok {
		sc.SetContext(context)
	}
}

//-------------models--------------

// SnapshotConf is model for registry snapshot config.
type SnapshotConf struct {
	// SnapshotInterval is the interval of creating snapshot
	SnapshotInterval time.Duration
	SnapshotDir      string
}

// Exception :Exception
type Exception struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
	ErrType int    `json:"errtype"`
}

// RPCContext : Context for RPC call
type RPCContext struct {
	ExtFactory      ExtensionFactory
	OriginalMessage interface{}
	Oneway          bool
	Proxy           bool
	GzipSize        int
	BodySize        int
	SerializeNum    int
	Serialized      bool

	// for call
	AsyncCall bool
	Result    *AsyncResult
	Reply     interface{}
	// various time, it's owned by motan request context
	RequestSendTime     time.Time
	RequestReceiveTime  time.Time
	ResponseSendTime    time.Time
	ResponseReceiveTime time.Time

	FinishHandlers []FinishHandler

	// trace context
	Tc *TraceContext

	// ----  internal vars ----
	IsMotanV1  bool
	RemoteAddr string // remote address
}

func (c *RPCContext) Reset() {
	// because there is a binding between RPCContext and request/response,
	// some attributes such as RequestSendTime、RequestReceiveTime will be reset by request/response
	// therefore, these attributes do not need to be reset here.
	c.ExtFactory = nil
	c.OriginalMessage = nil
	c.Oneway = false
	c.Proxy = false
	c.GzipSize = 0
	c.BodySize = 0
	c.SerializeNum = 0
	c.Serialized = false
	c.Result = nil
	c.Reply = nil
	c.FinishHandlers = c.FinishHandlers[:0]
	c.Tc = nil
	c.IsMotanV1 = false
	c.RemoteAddr = ""
}

func (c *RPCContext) AddFinishHandler(handler FinishHandler) {
	c.FinishHandlers = append(c.FinishHandlers, handler)
}

func (c *RPCContext) OnFinish() {
	for _, h := range c.FinishHandlers {
		h.Handle()
	}
}

// AsyncResult : async call result
type AsyncResult struct {
	Done  chan *AsyncResult
	Error error
}

// DeserializableValue : for lazy deserialize
type DeserializableValue struct {
	Serialization Serialization
	Body          []byte
}

// Deserialize : Deserialize
func (d *DeserializableValue) Deserialize(v interface{}) (interface{}, error) {
	if d.Serialization == nil {
		return nil, errors.New("deserialize fail in DeserializableValue, Serialization is nil")
	}
	return d.Serialization.DeSerialize(d.Body, v)
}

// DeserializeMulti : DeserializeMulti
func (d *DeserializableValue) DeserializeMulti(v []interface{}) ([]interface{}, error) {
	if d.Serialization == nil {
		return nil, errors.New("deserialize fail in DeserializableValue, Serialization is nil")
	}
	return d.Serialization.DeSerializeMulti(d.Body, v)
}

// MotanRequest : Request default implement
type MotanRequest struct {
	RequestID   uint64
	ServiceName string
	Method      string
	MethodDesc  string
	Arguments   []interface{}
	Attachment  *StringMap
	RPCContext  *RPCContext
	mu          sync.Mutex
}

func AcquireMotanRequest() *MotanRequest {
	return requestPool.Get().(*MotanRequest)
}

func ReleaseMotanRequest(req *MotanRequest) {
	if req != nil {
		req.Reset()
		requestPool.Put(req)
	}
}

// Reset reset motan request
func (req *MotanRequest) Reset() {
	req.Method = ""
	req.RequestID = 0
	req.ServiceName = ""
	req.MethodDesc = ""
	req.RPCContext.Reset()
	req.Attachment = nil
	req.Arguments = req.Arguments[:0]
}

// GetAttachment GetAttachment
func (req *MotanRequest) GetAttachment(key string) string {
	if req.Attachment == nil {
		return ""
	}
	return req.Attachment.LoadOrEmpty(key)
}

// SetAttachment : SetAttachment
func (req *MotanRequest) SetAttachment(key string, value string) {
	req.GetAttachments().Store(key, value)
}

// GetServiceName GetServiceName
func (req *MotanRequest) GetServiceName() string {
	return req.ServiceName
}

// GetMethod GetMethod
func (req *MotanRequest) GetMethod() string {
	return req.Method
}

// GetMethodDesc GetMethodDesc
func (req *MotanRequest) GetMethodDesc() string {
	return req.MethodDesc
}

func (req *MotanRequest) GetArguments() []interface{} {
	return req.Arguments
}

func (req *MotanRequest) GetRequestID() uint64 {
	return req.RequestID
}

func (req *MotanRequest) SetArguments(arguments []interface{}) {
	req.Arguments = arguments
}

func (req *MotanRequest) GetAttachments() *StringMap {
	attachment := (*StringMap)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&req.Attachment))))
	if attachment != nil {
		return attachment
	}
	req.mu.Lock()
	defer req.mu.Unlock()
	if req.Attachment == nil {
		attachment = NewStringMap(DefaultAttachmentSize)
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&req.Attachment)), unsafe.Pointer(attachment))
	} else {
		attachment = req.Attachment
	}
	return attachment
}

func (req *MotanRequest) GetRPCContext(canCreate bool) *RPCContext {
	if req.RPCContext == nil && canCreate {
		req.RPCContext = &RPCContext{}
	}
	return req.RPCContext
}

func (req *MotanRequest) Clone() interface{} {
	newRequest := &MotanRequest{
		RequestID:   req.RequestID,
		ServiceName: req.ServiceName,
		Method:      req.Method,
		MethodDesc:  req.MethodDesc,
		Arguments:   req.Arguments,
	}
	if req.Attachment != nil {
		newRequest.Attachment = req.Attachment.Copy()
	}
	if req.RPCContext != nil {
		newRequest.RPCContext = &RPCContext{
			ExtFactory:          req.RPCContext.ExtFactory,
			Oneway:              req.RPCContext.Oneway,
			Proxy:               req.RPCContext.Proxy,
			GzipSize:            req.RPCContext.GzipSize,
			SerializeNum:        req.RPCContext.SerializeNum,
			Serialized:          req.RPCContext.Serialized,
			Result:              req.RPCContext.Result,
			Reply:               req.RPCContext.Reply,
			RequestSendTime:     req.RPCContext.RequestSendTime,
			RequestReceiveTime:  req.RPCContext.RequestReceiveTime,
			ResponseSendTime:    req.RPCContext.ResponseSendTime,
			ResponseReceiveTime: req.RPCContext.ResponseReceiveTime,
			FinishHandlers:      req.RPCContext.FinishHandlers,
			Tc:                  req.RPCContext.Tc,
		}
		if req.RPCContext.OriginalMessage != nil {
			if oldMessage, ok := req.RPCContext.OriginalMessage.(Cloneable); ok {
				newRequest.RPCContext.OriginalMessage = oldMessage.Clone()
			} else {
				newRequest.RPCContext.OriginalMessage = req.RPCContext.OriginalMessage
			}
		}
	}
	return newRequest
}

// ProcessDeserializable : DeserializableValue to real params according toType
// some serialization can deserialize without toType, so nil toType can be accepted in these serializations
func (req *MotanRequest) ProcessDeserializable(toTypes []interface{}) error {
	if req.GetArguments() != nil && len(req.GetArguments()) == 1 {
		if d, ok := req.GetArguments()[0].(*DeserializableValue); ok {
			v, err := d.DeserializeMulti(toTypes)
			if err != nil {
				return err
			}
			req.SetArguments(v)
		}
	}
	return nil
}

type MotanResponse struct {
	RequestID   uint64
	Value       interface{}
	Exception   *Exception
	ProcessTime int64
	Attachment  *StringMap
	RPCContext  *RPCContext
	mu          sync.Mutex
}

func AcquireMotanResponse() *MotanResponse {
	return responsePool.Get().(*MotanResponse)
}

func ReleaseMotanResponse(m *MotanResponse) {
	if m != nil {
		m.Reset()
		responsePool.Put(m)
	}
}

func (res *MotanResponse) Reset() {
	res.RequestID = 0
	res.Value = nil
	res.Exception = nil
	res.ProcessTime = 0
	res.Attachment = nil
	res.RPCContext.Reset()
}

func (res *MotanResponse) GetAttachment(key string) string {
	if res.Attachment == nil {
		return ""
	}
	return res.Attachment.LoadOrEmpty(key)
}

func (res *MotanResponse) SetAttachment(key string, value string) {
	res.GetAttachments().Store(key, value)
}

func (res *MotanResponse) GetValue() interface{} {
	return res.Value
}

func (res *MotanResponse) GetException() *Exception {
	return res.Exception
}

func (res *MotanResponse) GetRequestID() uint64 {
	return res.RequestID
}

func (res *MotanResponse) GetProcessTime() int64 {
	return res.ProcessTime
}

func (res *MotanResponse) GetAttachments() *StringMap {
	attachment := (*StringMap)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&res.Attachment))))
	if attachment != nil {
		return attachment
	}
	res.mu.Lock()
	defer res.mu.Unlock()
	if res.Attachment == nil {
		attachment = NewStringMap(DefaultAttachmentSize)
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&res.Attachment)), unsafe.Pointer(attachment))
	} else {
		attachment = res.Attachment
	}
	return attachment
}

func (res *MotanResponse) GetRPCContext(canCreate bool) *RPCContext {
	if res.RPCContext == nil && canCreate {
		res.RPCContext = &RPCContext{}
	}
	return res.RPCContext
}

func (res *MotanResponse) SetProcessTime(time int64) {
	res.ProcessTime = time
}

// ProcessDeserializable : same with MotanRequest
func (res *MotanResponse) ProcessDeserializable(toType interface{}) error {
	if res.GetValue() != nil {
		if d, ok := res.GetValue().(*DeserializableValue); ok {
			v, err := d.Deserialize(toType)
			if err != nil {
				return err
			}
			res.Value = v
		}
	}
	return nil
}

func BuildExceptionResponse(requestid uint64, e *Exception) *MotanResponse {
	resp := AcquireMotanResponse()
	resp.RequestID = requestid
	resp.Exception = e
	return resp
}

// extensions factory-func

type DefaultFilterFunc func() Filter
type NewHaFunc func(url *URL) HaStrategy
type NewLbFunc func(url *URL) LoadBalance
type NewEndpointFunc func(url *URL) EndPoint
type NewProviderFunc func(url *URL) Provider
type NewRegistryFunc func(url *URL) Registry
type NewServerFunc func(url *URL) Server
type NewMessageHandlerFunc func() MessageHandler
type NewSerializationFunc func() Serialization

type DefaultExtensionFactory struct {
	// factories
	filterFactories   map[string]DefaultFilterFunc
	haFactories       map[string]NewHaFunc
	lbFactories       map[string]NewLbFunc
	endpointFactories map[string]NewEndpointFunc
	providerFactories map[string]NewProviderFunc
	registryFactories map[string]NewRegistryFunc
	servers           map[string]NewServerFunc
	messageHandlers   map[string]NewMessageHandlerFunc
	serializations    map[string]NewSerializationFunc

	// singleton instance
	registries      map[string]Registry
	newRegistryLock sync.Mutex
}

func (d *DefaultExtensionFactory) GetRuntimeInfo() map[string]interface{} {
	info := map[string]interface{}{}
	// registries runtime info
	d.newRegistryLock.Lock()
	defer d.newRegistryLock.Unlock()
	registriesInfo := map[string]interface{}{}
	for s, registry := range d.registries {
		registriesInfo[s] = registry.GetRuntimeInfo()
	}
	info[RuntimeRegistriesKey] = registriesInfo

	return info
}

func (d *DefaultExtensionFactory) GetHa(url *URL) HaStrategy {
	haName := url.GetParam(Hakey, "failover")
	if newHa, ok := d.haFactories[haName]; ok {
		return newHa(url)
	}
	vlog.Errorf("HaStrategy name %s is not found in DefaultExtensionFactory!", haName)
	return nil
}

func (d *DefaultExtensionFactory) GetLB(url *URL) LoadBalance {
	lbName := url.GetParam(Lbkey, "random")
	if newLb, ok := d.lbFactories[lbName]; ok {
		return newLb(url)
	}
	vlog.Errorf("LoadBalance name %s is not found in DefaultExtensionFactory!", lbName)
	return nil
}

func (d *DefaultExtensionFactory) GetFilter(name string) Filter {
	if newDefault, ok := d.filterFactories[strings.TrimSpace(name)]; ok {
		return newDefault()
	}
	vlog.Errorf("filter name %s is not found in DefaultExtensionFactory!", name)
	return nil
}

func (d *DefaultExtensionFactory) GetRegistry(url *URL) Registry {
	key := url.GetIdentity()
	d.newRegistryLock.Lock()
	defer d.newRegistryLock.Unlock()
	if registry, exist := d.registries[key]; exist {
		return registry
	}
	if newRegistry, ok := d.registryFactories[url.Protocol]; ok {
		registry := newRegistry(url)
		Initialize(registry)
		d.registries[key] = registry
		return registry
	}
	vlog.Errorf("Registry name %s is not found in DefaultExtensionFactory!", url.Protocol)
	return nil
}

func (d *DefaultExtensionFactory) GetEndPoint(url *URL) EndPoint {
	if newEp, ok := d.endpointFactories[url.Protocol]; ok {
		endpoint := newEp(url)
		return endpoint
	}
	vlog.Errorf("EndPoint(protocol) name %s is not found in DefaultExtensionFactory!", url.Protocol)
	return nil
}

func (d *DefaultExtensionFactory) GetProvider(url *URL) Provider {
	pName := url.GetParam(ProviderKey, "")
	if pName == "" {
		if proxy := url.GetParam(ProxyKey, ""); proxy != "" {
			pName, _, _ = ParseExportInfo(proxy)
		} else {
			pName = "default"
		}
	}
	if newProviderFunc, ok := d.providerFactories[pName]; ok {
		return newProviderFunc(url)
	}
	vlog.Errorf("provider name %s is not found in DefaultExtensionFactory!", pName)
	return nil
}

func (d *DefaultExtensionFactory) GetServer(url *URL) Server {
	sname := url.Protocol
	if sname == "" {
		sname = "motan2"
		vlog.Warningln("not find server key. motan2 server will used.")
	}
	if f, ok := d.servers[sname]; ok {
		s := f(url)
		Initialize(s)
		return s
	}
	vlog.Errorf("server name %s is not found in DefaultExtensionFactory!", sname)
	return nil
}

func (d *DefaultExtensionFactory) GetMessageHandler(name string) MessageHandler {
	if newMessageHandler, ok := d.messageHandlers[strings.TrimSpace(name)]; ok {
		handler := newMessageHandler()
		Initialize(handler)
		return handler
	}
	vlog.Errorf("messageHandler name %s is not found in DefaultExtensionFactory!", name)
	return nil
}

func (d *DefaultExtensionFactory) GetSerialization(name string, id int) Serialization {
	if name != "" {
		if newSerialization, ok := d.serializations[strings.TrimSpace(name)]; ok {
			return newSerialization()
		}
	} else if id > -1 {
		if newSerialization, ok := d.serializations[strconv.Itoa(id)]; ok {
			return newSerialization()
		}
	}
	return nil
}

func (d *DefaultExtensionFactory) RegistExtFilter(name string, newFilter DefaultFilterFunc) {
	// 覆盖方式
	d.filterFactories[name] = newFilter
}

func (d *DefaultExtensionFactory) RegistExtHa(name string, newHa NewHaFunc) {
	d.haFactories[name] = newHa
}

func (d *DefaultExtensionFactory) RegistExtLb(name string, newLb NewLbFunc) {
	d.lbFactories[name] = newLb
}

func (d *DefaultExtensionFactory) RegistExtEndpoint(name string, newEndpoint NewEndpointFunc) {
	d.endpointFactories[name] = newEndpoint
}

func (d *DefaultExtensionFactory) RegistExtProvider(name string, newProvider NewProviderFunc) {
	d.providerFactories[name] = newProvider
}

func (d *DefaultExtensionFactory) RegistExtRegistry(name string, newRegistry NewRegistryFunc) {
	d.registryFactories[name] = newRegistry
}

func (d *DefaultExtensionFactory) RegistExtServer(name string, newServer NewServerFunc) {
	d.servers[name] = newServer
}

func (d *DefaultExtensionFactory) RegistryExtMessageHandler(name string, newMessage NewMessageHandlerFunc) {
	d.messageHandlers[name] = newMessage
}

func (d *DefaultExtensionFactory) RegistryExtSerialization(name string, id int, newSerialization NewSerializationFunc) {
	d.serializations[name] = newSerialization
	d.serializations[strconv.Itoa(id)] = newSerialization
}

func (d *DefaultExtensionFactory) Initialize() {
	d.filterFactories = make(map[string]DefaultFilterFunc)
	d.haFactories = make(map[string]NewHaFunc)
	d.lbFactories = make(map[string]NewLbFunc)
	d.endpointFactories = make(map[string]NewEndpointFunc)
	d.providerFactories = make(map[string]NewProviderFunc)
	d.registryFactories = make(map[string]NewRegistryFunc)
	d.servers = make(map[string]NewServerFunc)
	d.registries = make(map[string]Registry)
	d.messageHandlers = make(map[string]NewMessageHandlerFunc)
	d.serializations = make(map[string]NewSerializationFunc)
}

var (
	lef = new(lastEndPointFilter)
	lcf = new(lastClusterFilter)
)

func GetLastEndPointFilter() EndPointFilter {
	return lef
}

func GetLastClusterFilter() ClusterFilter {
	return lcf
}

type lastEndPointFilter struct{}

func (l *lastEndPointFilter) GetRuntimeInfo() map[string]interface{} {
	info := map[string]interface{}{
		RuntimeNameKey:  l.GetName(),
		RuntimeIndexKey: l.GetIndex(),
		RuntimeTypeKey:  l.GetType(),
	}
	return info
}

func (l *lastEndPointFilter) GetName() string {
	return "lastEndPointFilter"
}

func (l *lastEndPointFilter) NewFilter(url *URL) Filter {
	return GetLastEndPointFilter()
}

func (l *lastEndPointFilter) Filter(caller Caller, request Request) Response {
	if request.GetRPCContext(true).Tc != nil {
		request.GetRPCContext(true).Tc.PutReqSpan(&Span{Name: EpFilterEnd, Addr: caller.GetURL().GetAddressStr(), Time: time.Now()})
	}
	resp := caller.Call(request)
	if caller.GetURL() != nil {
		resp.GetRPCContext(true).RemoteAddr = net.JoinHostPort(caller.GetURL().Host, strconv.Itoa(caller.GetURL().Port))
	}
	return resp
}

func (l *lastEndPointFilter) HasNext() bool {
	return false
}

func (l *lastEndPointFilter) SetNext(nextFilter EndPointFilter) {
	vlog.Errorf("should not set next in lastEndPointFilter! filer:%s", nextFilter.GetName())
}

func (l *lastEndPointFilter) GetNext() EndPointFilter {
	return nil
}

func (l *lastEndPointFilter) GetIndex() int {
	return 100
}

func (l *lastEndPointFilter) GetType() int32 {
	return EndPointFilterType
}

type lastClusterFilter struct{}

func (l *lastClusterFilter) GetRuntimeInfo() map[string]interface{} {
	return map[string]interface{}{
		RuntimeNameKey:  l.GetName(),
		RuntimeIndexKey: l.GetIndex(),
		RuntimeTypeKey:  l.GetType(),
	}
}

func (l *lastClusterFilter) GetName() string {
	return "lastClusterFilter"
}

func (l *lastClusterFilter) NewFilter(url *URL) Filter {
	return GetLastClusterFilter()
}

func (l *lastClusterFilter) Filter(haStrategy HaStrategy, loadBalance LoadBalance, request Request) Response {
	if request.GetRPCContext(true).Tc != nil {
		// clusterFilter end
		request.GetRPCContext(true).Tc.PutReqSpan(&Span{Name: ClFilter, Time: time.Now()})
	}
	response := haStrategy.Call(request, loadBalance)
	if request.GetRPCContext(true).Tc != nil {
		// endpointFilter end
		request.GetRPCContext(true).Tc.PutResSpan(&Span{Name: EpFilterEnd, Time: time.Now()})
	}
	return response
}

func (l *lastClusterFilter) HasNext() bool {
	return false
}

func (l *lastClusterFilter) SetNext(nextFilter ClusterFilter) {
	vlog.Errorf("should not set next in lastClusterFilter! filer:%s", nextFilter.GetName())
}

func (l *lastClusterFilter) GetNext() ClusterFilter {
	return nil
}

func (l *lastClusterFilter) GetIndex() int {
	return 100
}

func (l *lastClusterFilter) GetType() int32 {
	return ClusterFilterType
}

type FilterEndPoint struct {
	URL           *URL
	Filter        EndPointFilter
	StatusFilters []Status
	Caller        Caller
}

func (f *FilterEndPoint) GetRuntimeInfo() map[string]interface{} {
	if f.Caller != nil {
		return f.Caller.GetRuntimeInfo()
	}
	return map[string]interface{}{}
}

func (f *FilterEndPoint) Call(request Request) Response {
	if request.GetRPCContext(true).Tc != nil {
		request.GetRPCContext(true).Tc.PutReqSpan(&Span{Name: EpFilterStart, Addr: f.GetURL().GetAddressStr(), Time: time.Now()})
	}
	return f.Filter.Filter(f.Caller, request)
}

func (f *FilterEndPoint) GetURL() *URL {
	return f.URL
}

func (f *FilterEndPoint) SetURL(url *URL) {
	f.URL = url
}

func (f *FilterEndPoint) GetName() string {
	return "FilterEndPoint"
}

func (f *FilterEndPoint) Destroy() {
	if f.Caller != nil {
		f.Caller.Destroy()
	}
}

func (f *FilterEndPoint) SetProxy(proxy bool) {}

func (f *FilterEndPoint) SetSerialization(s Serialization) {}

func (f *FilterEndPoint) IsAvailable() bool {
	if f.StatusFilters != nil && len(f.StatusFilters) > 0 {
		for i := len(f.StatusFilters) - 1; i >= 0; i-- {
			if !f.StatusFilters[i].IsAvailable() {
				return false
			}
		}
	}
	return f.Caller.IsAvailable()
}

type registryGroupCacheInfo struct {
	gr          GroupDiscoverableRegistry
	lastUpdTime atomic.Value //time.Time
	groups      atomic.Value //[]string
	lock        sync.Mutex
}

func newRegistryGroupCacheInfo(gr GroupDiscoverableRegistry) *registryGroupCacheInfo {
	c := &registryGroupCacheInfo{gr: gr}
	c.lastUpdTime.Store(time.Time{})
	c.groups.Store([]string(nil))
	return c

}

func (c *registryGroupCacheInfo) getGroups() []string {
	if time.Now().Sub(c.lastUpdTime.Load().(time.Time)) > registryGroupInfoMaxCacheTime {
		c.lock.Lock()
		defer c.lock.Unlock()
		groups, err := c.gr.DiscoverAllGroups()
		if err != nil {
			return c.groups.Load().([]string)
		}
		c.groups.Store(groups)
		c.lastUpdTime.Store(time.Now())
	}
	return c.groups.Load().([]string)
}

type registryGroupCache struct {
	cachedGroups map[string]*registryGroupCacheInfo
	lock         sync.Mutex
}

func (rc *registryGroupCache) getGroups(gr GroupDiscoverableRegistry) []string {
	key := gr.GetURL().GetIdentity()
	rc.lock.Lock()
	cacheInfo := rc.cachedGroups[key]
	if cacheInfo == nil {
		cacheInfo = newRegistryGroupCacheInfo(gr)
		rc.cachedGroups[key] = cacheInfo
	}
	rc.lock.Unlock()
	return cacheInfo.getGroups()
}

var globalRegistryGroupCache = registryGroupCache{cachedGroups: make(map[string]*registryGroupCacheInfo)}

func GetAllGroups(gr GroupDiscoverableRegistry) []string {
	return globalRegistryGroupCache.getGroups(gr)
}

type registryGroupServiceCacheInfo struct {
	sr          ServiceDiscoverableRegistry
	group       string
	lastUpdTime atomic.Value // time.Time
	services    atomic.Value // []string
	serviceMap  atomic.Value // map[string]string
	lock        sync.Mutex
}

func newRegistryGroupServiceCacheInfo(sr ServiceDiscoverableRegistry, group string) *registryGroupServiceCacheInfo {
	c := &registryGroupServiceCacheInfo{sr: sr, group: group}
	c.services.Store([]string(nil))
	c.serviceMap.Store(map[string]string(nil))
	c.lastUpdTime.Store(time.Time{})
	return c
}

func (c *registryGroupServiceCacheInfo) getServices() ([]string, map[string]string) {
	if time.Now().Sub(c.lastUpdTime.Load().(time.Time)) >= registryGroupServiceInfoMaxCacheTime {
		select {
		case refreshTaskPool <- func() { c.refreshServices() }:
		default:
			vlog.Warningf("Task pool is full, refresh service of group [%s] delay", c.group)
		}
	}
	return c.services.Load().([]string), c.serviceMap.Load().(map[string]string)
}

func (c *registryGroupServiceCacheInfo) refreshServices() {
	c.lock.Lock()
	defer c.lock.Unlock()
	// TODO: maybe we just need refresh services at startup
	if time.Now().Sub(c.lastUpdTime.Load().(time.Time)) < registryGroupServiceInfoMaxCacheTime {
		return
	}
	services, err := c.sr.DiscoverAllServices(c.group)
	if err != nil {
		return
	}
	c.services.Store(services)
	serviceMap := make(map[string]string, len(services))
	for _, service := range services {
		serviceMap[service] = service
	}
	c.serviceMap.Store(serviceMap)
	c.lastUpdTime.Store(time.Now())
}

type registryGroupServiceCache struct {
	cachedInfos sync.Map
	lock        sync.Mutex
}

func (rc *registryGroupServiceCache) getServices(sr ServiceDiscoverableRegistry, group string) ([]string, map[string]string) {
	// TODO: check the group is valid
	key := sr.GetURL().GetIdentity() + "_" + group
	cacheInfo, ok := rc.cachedInfos.Load(key)
	if !ok {
		rc.lock.Lock()
		defer rc.lock.Unlock()
		cacheInfo, ok = rc.cachedInfos.Load(key)
		if !ok {
			serviceCacheInfo := newRegistryGroupServiceCacheInfo(sr, group)
			serviceCacheInfo.refreshServices()
			rc.cachedInfos.Store(key, serviceCacheInfo)
			cacheInfo = serviceCacheInfo
		}
	}
	return cacheInfo.(*registryGroupServiceCacheInfo).getServices()
}

var globalRegistryGroupServiceCache registryGroupServiceCache

func ServiceInGroup(sr ServiceDiscoverableRegistry, group string, service string) bool {
	_, serviceMap := globalRegistryGroupServiceCache.getServices(sr, group)
	if serviceMap == nil {
		return false
	}
	if _, ok := serviceMap[service]; ok {
		return true
	}
	return false
}

var localProviders = NewCopyOnWriteMap()

func RegistLocalProvider(service string, provider Provider) {
	localProviders.Store(service, provider)
}

func GetLocalProvider(service string) Provider {
	if p, ok := localProviders.Load(service); ok {
		return p.(Provider)
	}
	return nil
}

//-----------RuntimeInfo interface-------------

// RuntimeInfo : output runtime information
type RuntimeInfo interface {
	GetRuntimeInfo() map[string]interface{}
}

// GetRuntimeInfo : call s.GetRuntimeInfo
func GetRuntimeInfo(s interface{}) map[string]interface{} {
	if sc, ok := s.(RuntimeInfo); ok {
		return sc.GetRuntimeInfo()
	}
	return map[string]interface{}{}
}

// -----------CircularRecorder-------------
type circularRecorderItem struct {
	timestamp int64
	value     interface{}
}

type CircularRecorder struct {
	ring   *ring.Ring
	lock   sync.RWMutex
	keyBuf []byte
}

func NewCircularRecorder(size int) *CircularRecorder {
	return &CircularRecorder{
		ring:   ring.New(size),
		lock:   sync.RWMutex{},
		keyBuf: make([]byte, 0, 18),
	}
}

func (c *CircularRecorder) AddRecord(item interface{}) {
	if item == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	c.ring.Value = &circularRecorderItem{
		timestamp: time.Now().UnixNano() / 1e6,
		value:     item,
	}
	c.ring = c.ring.Next()
}

func (c *CircularRecorder) GetRecords() map[string]interface{} {
	c.lock.RLock()
	defer c.lock.RUnlock()
	records := make(map[string]interface{})
	idx := int64(0)
	c.ring.Do(func(i interface{}) {
		if i != nil {
			item, ok := i.(*circularRecorderItem)
			if ok {
				records[c.generateRecordKey(idx, item.timestamp)] = item.value
				idx++
			}
		}
	})
	return records
}

func (c *CircularRecorder) generateRecordKey(idx, timestamp int64) string {
	c.keyBuf = c.keyBuf[:0]
	c.keyBuf = strconv.AppendInt(c.keyBuf, idx, 10)
	c.keyBuf = append(c.keyBuf, ':')
	c.keyBuf = strconv.AppendInt(c.keyBuf, timestamp, 10)
	return string(c.keyBuf)
}
