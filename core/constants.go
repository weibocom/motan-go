package core

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
	NodeTypeKey       = "nodeType"
	Hakey             = "haStrategy"
	Lbkey             = "loadbalance"
	TimeOutKey        = "requestTimeout"
	SessionTimeOutKey = "registrySessionTimeout"
	ApplicationKey    = "application"
	VersionKey        = "version"
	FilterKey         = "filter"
	RegistryKey       = "registry"
	WeightKey         = "weight"
	SerializationKey  = "serialization"
	RefKey            = "ref"
	ExportKey         = "export"
	ModuleKey         = "module"
	GroupKey          = "group"
	ProviderKey       = "provider"
	AddressKey        = "address"
	GzipSizeKey       = "mingzSize"
)

// nodeType
const (
	NodeTypeService = "service"
	NodeTypeReferer = "referer"
	NodeTypeAgent   = "agent"
)
