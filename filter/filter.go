package filter

import (
	motan "github.com/weibocom/motan-go/core"
)

// ext name
const (
	AccessLog      = "accessLog"
	Metrics        = "metrics"
	CircuitBreaker = "circuitbreaker"
	FailFast       = "failfast"
	ClusterMetrics = "clusterMetrics"
	Trace          = "trace"
)

func RegistDefaultFilters(extFactory motan.ExtensionFactory) {
	extFactory.RegistExtFilter(AccessLog, func() motan.Filter {
		return &AccessLogEndPointFilter{}
	})

	extFactory.RegistExtFilter(Metrics, func() motan.Filter {
		return &MetricsFilter{}
	})

	extFactory.RegistExtFilter(CircuitBreaker, func() motan.Filter {
		return &CircuitBreakerEndPointFilter{}
	})

	extFactory.RegistExtFilter(FailFast, func() motan.Filter {
		return &FailfastFilter{}
	})

	extFactory.RegistExtFilter(ClusterMetrics, func() motan.Filter {
		return &ClusterMetricsFilter{}
	})

	extFactory.RegistExtFilter(Trace, func() motan.Filter {
		return &TracingFilter{}
	})
}
