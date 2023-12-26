package filter

import (
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/protocol"
)

type ClusterMetricsFilter struct {
	next motan.ClusterFilter
}

func (c *ClusterMetricsFilter) GetIndex() int {
	return 5
}

func (c *ClusterMetricsFilter) NewFilter(url *motan.URL) motan.Filter {
	return &ClusterMetricsFilter{}
}

func (c *ClusterMetricsFilter) GetName() string {
	return ClusterMetrics
}

func (c *ClusterMetricsFilter) HasNext() bool {
	if c.next != nil {
		return true
	}
	return false
}

func (c *ClusterMetricsFilter) GetType() int32 {
	return motan.ClusterFilterType
}

func (c *ClusterMetricsFilter) SetNext(cf motan.ClusterFilter) {
	c.next = cf
	return
}

func (c *ClusterMetricsFilter) GetNext() motan.ClusterFilter {
	if c.next != nil {
		return c.next
	}
	return nil
}

func (c *ClusterMetricsFilter) Filter(haStrategy motan.HaStrategy, loadBalance motan.LoadBalance, request motan.Request) motan.Response {
	start := time.Now()

	response := c.GetNext().Filter(haStrategy, loadBalance, request)

	role := "motan-client"
	ctx := request.GetRPCContext(false)
	if ctx != nil && ctx.Proxy {
		role = "motan-client-agent"
	}
	keys := []string{role, request.GetAttachment(protocol.MSource), request.GetMethod()}
	addMetricWithKeys(request.GetAttachment(protocol.MGroup), ".cluster",
		request.GetAttachment(protocol.MPath), keys, time.Since(start).Nanoseconds()/1e6, response)
	return response
}
