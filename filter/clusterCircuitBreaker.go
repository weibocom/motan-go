package filter

import (
	"github.com/afex/hystrix-go/hystrix"
	motan "github.com/weibocom/motan-go/core"
)

type ClusterCircuitBreakerFilter struct {
	url                 *motan.URL
	next                motan.ClusterFilter
	includeBizException bool
}

func (c *ClusterCircuitBreakerFilter) GetIndex() int {
	return 20
}

func (c *ClusterCircuitBreakerFilter) NewFilter(url *motan.URL) motan.Filter {
	bizException := newCircuitBreaker(c.GetName(), url)
	return &ClusterCircuitBreakerFilter{url: url, includeBizException: bizException}
}

func (c *ClusterCircuitBreakerFilter) Filter(ha motan.HaStrategy, lb motan.LoadBalance, request motan.Request) motan.Response {
	var response motan.Response
	err := hystrix.Do(c.url.GetIdentity(), func() error {
		response = c.GetNext().Filter(ha, lb, request)
		return checkException(response, c.includeBizException)
	}, nil)
	if err != nil {
		return defaultErrMotanResponse(request, err.Error())
	}
	return response
}

func (c *ClusterCircuitBreakerFilter) GetName() string {
	return ClusterCircuitBreaker
}

func (c *ClusterCircuitBreakerFilter) HasNext() bool {
	return c.next != nil
}

func (c *ClusterCircuitBreakerFilter) GetType() int32 {
	return motan.ClusterFilterType
}

func (c *ClusterCircuitBreakerFilter) SetNext(cf motan.ClusterFilter) {
	c.next = cf
	return
}

func (c *ClusterCircuitBreakerFilter) GetNext() motan.ClusterFilter {
	return c.next
}
