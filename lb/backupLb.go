package lb

import (
	motan "github.com/weibocom/motan-go/core"
)

type BackupLB struct {
	url       *motan.URL
	endpoints []motan.EndPoint
	weight    string
}

func (b *BackupLB) OnRefresh(endpoints []motan.EndPoint) {
	b.endpoints = endpoints
}
func (b *BackupLB) Select(request motan.Request) motan.EndPoint {
	eps := b.endpoints
	_, endpoint := SelectOneAtRandom(eps)
	return endpoint
}
func (b *BackupLB) SelectArray(request motan.Request) []motan.EndPoint {
	eps := b.endpoints
	index, endpoint := SelectOneAtRandom(eps)
	if endpoint == nil {
		return nil
	}
	return SelectArrayFromIndex(eps, index)
}

func (b *BackupLB) SetWeight(weight string) {
	b.weight = weight
}
