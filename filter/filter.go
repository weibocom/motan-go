package filter

import (
	motan "github.com/weibocom/motan-go/core"
	"time"
)

// ext name
const (
	// endpoint filter
	AccessLog      = "accessLog"
	Metrics        = "metrics"
	CircuitBreaker = "circuitBreaker"
	FailFast       = "failfast"
	Trace          = "trace"
	RateLimit      = "rateLimit"

	// cluster filter
	ClusterAccessLog      = "clusterAccessLog"
	ClusterMetrics        = "clusterMetrics"
	ClusterCircuitBreaker = "clusterCircuitBreaker"
)

func RegistDefaultFilters(extFactory motan.ExtensionFactory) {
	// endpoint filter
	extFactory.RegistExtFilter(AccessLog, func() motan.Filter {
		return &AccessLogFilter{}
	})

	extFactory.RegistExtFilter(Metrics, func() motan.Filter {
		return &MetricsFilter{}
	})

	extFactory.RegistExtFilter(CircuitBreaker, func() motan.Filter {
		return &CircuitBreakerFilter{}
	})

	extFactory.RegistExtFilter(FailFast, func() motan.Filter {
		return &FailfastFilter{}
	})

	extFactory.RegistExtFilter(Trace, func() motan.Filter {
		return &TracingFilter{}
	})

	extFactory.RegistExtFilter(RateLimit, func() motan.Filter {
		return &RateLimitFilter{}
	})

	// cluster filter
	extFactory.RegistExtFilter(ClusterAccessLog, func() motan.Filter {
		return &ClusterAccessLogFilter{}
	})

	extFactory.RegistExtFilter(ClusterMetrics, func() motan.Filter {
		return &ClusterMetricsFilter{}
	})

	extFactory.RegistExtFilter(ClusterCircuitBreaker, func() motan.Filter {
		return &ClusterCircuitBreakerFilter{}
	})
}

func getFilterStartTime(caller motan.Caller, request motan.Request) time.Time {
	switch caller.(type) {
	case motan.Provider:
		return request.GetRPCContext(true).RequestReceiveTime
	case motan.EndPoint:
		return time.Now()
	default:
		return time.Now()
	}
}

func GetFilterRuntimeInfo(filter motan.Filter) map[string]interface{} {
	return map[string]interface{}{
		motan.RuntimeNameKey:  filter.GetName(),
		motan.RuntimeIndexKey: filter.GetIndex(),
		motan.RuntimeTypeKey:  filter.GetType(),
	}
}
