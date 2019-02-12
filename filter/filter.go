package filter

import (
	motan "github.com/weibocom/motan-go/core"
)

// ext name
const (
	AccessLog             = "accessLog"
	ClusterAccessLog      = "clusterAccessLog"
	Metrics               = "metrics"
	ClusterMetrics        = "clusterMetrics"
	CircuitBreaker        = "circuitBreaker"
	ClusterCircuitBreaker = "clusterCircuitBreaker"
	FailFast              = "failfast"
	Trace                 = "trace"
	RateLimit             = "rateLimit"
)

func RegistDefaultFilters(extFactory motan.ExtensionFactory) {
	extFactory.RegistExtFilter(AccessLog, func() motan.Filter {
		return &AccessLogFilter{}
	})

	extFactory.RegistExtFilter(ClusterAccessLog, func() motan.Filter {
		return &ClusterAccessLogFilter{}
	})

	extFactory.RegistExtFilter(Metrics, func() motan.Filter {
		return &MetricsFilter{}
	})

	extFactory.RegistExtFilter(ClusterMetrics, func() motan.Filter {
		return &ClusterMetricsFilter{}
	})

	extFactory.RegistExtFilter(CircuitBreaker, func() motan.Filter {
		return &CircuitBreakerFilter{}
	})

	extFactory.RegistExtFilter(ClusterCircuitBreaker, func() motan.Filter {
		return &ClusterCircuitBreakerFilter{}
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
}
