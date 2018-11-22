package filter

import (
	"errors"
	"github.com/afex/hystrix-go/hystrix"
	motan "github.com/weibocom/motan-go/core"
)

type ClusterCircuitBreakerFilter struct {
	url            *motan.URL
	next           motan.ClusterFilter
	available      bool
	circuitBreaker *hystrix.CircuitBreaker
}

func (c *ClusterCircuitBreakerFilter) GetIndex() int {
	return 20
}

func (c *ClusterCircuitBreakerFilter) NewFilter(url *motan.URL) motan.Filter {
	available, circuitBreaker := newCircuitBreaker(url)
	return &ClusterCircuitBreakerFilter{url: url, available: available, circuitBreaker: circuitBreaker}
}

func (c *ClusterCircuitBreakerFilter) Filter(ha motan.HaStrategy, lb motan.LoadBalance, request motan.Request) motan.Response {
	var response motan.Response
	if c.available {
		_ = hystrix.Do(c.url.GetIdentity(), func() error {
			response = c.GetNext().Filter(ha, lb, request)
			if ex := response.GetException(); ex != nil {
				return errors.New(ex.ErrMsg)
			}
			return nil
		}, func(err error) error {
			if response == nil {
				response = defaultErrMotanResponse(request, err.Error())
			}
			return err
		})
	} else {
		response = c.GetNext().Filter(ha, lb, request)
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
