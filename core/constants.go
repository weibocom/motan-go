package core

import "time"

//--------------all global public constants--------------
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
	SessionTimeOutKey       = "registrySessionTimeout"
	RetriesKey              = "retries"
	ApplicationKey          = "application"
	VersionKey              = "version"
	FilterKey               = "filter"
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
	ConnectTimeoutKey       = "connectTimeout"
	ConnectRetryIntervalKey = "connectRetryInterval"
	ClientConnectionKey     = "clientConnection"
	ErrorCountThresholdKey  = "errorCountThreshold"
	KeepaliveIntervalKey    = "keepaliveInterval"
	UnixSockKey             = "unixSock"
	ManagementUnixSockKey   = "managementUnixSock"
	ManagementPortRangeKey  = "managementPortRange"
	HTTPProxyUnixSockKey    = "httpProxyUnixSock"
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
	DefaultWriteTimeout = 5 * time.Second
)

// errorCodes
const (
	ENoEndpoints = 1001
	ENoChannel   = 1002
)
