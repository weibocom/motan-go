package core

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/weibocom/motan-go/log"
)

//--------------const--------------
// exception type
const (
	FrameworkException = iota
	// ServiceException : exception by service call
	ServiceException
	// BizException : exception by service implements
	BizException
)

const (
	// EndPointFilterType filter for endpoint
	EndPointFilterType = iota
	// ClusterFilterType filter for cluster
	ClusterFilterType
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
	GetAttachments() map[string]string
	GetAttachment(key string) string
	SetAttachment(key string, value string)
}

// Destroyable : can destroy ....
type Destroyable interface {
	Destroy()
}

// Caller : can process a motan request. the call maybe process from remote by endpoint, maybe process by some kinds of provider
type Caller interface {
	WithURL
	Status
	Call(request Request) Response
	Destroyable
}

// Request : motan request
type Request interface {
	Attachment
	GetServiceName() string // service name  e.g. request path.or interface name
	GetMethod() string
	GetMethodDesc() string
	GetArguments() []interface{}
	GetRequestID() uint64
	GetRPCContext(canCreate bool) *RPCContext
	ProcessDeserializable(toTypes []interface{}) error
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

// DiscoverService : discover service for cluster
type DiscoverService interface {
	Subscribe(url *URL, listener NotifyListener)

	Unsubscribe(url *URL, listener NotifyListener)

	Discover(url *URL) []*URL
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
	Name
	WithURL
	DiscoverService
	RegisterService
	SnapshotService
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
	SetMessageHandler(mh MessageHandler)
	GetMessageHandler() MessageHandler
	Open(block bool, proxy bool, handler MessageHandler, extFactory ExtentionFactory) error
}

// Exporter : export and manage a service. one exporter bind with a service
type Exporter interface {
	Export(server Server) error
	Unexport() error
	SetProvider(provider Provider)
	GetProvider() Provider
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

// ExtentionFactory : can regiser and get all kinds of extension implements.
type ExtentionFactory interface {
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
	ExtFactory      ExtentionFactory
	OriginalMessage interface{}
	Oneway          bool
	Proxy           bool
	GzipSize        int

	// for call
	AsyncCall bool
	Result    *AsyncResult
	Reply     interface{}
}

// AsyncResult : async call result
type AsyncResult struct {
	StartTime int64
	Done      chan *AsyncResult
	Reply     interface{}
	Error     error
}

// DeserializableValue : for lazy deserialize
type DeserializableValue struct {
	Serialization Serialization
	Body          []byte
}

// Deserialize : Deserialize
func (d *DeserializableValue) Deserialize(v interface{}) (interface{}, error) {
	return d.Serialization.DeSerialize(d.Body, v)
}

// DeserializeMulti : DeserializeMulti
func (d *DeserializableValue) DeserializeMulti(v []interface{}) ([]interface{}, error) {
	return d.Serialization.DeSerializeMulti(d.Body, v)
}

// MotanRequest : Request default implement
type MotanRequest struct {
	RequestID   uint64
	ServiceName string
	Method      string
	MethodDesc  string
	Arguments   []interface{}
	Attachment  map[string]string
	RPCContext  *RPCContext
}

// GetAttachment GetAttachment
func (m *MotanRequest) GetAttachment(key string) string {
	if m.Attachment == nil {
		return ""
	}
	return m.Attachment[key]
}

// SetAttachment : SetAttachment
func (m *MotanRequest) SetAttachment(key string, value string) {
	if m.Attachment == nil {
		m.Attachment = make(map[string]string)
	}
	m.Attachment[key] = value
}

// GetServiceName GetServiceName
func (m *MotanRequest) GetServiceName() string {
	return m.ServiceName
}

// GetMethod GetMethod
func (m *MotanRequest) GetMethod() string {
	return m.Method
}

// GetMethodDesc GetMethodDesc
func (m *MotanRequest) GetMethodDesc() string {
	return m.MethodDesc
}

func (m *MotanRequest) GetArguments() []interface{} {
	return m.Arguments
}
func (m *MotanRequest) GetRequestID() uint64 {
	return m.RequestID
}

func (m *MotanRequest) SetArguments(arguments []interface{}) {
	m.Arguments = arguments
}

func (m *MotanRequest) GetAttachments() map[string]string {
	return m.Attachment
}

func (m *MotanRequest) GetRPCContext(canCreate bool) *RPCContext {
	if m.RPCContext == nil && canCreate {
		m.RPCContext = &RPCContext{}
	}
	return m.RPCContext
}

// ProcessDeserializable : DeserializableValue to real params according toType
// some serialization can deserialize without toType, so nil toType can be accepted in these serializations
func (m *MotanRequest) ProcessDeserializable(toTypes []interface{}) error {
	if m.GetArguments() != nil && len(m.GetArguments()) == 1 {
		if d, ok := m.GetArguments()[0].(*DeserializableValue); ok {
			v, err := d.DeserializeMulti(toTypes)
			if err != nil {
				return err
			}
			m.SetArguments(v)
		}
	}
	return nil
}

type MotanResponse struct {
	RequestID   uint64
	Value       interface{}
	Exception   *Exception
	ProcessTime int64
	Attachment  map[string]string
	RPCContext  *RPCContext
}

func (m *MotanResponse) GetAttachment(key string) string {
	if m.Attachment == nil {
		return ""
	}
	return m.Attachment[key]
}

func (m *MotanResponse) SetAttachment(key string, value string) {
	if m.Attachment == nil {
		m.Attachment = make(map[string]string)
	}
	m.Attachment[key] = value
}

func (m *MotanResponse) GetValue() interface{} {
	return m.Value
}

func (m *MotanResponse) GetException() *Exception {
	return m.Exception
}

func (m *MotanResponse) GetRequestID() uint64 {
	return m.RequestID
}

func (m *MotanResponse) GetProcessTime() int64 {
	return m.ProcessTime
}

func (m *MotanResponse) GetAttachments() map[string]string {
	return m.Attachment
}

func (m *MotanResponse) GetRPCContext(canCreate bool) *RPCContext {
	if m.RPCContext == nil && canCreate {
		m.RPCContext = &RPCContext{}
	}
	return m.RPCContext
}

func (m *MotanResponse) SetProcessTime(time int64) {
	m.ProcessTime = time
}

// ProcessDeserializable : same with MotanRequest
func (m *MotanResponse) ProcessDeserializable(toType interface{}) error {
	if m.GetValue() != nil {
		if d, ok := m.GetValue().(*DeserializableValue); ok {
			v, err := d.Deserialize(toType)
			if err != nil {
				return err
			}
			m.Value = v
		}
	}
	return nil
}

func BuildExceptionResponse(requestid uint64, e *Exception) *MotanResponse {
	return &MotanResponse{RequestID: requestid, Exception: e}
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

type DefaultExtentionFactory struct {
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

func (d *DefaultExtentionFactory) GetHa(url *URL) HaStrategy {
	haName := url.GetParam(Hakey, "failover")
	if newHa, ok := d.haFactories[haName]; ok {
		return newHa(url)
	}
	vlog.Errorf("HaStrategy name %s is not found in DefaultExtentionFactory!\n", haName)
	return nil
}

func (d *DefaultExtentionFactory) GetLB(url *URL) LoadBalance {
	lbName := url.GetParam(Lbkey, "random")
	if newLb, ok := d.lbFactories[lbName]; ok {
		return newLb(url)
	}
	vlog.Errorf("LoadBalance name %s is not found in DefaultExtentionFactory!\n", lbName)
	return nil
}

func (d *DefaultExtentionFactory) GetFilter(name string) Filter {
	if newDefualt, ok := d.filterFactories[strings.TrimSpace(name)]; ok {
		return newDefualt()
	}
	vlog.Errorf("filter name %s is not found in DefaultExtentionFactory!\n", name)
	return nil
}

func (d *DefaultExtentionFactory) GetRegistry(url *URL) Registry {
	key := url.GetIdentity()
	if registry, exist := d.registries[key]; exist {
		return registry
	}
	d.newRegistryLock.Lock()
	defer d.newRegistryLock.Unlock()
	if registry, exist := d.registries[key]; exist {
		return registry
	} else if newRegistry, ok := d.registryFactories[url.Protocol]; ok {
		registry := newRegistry(url)
		Initialize(registry)
		d.registries[key] = registry
		return registry
	}
	vlog.Errorf("Registry name %s is not found in DefaultExtentionFactory!\n", url.Protocol)
	return nil
}

func (d *DefaultExtentionFactory) GetEndPoint(url *URL) EndPoint {
	if newEp, ok := d.endpointFactories[url.Protocol]; ok {
		endpoint := newEp(url)
		return endpoint
	}
	vlog.Errorf("EndPoint(protocol) name %s is not found in DefaultExtentionFactory!\n", url.Protocol)
	return nil
}

func (d *DefaultExtentionFactory) GetProvider(url *URL) Provider {
	if newProviderFunc, ok := d.providerFactories[url.GetParam(ProviderKey, "default")]; ok {
		provider := newProviderFunc(url)
		return provider
	}
	vlog.Errorf("provider(protocol) name %s is not found in DefaultExtentionFactory!\n", url.Protocol)
	return nil
}

func (d *DefaultExtentionFactory) GetServer(url *URL) Server {
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
	vlog.Errorf("server name %s is not found in DefaultExtentionFactory!\n", sname)
	return nil
}

func (d *DefaultExtentionFactory) GetMessageHandler(name string) MessageHandler {
	if newMessageHandler, ok := d.messageHandlers[strings.TrimSpace(name)]; ok {
		handler := newMessageHandler()
		Initialize(handler)
		return handler
	}
	vlog.Errorf("messageHandler name %s is not found in DefaultExtentionFactory!\n", name)
	return nil
}

func (d *DefaultExtentionFactory) GetSerialization(name string, id int) Serialization {
	if name != "" {
		if newSerialization, ok := d.serializations[strings.TrimSpace(name)]; ok {
			return newSerialization()
		}
	} else if id > -1 {
		if newSerialization, ok := d.serializations[strconv.Itoa(id)]; ok {
			return newSerialization()
		}
	}

	vlog.Errorf("messageHandler name %s is not found in DefaultExtentionFactory!\n", name)
	return nil
}

func (d *DefaultExtentionFactory) RegistExtFilter(name string, newFilter DefaultFilterFunc) {
	// 覆盖方式
	d.filterFactories[name] = newFilter
}

func (d *DefaultExtentionFactory) RegistExtHa(name string, newHa NewHaFunc) {
	d.haFactories[name] = newHa
}

func (d *DefaultExtentionFactory) RegistExtLb(name string, newLb NewLbFunc) {
	d.lbFactories[name] = newLb
}

func (d *DefaultExtentionFactory) RegistExtEndpoint(name string, newEndpoint NewEndpointFunc) {
	d.endpointFactories[name] = newEndpoint
}

func (d *DefaultExtentionFactory) RegistExtProvider(name string, newProvider NewProviderFunc) {
	d.providerFactories[name] = newProvider
}

func (d *DefaultExtentionFactory) RegistExtRegistry(name string, newRegistry NewRegistryFunc) {
	d.registryFactories[name] = newRegistry
}

func (d *DefaultExtentionFactory) RegistExtServer(name string, newServer NewServerFunc) {
	d.servers[name] = newServer
}

func (d *DefaultExtentionFactory) RegistryExtMessageHandler(name string, newMessage NewMessageHandlerFunc) {
	d.messageHandlers[name] = newMessage
}

func (d *DefaultExtentionFactory) RegistryExtSerialization(name string, id int, newSerialization NewSerializationFunc) {
	d.serializations[name] = newSerialization
	d.serializations[strconv.Itoa(id)] = newSerialization
}

func (d *DefaultExtentionFactory) Initialize() {
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
	lef *lastEndPointFilter
	lcf *lastClusterFilter
)

func GetLastEndPointFilter() EndPointFilter {
	if lef == nil {
		lef = new(lastEndPointFilter)
	}
	return lef
}

func GetLastClusterFilter() ClusterFilter {
	if lcf == nil {
		lcf = new(lastClusterFilter)
	}
	return lcf
}

type lastEndPointFilter struct{}

func (l *lastEndPointFilter) GetName() string {
	return "lastEndPointFilter"
}

func (l *lastEndPointFilter) NewFilter(url *URL) Filter {
	return GetLastEndPointFilter()
}

func (l *lastEndPointFilter) Filter(caller Caller, request Request) Response {
	return caller.Call(request)
}

func (l *lastEndPointFilter) HasNext() bool {
	return false
}

func (l *lastEndPointFilter) SetNext(nextFilter EndPointFilter) {
	vlog.Errorf("should not set next in lastEndPointFilter! filer:%s\n", nextFilter.GetName())
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

func (l *lastClusterFilter) GetName() string {
	return "lastClusterFilter"
}
func (l *lastClusterFilter) NewFilter(url *URL) Filter {
	return GetLastClusterFilter()
}

func (l *lastClusterFilter) Filter(haStrategy HaStrategy, loadBalance LoadBalance, request Request) Response {
	return haStrategy.Call(request, loadBalance)
}

func (l *lastClusterFilter) HasNext() bool {
	return false
}
func (l *lastClusterFilter) SetNext(nextFilter ClusterFilter) {
	vlog.Errorf("should not set next in lastClusterFilter! filer:%s\n", nextFilter.GetName())
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

func (f *FilterEndPoint) Call(request Request) Response {
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
