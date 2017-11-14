package lb

import (
	motan "github.com/weibocom/motan-go/core"
)

type RandomLB struct {
	url       *motan.URL
	endpoints []motan.EndPoint
	weight    string
}

func (r *RandomLB) OnRefresh(endpoints []motan.EndPoint) {
	r.endpoints = endpoints
}
func (r *RandomLB) Select(request motan.Request) motan.EndPoint {
	eps := r.endpoints
	_, endpoint := SelectOneAtRandom(eps)
	return endpoint
}
func (r *RandomLB) SelectArray(request motan.Request) []motan.EndPoint {
	eps := r.endpoints
	index, endpoint := SelectOneAtRandom(eps)
	if endpoint == nil {
		return nil
	}
	return SelectArrayFromIndex(eps, index)
}

func (r *RandomLB) SetWeight(weight string) {
	r.weight = weight
}

