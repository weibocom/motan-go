package lb

import (
	"math/rand"
	"sync/atomic"

	motan "github.com/weibocom/motan-go/core"
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
	index := r.selectIndex(request)
	if index == -1 {
		return nil
	}
	return r.endpoints[index]
}

func (r *RoundrobinLB) SelectArray(request motan.Request) []motan.EndPoint {
	index := r.selectIndex(request)
	if index == -1 {
		return nil
	}
	epsLen := len(r.endpoints)
	eps := make([]motan.EndPoint, 0)
	for idx := 0; idx < epsLen && idx < MaxSelectArraySize; idx++ {
		if ep := r.endpoints[(index+idx)%epsLen]; ep.IsAvailable() {
			eps = append(eps, ep)
		}
	}
	return eps
}
func (r *RoundrobinLB) SetWeight(weight string) {
	r.weight = weight
}

func (r *RoundrobinLB) selectIndex(request motan.Request) int {
	eps := r.endpoints
	epsLen := len(eps)
	if epsLen == 0 {
		return -1
	}
	nextIndex := atomic.AddUint32(&r.index, 1)
	if index := nextIndex % uint32(epsLen); eps[index].IsAvailable() {
		return int(index)
	}
	// skip random length
	random := rand.Intn(epsLen)
	for idx := 0; idx < epsLen; idx++ {
		if index := (nextIndex + uint32(random) + uint32(idx)) % uint32(epsLen); eps[index].IsAvailable() {
			return int(index)
		}
	}
	return -1
}
