package lb

import (
	motan "github.com/weibocom/motan-go/core"
	"sync/atomic"
)

type RoundrobinLB struct {
	url       *motan.URL
	endpoints []motan.EndPoint
	index     uint32
	weight    string
}

func (r *RoundrobinLB) OnRefresh(endpoints []motan.EndPoint) {
	r.endpoints = endpoints
}

func (r *RoundrobinLB) Select(request motan.Request) motan.EndPoint {
	eps := r.endpoints
	_, endpoint := r.roundrobinSelect(eps)
	return endpoint
}

func (r *RoundrobinLB) SelectArray(request motan.Request) []motan.EndPoint {
	eps := r.endpoints
	index, endpoint := r.roundrobinSelect(eps)
	if endpoint == nil {
		return nil
	}
	return SelectArrayFromIndex(eps, index)
}

func (r *RoundrobinLB) SetWeight(weight string) {
	r.weight = weight
}

func (r *RoundrobinLB) roundrobinSelect(eps []motan.EndPoint) (int, motan.EndPoint) {
	epsLen := len(eps)
	if epsLen == 0 {
		return -1, nil
	}
	nextIndex := atomic.AddUint32(&r.index, 1)
	if index := nextIndex % uint32(epsLen); eps[index].IsAvailable() {
		return int(index), eps[index]
	}
	return SelectOneAtRandom(eps)
}
