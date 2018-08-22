package filter

import (
	"fmt"
	"strings"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/metrics"
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
	return "clusterMetrics"
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

	mP := strings.Replace(request.GetAttachment("M_p"), ".", "_", -1)
	key := fmt.Sprintf("motan-client-agent:%s:%s.cluster:%s:%s", request.GetAttachment("M_s"), request.GetAttachment("M_g"), mP, request.GetMethod())
	keyCount := key + ".total_count"
	metrics.AddCounter(keyCount, 1) //total_count

	if response.GetException() != nil { //err_count
		exception := response.GetException()
		if exception.ErrType == motan.BizException {
			bizErrCountKey := key + ".biz_error_count"
			metrics.AddCounter(bizErrCountKey, 1)
		} else {
			otherErrCountKey := key + ".other_error_count"
			metrics.AddCounter(otherErrCountKey, 1)
		}
	}

	end := time.Now()
	cost := end.Sub(start).Nanoseconds() / 1e6
	metrics.AddCounter(key+"."+metrics.ElapseTimeString(cost), 1)

	if cost > 200 {
		metrics.AddCounter(key+".slow_count", 1)
	}

	metrics.AddHistograms(key, cost)
	return response
}
