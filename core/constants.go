package core

import "time"

// --------------all global public constants--------------
// exception type
const (
	FrameworkException = iota
	// ServiceException : exception by service call
	ServiceException
	// BizException : exception by service implements
	BizException
)

// filter type
const (
	// EndPointFilterType filter for endpoint
	EndPointFilterType = iota
	// ClusterFilterType filter for cluster
	ClusterFilterType
)

// common url parameter key
const (
	NodeTypeKey             = "nodeType"
	Hakey                   = "haStrategy"
	Lbkey                   = "loadbalance"
	TimeOutKey              = "requestTimeout"
	MinTimeOutKey           = "minRequestTimeout"
	MaxTimeOutKey           = "maxRequestTimeout"
	SessionTimeOutKey       = "registrySessionTimeout"
	RetriesKey              = "retries"
	ApplicationKey          = "application"
	VersionKey              = "version"
	FilterKey               = "filter"
	GlobalFilter            = "globalFilter"
	DisableGlobalFilter     = "disableGlobalFilter"
	DefaultFilter           = "defaultFilter"
	DisableDefaultFilter    = "disableDefaultFilter"
	MotanEpAsyncInit        = "motanEpAsyncInit"
	RegistryKey             = "registry"
	WeightKey               = "weight"
	SerializationKey        = "serialization"
	RefKey                  = "ref"
	ExportKey               = "export"
	ModuleKey               = "module"
	GroupKey                = "group"
	ProviderKey             = "provider"
	ProxyKey                = "proxy"
	AddressKey              = "address"
	GzipSizeKey             = "mingzSize"
	HostKey                 = "host"
	RemoteIPKey             = "remoteIP"
	ProxyRegistryKey        = "proxyRegistry"
	ProxyRegistryUrlString  = "proxyRegistryUrlString"
	InitClusterTimeoutKey   = "initClusterTimeout"
	ConnectTimeoutKey       = "connectTimeout"
	ConnectRetryIntervalKey = "connectRetryInterval"
	ClientConnectionKey     = "clientConnection"
	LazyInit                = "lazyInit"
	AsyncInitConnection     = "asyncInitConnection"
	ErrorCountThresholdKey  = "errorCountThreshold"
	KeepaliveIntervalKey    = "keepaliveInterval"
	UnixSockKey             = "unixSock"
	ManagementUnixSockKey   = "managementUnixSock"
	ManagementPortRangeKey  = "managementPortRange"
	HTTPProxyUnixSockKey    = "httpProxyUnixSock"
	MixGroups               = "mixGroups"
	MaxContentLength        = "maxContentLength"
	UnixSockProtocolFlag    = "unix://"
)

// attachment keys
const (
	XForwardedForLower = "x-forwarded-for" // used as motan default proxy key
	XForwardedFor      = "X-Forwarded-For"
	ConsistentHashKey  = "consistentHashKey" //string used to calculate consistent hash
)

// registryStatus
const (
	RegisterSuccess   = "register-success"
	RegisterFailed    = "register-failed"
	UnregisterSuccess = "unregister-success"
	UnregisterFailed  = "unregister-failed"
	NotRegister       = "not-register"
)

// nodeType
const (
	NodeTypeService = "service"
	NodeTypeReferer = "referer"
	NodeTypeAgent   = "agent"
)

// trace span name
const (
	Receive       = "receive"
	Decode        = "decode"
	Convert       = "convert"
	ClFilter      = "clusterFilter"
	EpFilterStart = "selectEndpoint"
	EpFilterEnd   = "endpointFilter"
	Encode        = "encode"
	Send          = "send"
)

const (
	DefaultWriteTimeout     = 5 * time.Second
	DefaultMaxContentLength = 10 * 1024 * 1024
	GroupNameSeparator      = ","
)

// env variables
const (
	GroupEnvironmentName     = "MESH_SERVICE_ADDITIONAL_GROUP"
	DirectRPCEnvironmentName = "MESH_DIRECT_RPC"
	FilterEnvironmentName    = "MESH_FILTERS"
)

// meta keys
const (
	MetaUpstreamCode = "upstreamCode"
)

// errorCodes
const (
	ENoEndpoints = 1001
	ENoChannel   = 1002
	EUnkonwnMsg  = 1003
	EConvertMsg  = 1004
)

const (
	DefaultReferVersion = "1.0"
)
