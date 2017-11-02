package lb

import (
	"math/rand"

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
	index := r.selectIndex(request)
	if index == -1 {
		return nil
	}
	return r.endpoints[index]
}
func (r *RandomLB) SelectArray(request motan.Request) []motan.EndPoint {
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
func (r *RandomLB) SetWeight(weight string) {
	r.weight = weight
}

func (r *RandomLB) selectIndex(request motan.Request) int {
	eps := r.endpoints
	epsLen := len(eps)
	if epsLen == 0 {
		return -1
	}
	index := rand.Intn(epsLen)
	if eps[index].IsAvailable() {
		return index
	}

	random := rand.Intn(epsLen)
	for idx := 0; idx < epsLen; idx++ {
		if index := (index + random + idx) % epsLen; eps[index].IsAvailable() {
			return index
		}
	}
	return -1
}
