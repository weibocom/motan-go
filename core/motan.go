package core

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/weibocom/motan-go/log"
)

//--------------const--------------
const (
	FrameworkException = iota
	ServiceException
	BizException
)

const (
	EndPointFilterType = iota
	ClusterFilterType
)

//-----------interface-------------
type Name interface {
	GetName() string
}

type Identity interface {
	GetIdentity() string
}

type Withurl interface {
	GetUrl() *Url
	SetUrl(url *Url)
}

type Attachment interface {
	GetAttachments() map[string]string
	GetAttachment(key string) string
	SetAttachment(key string, value string)
}

type Destroyable interface {
	Destroy()
}

type Caller interface {
	Withurl
	Status
	Call(request Request) Response
	Destroyable
}

type Request interface {
	Attachment
	GetServiceName() string // service name  e.g. request path.or interface name
	GetMethod() string
	GetMethodDesc() string
	GetArguments() []interface{}
	GetRequestId() uint64
	GetRpcContext(canCreate bool) *RpcContext
	ProcessDeserializable(toTypes []interface{}) error
}

type Response interface {
	Attachment
	GetValue() interface{}
	GetException() *Exception
	GetRequestId() uint64
	GetProcessTime() int64
	SetProcessTime(time int64)
	GetRpcContext(canCreate bool) *RpcContext
	ProcessDeserializable(toType interface{}) error
}

type Status interface {
	IsAvailable() bool
}

type EndPoint interface {
	Name
	Caller
	SetSerialization(s Serialization)
	SetProxy(proxy bool)
}

type HaStrategy interface {
	Name
	Withurl
	Call(request Request, loadBalance LoadBalance) Response
}

type LoadBalance interface {
	OnRefresh(endpoints []EndPoint)

	Select(request Request) EndPoint

	SelectArray(request Request) []EndPoint

	SetWeight(weight string)
}

type DiscoverService interface {
	Subscribe(url *Url, listener NotifyListener)

	Unsubscribe(url *Url, listener NotifyListener)

	Discover(url *Url) []*Url
}

type DiscoverCommand interface {
	SubscribeCommand(url *Url, listener CommandNotifyListener)
	UnSubscribeCommand(url *Url, listener CommandNotifyListener)
	DiscoverCommand(url *Url) string
}

type RegisterService interface {
	Register(serverUrl *Url)
	UnRegister(serverUrl *Url)
	Available(serverUrl *Url)
	Unavailable(serverUrl *Url)
	GetRegisteredServices() []*Url
}

type SnapshotService interface {
	StartSnapshot(conf *SnapshotConf)
}

type Registry interface {
	Name
	Withurl
	DiscoverService
	RegisterService
	SnapshotService
}

type NotifyListener interface {
	Identity
	Notify(registryUrl *Url, urls []*Url)
}

type CommandNotifyListener interface {
	Identity
	NotifyCommand(registryUrl *Url, commandType int, commandInfo string)
}

type Filter interface {
	Name
	// filter must be prototype
	NewFilter(url *Url) Filter
	HasNext() bool
	GetIndex() int
	GetType() int32
}

type EndPointFilter interface {
	Filter
	SetNext(nextFilter EndPointFilter)
	GetNext() EndPointFilter
	//Filter for endpoint
	Filter(caller Caller, request Request) Response
}

type ClusterFilter interface {
	Filter
	SetNext(nextFilter ClusterFilter)
	GetNext() ClusterFilter
	//Filter for Cluster
	Filter(haStrategy HaStrategy, loadBalance LoadBalance, request Request) Response
}

type Server interface {
	Withurl
	Name
	Destroyable
	SetMessageHandler(mh MessageHandler)
	GetMessageHandler() MessageHandler
	Open(block bool, proxy bool, handler MessageHandler, extFactory ExtentionFactory) error
}

type Exporter interface {
	Export(server Server) error
	Unexport() error
	SetProvider(provider Provider)
	GetProvider() Provider
	Withurl
}

type Provider interface {
	SetService(s interface{})
	Caller
	GetPath() string
}

type MessageHandler interface {
	Call(request Request) (res Response)
	AddProvider(p Provider) error
	RmProvider(p Provider)
	GetProvider(serviceName string) Provider
}

type Serialization interface {
	GetSerialNum() int
	Serialize(v interface{}) ([]byte, error)
	DeSerialize(b []byte, v interface{}) (interface{}, error)
	SerializeMulti(v []interface{}) ([]byte, error)
	DeSerializeMulti(b []byte, v []interface{}) ([]interface{}, error)
}

type ExtentionFactory interface {
	GetHa(url *Url) HaStrategy
	GetLB(url *Url) LoadBalance
	GetFilter(name string) Filter
	GetRegistry(url *Url) Registry
	GetEndPoint(url *Url) EndPoint
	GetProvider(url *Url) Provider
	GetServer(url *Url) Server
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

type Initializable interface {
	Initialize()
}

type SetContext interface {
	SetContext(context *Context)
}

func Initialize(s interface{}) {
	if init, ok := s.(Initializable); ok {
		init.Initialize()
	}
}

func CanSetContext(s interface{}, context *Context) {
	if sc, ok := s.(SetContext); ok {
		sc.SetContext(context)
	}
}

//-------------models--------------
type SnapshotConf struct {
	// the interval of creating snapshot
	SnapshotInterval time.Duration
	SnapshotDir      string
}

type Exception struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
	ErrType int    `json:"errtype"`
}

type RpcContext struct {
	ExtFactory      ExtentionFactory
	OriginalMessage interface{}
	Oneway          bool
	Proxy           bool
	GzipSize        int

	//for call
	AsyncCall bool
	Result    *AsyncResult
	Reply     interface{}
}

type AsyncResult struct {
	StartTime int64
	Done      chan *AsyncResult
	Reply     interface{}
	Error     error
}

type DeserializableValue struct {
	Serialization Serialization
	Body          []byte
}

func (d *DeserializableValue) Deserialize(v interface{}) (interface{}, error) {
	return d.Serialization.DeSerialize(d.Body, v)
}

func (d *DeserializableValue) DeserializeMulti(v []interface{}) ([]interface{}, error) {
	return d.Serialization.DeSerializeMulti(d.Body, v)
}

type MotanRequest struct {
	RequestId   uint64
	ServiceName string
	Method      string
	MethodDesc  string
	Arguments   []interface{}
	Attachment  map[string]string
	RpcContext  *RpcContext
}

func (m *MotanRequest) GetAttachment(key string) string {
	if m.Attachment == nil {
		return ""
	}
	return m.Attachment[key]
}
func (m *MotanRequest) SetAttachment(key string, value string) {
	if m.Attachment == nil {
		m.Attachment = make(map[string]string)
	}
	m.Attachment[key] = value
}
func (m *MotanRequest) GetServiceName() string {
	return m.ServiceName
}
func (m *MotanRequest) GetMethod() string {
	return m.Method
}
func (m *MotanRequest) GetMethodDesc() string {
	return m.MethodDesc
}
func (m *MotanRequest) GetArguments() []interface{} {
	return m.Arguments
}
func (m *MotanRequest) GetRequestId() uint64 {
	return m.RequestId
}

func (m *MotanRequest) SetArguments(arguments []interface{}) {
	m.Arguments = arguments
}

func (m *MotanRequest) GetAttachments() map[string]string {
	return m.Attachment
}

func (m *MotanRequest) GetRpcContext(canCreate bool) *RpcContext {
	if m.RpcContext == nil && canCreate {
		m.RpcContext = &RpcContext{}
	}
	return m.RpcContext
}

// process DeserializableValue to real params according toType
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
	RequestId   uint64
	Value       interface{}
	Exception   *Exception
	ProcessTime int64
	Attachment  map[string]string
	RpcContext  *RpcContext
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

func (m *MotanResponse) GetRequestId() uint64 {
	return m.RequestId
}

func (m *MotanResponse) GetProcessTime() int64 {
	return m.ProcessTime
}

func (m *MotanResponse) GetAttachments() map[string]string {
	return m.Attachment
}

func (m *MotanResponse) GetRpcContext(canCreate bool) *RpcContext {
	if m.RpcContext == nil && canCreate {
		m.RpcContext = &RpcContext{}
	}
	return m.RpcContext
}

func (m *MotanResponse) SetProcessTime(time int64) {
	m.ProcessTime = time
}

// process DeserializableValue to real value according toType
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
	return &MotanResponse{RequestId: requestid, Exception: e}
}

// extensions factory-func
type DefaultFilterFunc func() Filter
type NewHaFunc func(url *Url) HaStrategy
type NewLbFunc func(url *Url) LoadBalance
type NewEndpointFunc func(url *Url) EndPoint
type NewProviderFunc func(url *Url) Provider
type NewRegistryFunc func(url *Url) Registry
type NewServerFunc func(url *Url) Server
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

func (d *DefaultExtentionFactory) GetHa(url *Url) HaStrategy {
	haName := url.GetParam(Hakey, "failover")
	if newHa, ok := d.haFactories[haName]; ok {
		return newHa(url)
	}
	vlog.Errorf("HaStrategy name %s is not found in DefaultExtentionFactory!\n", haName)
	return nil
}

func (d *DefaultExtentionFactory) GetLB(url *Url) LoadBalance {
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

func (d *DefaultExtentionFactory) GetRegistry(url *Url) Registry {
	key := url.GetIdentity()
	if registry, exist := d.registries[key]; exist {
		return registry
	} else {
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

}

func (d *DefaultExtentionFactory) GetEndPoint(url *Url) EndPoint {
	if newEp, ok := d.endpointFactories[url.Protocol]; ok {
		endpoint := newEp(url)
		return endpoint
	}
	vlog.Errorf("EndPoint(protocol) name %s is not found in DefaultExtentionFactory!\n", url.Protocol)
	return nil
}

func (d *DefaultExtentionFactory) GetProvider(url *Url) Provider {
	if newProviderFunc, ok := d.providerFactories[url.GetParam(ProviderKey, "default")]; ok {
		provider := newProviderFunc(url)
		return provider
	}
	vlog.Errorf("provider(protocol) name %s is not found in DefaultExtentionFactory!\n", url.Protocol)
	return nil
}

func (d *DefaultExtentionFactory) GetServer(url *Url) Server {
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

func GetLastEndPointFilter() *lastEndPointFilter {
	if lef == nil {
		lef = new(lastEndPointFilter)
	}
	return lef
}

func GetLastClusterFilter() *lastClusterFilter {
	if lcf == nil {
		lcf = new(lastClusterFilter)
	}
	return lcf
}

type lastEndPointFilter struct{}

func (l *lastEndPointFilter) GetName() string {
	return "lastEndPointFilter"
}

func (l *lastEndPointFilter) NewFilter(url *Url) Filter {
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
func (l *lastClusterFilter) NewFilter(url *Url) Filter {
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
	Url           *Url
	Filter        EndPointFilter
	StatusFilters []Status
	Caller        Caller
}

func (f *FilterEndPoint) Call(request Request) Response {
	return f.Filter.Filter(f.Caller, request)
}
func (f *FilterEndPoint) GetUrl() *Url {
	return f.Url
}
func (f *FilterEndPoint) SetUrl(url *Url) {
	f.Url = url
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
