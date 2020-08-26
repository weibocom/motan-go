package core

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/weibocom/motan-go/log"
)

type taskHandler func()

var refreshTaskPool = make(chan taskHandler, 100)

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

	ProtocolLocal = "local"
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
	Open(block bool, proxy bool, handler MessageHandler, extFactory ExtensionFactory) error
}

// Exporter : export and manage a service. one exporter bind with a service
type Exporter interface {
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

	// trace context
	Tc *TraceContext
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

// GetAttachment GetAttachment
func (m *MotanRequest) GetAttachment(key string) string {
	if m.Attachment == nil {
		return ""
	}
	return m.Attachment.LoadOrEmpty(key)
}

// SetAttachment : SetAttachment
func (m *MotanRequest) SetAttachment(key string, value string) {
	m.GetAttachments().Store(key, value)
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

func (m *MotanRequest) GetAttachments() *StringMap {
	attachment := (*StringMap)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&m.Attachment))))
	if attachment != nil {
		return attachment
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Attachment == nil {
		attachment = NewStringMap(DefaultAttachmentSize)
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&m.Attachment)), unsafe.Pointer(attachment))
	} else {
		attachment = m.Attachment
	}
	return attachment
}

func (m *MotanRequest) GetRPCContext(canCreate bool) *RPCContext {
	if m.RPCContext == nil && canCreate {
		m.RPCContext = &RPCContext{}
	}
	return m.RPCContext
}

func (m *MotanRequest) Clone() interface{} {
	newRequest := &MotanRequest{
		RequestID:   m.RequestID,
		ServiceName: m.ServiceName,
		Method:      m.Method,
		MethodDesc:  m.MethodDesc,
		Arguments:   m.Arguments,
	}
	if m.Attachment != nil {
		newRequest.Attachment = m.Attachment.Copy()
	}
	if m.RPCContext != nil {
		newRequest.RPCContext = &RPCContext{
			ExtFactory:   m.RPCContext.ExtFactory,
			Oneway:       m.RPCContext.Oneway,
			Proxy:        m.RPCContext.Proxy,
			GzipSize:     m.RPCContext.GzipSize,
			SerializeNum: m.RPCContext.SerializeNum,
			Serialized:   m.RPCContext.Serialized,
			AsyncCall:    m.RPCContext.AsyncCall,
			Result:       m.RPCContext.Result,
			Reply:        m.RPCContext.Reply,
			Tc:           m.RPCContext.Tc,
		}
		if m.RPCContext.OriginalMessage != nil {
			if oldMessage, ok := m.RPCContext.OriginalMessage.(Cloneable); ok {
				newRequest.RPCContext.OriginalMessage = oldMessage.Clone()
			} else {
				newRequest.RPCContext.OriginalMessage = oldMessage
			}
		}
	}
	return newRequest
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
	Attachment  *StringMap
	RPCContext  *RPCContext
	mu          sync.Mutex
}

func (m *MotanResponse) GetAttachment(key string) string {
	if m.Attachment == nil {
		return ""
	}
	return m.Attachment.LoadOrEmpty(key)
}

func (m *MotanResponse) SetAttachment(key string, value string) {
	m.GetAttachments().Store(key, value)
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

func (m *MotanResponse) GetAttachments() *StringMap {
	attachment := (*StringMap)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&m.Attachment))))
	if attachment != nil {
		return attachment
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Attachment == nil {
		attachment = NewStringMap(DefaultAttachmentSize)
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&m.Attachment)), unsafe.Pointer(attachment))
	} else {
		attachment = m.Attachment
	}
	return attachment
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
	if newDefualt, ok := d.filterFactories[strings.TrimSpace(name)]; ok {
		return newDefualt()
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
	return caller.Call(request)
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
		case refreshTaskPool <- taskHandler(func() { c.refreshServices() }):
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
