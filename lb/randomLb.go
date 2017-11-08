package lb

import (
	motan "github.com/weibocom/motan-go/core"
	"math/rand"
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
	_, endpoint := r.randomSelect(eps)
	return endpoint
}
func (r *RandomLB) SelectArray(request motan.Request) []motan.EndPoint {
	eps := r.endpoints
	index, endpoint := r.randomSelect(eps)
	if endpoint == nil {
		return nil
	}
	return selectArrayFromIndex(eps, index)
}

func (r *RandomLB) SetWeight(weight string) {
	r.weight = weight
}

func (r *RandomLB) randomSelect(eps []motan.EndPoint) (int, motan.EndPoint) {
	epsLen := len(eps)
	if epsLen == 0 {
		return -1, nil
	}
	index := rand.Intn(epsLen)
	if eps[index].IsAvailable() {
		return index, eps[index]
	}
	return selectOneAtRandom(eps)
}
