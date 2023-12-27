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

// metrics request application
const (
	MetricsReqApplication = "metricsReqApp"
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
	HandlerEnvironmentName   = "MESH_ADMIN_EXT_HANDLERS"
	RegGroupSuffix           = "RPC_REG_GROUP_SUFFIX"
	SubGroupSuffix           = "MESH_MULTI_SUB_GROUP_SUFFIX"
)

// meta keys
const (
	MetaUpstreamCode = "upstreamCode"
)

// errorCodes
const (
	ENoEndpoints      = 1001
	ENoChannel        = 1002
	EUnkonwnMsg       = 1003
	EConvertMsg       = 1004
	EProviderNotExist = 404
)

// ProviderNotExistPrefix errorMessage
const (
	ProviderNotExistPrefix = "provider not exist serviceKey="
)

const (
	DefaultReferVersion = "1.0"
)

//----------- runtime -------------

const (
	// -----------top level keys-------------

	RuntimeInstanceTypeKey     = "instanceType"
	RuntimeExportersKey        = "exporters"
	RuntimeClustersKey         = "clusters"
	RuntimeHttpClustersKey     = "httpClusters"
	RuntimeExtensionFactoryKey = "extensionFactory"
	RuntimeServersKey          = "servers"
	RuntimeBasicKey            = "basic"

	//-----------common keys-------------

	RuntimeUrlKey            = "url"
	RuntimeIsAvailableKey    = "isAvailable"
	RuntimeProxyKey          = "proxy"
	RuntimeAvailableKey      = "available"
	RuntimeEndpointKey       = "endpoint"
	RuntimeFiltersKey        = "filters"
	RuntimeClusterFiltersKey = "clusterFilters"
	RuntimeNameKey           = "name"
	RuntimeIndexKey          = "index"
	RuntimeTypeKey           = "type"

	// -----------exporter keys-------------

	RuntimeProviderKey = "provider"

	// -----------server keys-------------

	RuntimeAgentServerKey        = "agentServer"
	RuntimeAgentPortServerKey    = "agentPortServer"
	RuntimeMaxContentLengthKey   = "maxContentLength"
	RuntimeHeartbeatEnabledKey   = "heartbeatEnabled"
	RuntimeMessageHandlerKey     = "messageHandler"
	RuntimeProvidersKey          = "providers"
	RuntimeMessageHandlerTypeKey = "messageHandlerType"

	// -----------http proxy server-------------

	RuntimeHttpProxyServerKey = "httpProxyServer"
	RuntimeDenyKey            = "deny"
	RuntimeKeepaliveKey       = "keepalive"
	RuntimeDefaultDomainKey   = "defaultDomain"

	// -----------cluster keys-------------

	RuntimeReferersKey    = "referers"
	RuntimeRefererSizeKey = "refererSize"
	RuntimeUnavailableKey = "unavailable"

	// -----------endpoint keys-------------

	RuntimeErrorCountKey       = "errorCount"
	RuntimeKeepaliveRunningKey = "keepaliveRunning"
	RuntimeKeepaliveTypeKey    = "keepaliveType"

	// -----------extensionFactory keys-------------

	RuntimeRegistriesKey = "registries"

	// -----------registry keys-------------

	RuntimeRegisteredServiceUrlsKey = "registeredServiceUrls"
	RuntimeSubscribedServiceUrlsKey = "subscribedServiceUrls"
	RuntimeFailedRegisterUrls       = "failedRegisterUrls"
	RuntimeFailedUnregisterUrls     = "failedUnregisterUrls"
	RuntimeFailedSubscribeUrls      = "failedSubscribeUrls"
	RuntimeFailedUnsubScribeUrls    = "failedUnsubscribeUrls"
	RuntimeSubscribeInfoKey         = "subscribeInfo"
	RuntimeAgentCommandKey          = "agentCommand"
	RuntimeServiceCommandKey        = "serviceCommand"
	RuntimeStaticCommandKey         = "staticCommand"
	RuntimeWeightKey                = "weight"
	RuntimeCommandHistoryKey        = "commandHistory"
	RuntimeNotifyHistoryKey         = "notifyHistory"

	// -----------basic keys-------------

	RuntimeCpuPercentKey = "cpuPercent"
	RuntimeRssMemoryKey  = "rssMemory"
)
