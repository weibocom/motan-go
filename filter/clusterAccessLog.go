package filter

import (
	"time"

	motan "github.com/weibocom/motan-go/core"
)

type ClusterAccessLogFilter struct {
	next motan.ClusterFilter
}

func (t *ClusterAccessLogFilter) GetIndex() int {
	return 1
}

func (t *ClusterAccessLogFilter) GetName() string {
	return ClusterAccessLog
}

func (t *ClusterAccessLogFilter) NewFilter(url *motan.URL) motan.Filter {
	return &ClusterAccessLogFilter{}
}

func (t *ClusterAccessLogFilter) Filter(haStrategy motan.HaStrategy, loadBalance motan.LoadBalance, request motan.Request) motan.Response {
	start := time.Now()
	response := t.GetNext().Filter(haStrategy, loadBalance, request)
	doAccessLog(t.GetName(), clientAgentRole, "", start, request, response)
	return response
}

func (t *ClusterAccessLogFilter) HasNext() bool {
	return t.next != nil
}

func (t *ClusterAccessLogFilter) SetNext(nextFilter motan.ClusterFilter) {
	t.next = nextFilter
}

func (t *ClusterAccessLogFilter) GetNext() motan.ClusterFilter {
	return t.next
}

func (t *ClusterAccessLogFilter) GetType() int32 {
	return motan.ClusterFilterType
}
