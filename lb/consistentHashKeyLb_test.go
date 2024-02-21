package lb

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
	"testing"
)

func TestConsistentHashLB_OnRefresh(t *testing.T) {
	lb := &ConsistentHashLB{}

	// first refresh
	resetLB(lb)
	checkDefault(t, lb, 10)

	// refresh multiple times
	sizeArray := []int{2, 8, 12, 51, 100, 123, 500, 1000}
	for _, s := range sizeArray {
		if s >= 500 {
			lb.replica = 10 // To reduce testing costs
		}
		checkDefault(t, lb, s)
	}

	// specified part size, loadFactor, replica
	resetLB(lb)
	lb.url.PutParam(PartSizeKey, "100")
	lb.url.PutParam(LoadKey, "1.35")
	lb.url.PutParam(ReplicaKey, "5")
	checkDefault(t, lb, 50)
	assert.Equal(t, 100, lb.lastPartSize)
	assert.Equal(t, 1.35, lb.load)
	assert.Equal(t, 5, lb.replica)

	// inappropriate value specified
	resetLB(lb)
	lb.url.PutParam(PartSizeKey, "70") // < 2 * len(eps)
	lb.url.PutParam(LoadKey, "1.0")    // < 1.1
	lb.url.PutParam(ReplicaKey, "-5")  // < 0
	checkDefault(t, lb, 50)
	assert.NotEqual(t, 70, lb.lastPartSize)
	assert.NotEqual(t, 1.0, lb.load)
	assert.NotEqual(t, -5, lb.replica)

	// change part size
	resetLB(lb)
	lb.url.PutParam(PartSizeKey, "100")
	checkDefault(t, lb, 30)
	assert.Equal(t, 100, lb.lastPartSize)
	checkDefault(t, lb, 55)
	assert.NotEqual(t, 100, lb.lastPartSize)

	// change loadFactor
	resetLB(lb)
	lb.load = 1.0
	checkDefault(t, lb, 30)
}

func TestConsistentHashLB_Select(t *testing.T) {
	lb := &ConsistentHashLB{}
	resetLB(lb)
	req := &motan.MotanRequest{Method: "test"}

	// one endpoint
	lb.OnRefresh(buildEndpoint(1))
	checkHashEP(t, lb, req, lb.endpoints[0])

	// use hash key
	req.SetAttachment(motan.ConsistentHashKey, "234234")
	checkHashEP(t, lb, req, lb.endpoints[0])

	// repeat select
	lb.OnRefresh(buildEndpoint(30))
	keys := []string{"234", "1", "sjide", "Y&^(U23j49", "skd9f0i9*(RK3erp3oi29kf"}
	for _, k := range keys {
		checkConsistent(t, lb, k, 20)
	}

	// not use hash key(random)
	req.SetAttachment(motan.ConsistentHashKey, "")
	ep := lb.Select(req)
	ep2 := lb.Select(req)
	assert.NotEqual(t, ep, ep2)

	// change part size
	resetLB(lb)
	lb.url.PutParam(PartSizeKey, "100")
	lb.OnRefresh(buildEndpoint(49))
	key := "sjide123"
	checkConsistent(t, lb, key, 20)
	req.SetAttachment(motan.ConsistentHashKey, key)
	ep = lb.Select(req)
	lb.OnRefresh(buildEndpoint(51))
	checkConsistent(t, lb, key, 20)
	ep2 = lb.Select(req)
	assert.NotEqual(t, ep, ep2)

	// endpoint unavailable
	resetLB(lb)
	lb.OnRefresh(buildEndpoint(30))
	ep = lb.Select(req)
	ep.(*lbTestMockEndpoint).isAvail = false
	ep2 = lb.Select(req)
	assert.NotEqual(t, ep, ep2)
}

func TestConsistentHashLB_SelectArray(t *testing.T) {
	lb := &ConsistentHashLB{}
	resetLB(lb)
	req := &motan.MotanRequest{Method: "test"}

	// less endpoint
	lb.OnRefresh(buildEndpoint(MaxSelectArraySize))
	eps := lb.SelectArray(req)
	assert.Equal(t, MaxSelectArraySize, len(eps))

	// use hash key
	resetLB(lb)
	lb.OnRefresh(buildEndpoint(30))
	req.SetAttachment(motan.ConsistentHashKey, "234234")
	ep := lb.Select(req)
	eps = lb.SelectArray(req)
	assert.Equal(t, ep, eps[0])
	assert.Equal(t, MaxSelectArraySize, len(eps))

	// repeat select
	var eps2 []motan.EndPoint
	for i := 0; i < 20; i++ {
		eps2 = lb.SelectArray(req)
		assert.Equal(t, eps, eps2)
	}

	// test random
	req.SetAttachment(motan.ConsistentHashKey, "")
	eps = lb.SelectArray(req)
	eps2 = lb.SelectArray(req)
	assert.NotEqual(t, eps, eps2)
}

func TestConsistentMigration(t *testing.T) {
	lb := &ConsistentHashLB{}
	resetLB(lb)

	// distribution migration under default configuration
	// endpoint changes slightly
	diffDistribution(lb, 49, 50)
	diffDistribution(lb, 49, 51)
	diffDistribution(lb, 49, 48)
	diffDistribution(lb, 49, 47)

	// endpoint changes significantly
	diffDistribution(lb, 49, 70)
	diffDistribution(lb, 49, 99)
	diffDistribution(lb, 49, 31)
	diffDistribution(lb, 49, 27)
	diffDistribution(lb, 500, 1000)
}

func checkDefault(t *testing.T, lb *ConsistentHashLB, epsSize int) {
	eps := buildEndpoint(epsSize)
	lb.OnRefresh(eps)
	assert.True(t, lb.lastPartSize >= len(eps)*2)
	assert.True(t, lb.load >= DefaultMinLoad)
	assert.True(t, lb.replica > 0)
	assert.NotNil(t, lb.cHash)
	assert.Equal(t, eps, lb.endpoints)
}

func checkHashEP(t *testing.T, lb *ConsistentHashLB, req motan.Request, expectEP motan.EndPoint) {
	ep := lb.Select(req)
	assert.Equal(t, expectEP, ep)
}

func checkConsistent(t *testing.T, lb *ConsistentHashLB, hashKey string, times int) {
	req := &motan.MotanRequest{Method: "test"}
	req.SetAttachment(motan.ConsistentHashKey, hashKey)
	ep := lb.Select(req)
	for i := 0; i < times; i++ {
		assert.Equal(t, ep, lb.Select(req))
	}
}

func diffDistribution(lb *ConsistentHashLB, epSize1 int, epSize2 int) int {
	lb.OnRefresh(buildEndpoint(epSize1))
	cHash1 := lb.cHash
	partSize := lb.lastPartSize
	lb.OnRefresh(buildEndpoint(epSize2))
	cHash2 := lb.cHash
	if lb.lastPartSize < partSize {
		partSize = lb.lastPartSize
	}
	diff := 0
	for i := 0; i < partSize; i++ {
		if cHash1.GetPartitionOwner(i).String() != cHash2.GetPartitionOwner(i).String() {
			diff++
		}
	}
	fmt.Printf("=== ep size %d -> %d, diff distribution: %f\n", epSize1, epSize2, float64(diff)/float64(partSize))
	return diff
}

func resetLB(lb *ConsistentHashLB) {
	lb.endpoints = nil
	lb.lastPartSize = 0
	lb.load = 0
	lb.replica = 0
	lb.cHash = nil
	lb.url = &motan.URL{Host: "localhost", Port: 0, Protocol: "motan2"}
}

func buildEndpoint(size int) []motan.EndPoint {
	endpoints := make([]motan.EndPoint, 0, size)
	for i := 0; i < size; i++ {
		url := &motan.URL{Host: "10.10.10." + fmt.Sprintf("%d", i), Port: 8000 + i, Protocol: "motan2"}
		endpoints = append(endpoints, &lbTestMockEndpoint{MockEndpoint: &endpoint.MockEndpoint{URL: url}, index: i, isAvail: true})
	}
	return endpoints
}
