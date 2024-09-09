package lb

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
	"github.com/weibocom/motan-go/meta"
	"math"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"
)

func TestDynamicStaticWeight(t *testing.T) {
	url := &core.URL{
		Protocol: "motan2",
		Host:     "127.0.0.1",
		Port:     8080,
		Path:     "mockService",
	}
	url.PutParam(core.DynamicMetaKey, "false")
	// test dynamic weight config
	lb := NewWeightRondRobinLb(url)
	assert.False(t, lb.refresher.supportDynamicWeight)
	lb.Destroy()

	url.PutParam(core.DynamicMetaKey, "true")
	lb = NewWeightRondRobinLb(url)
	assert.True(t, lb.refresher.supportDynamicWeight)

	meta.ClearMetaCache()
	var staticWeight int64 = 9
	eps := buildTestDynamicEps(10, true, staticWeight, url)
	eps = core.EndpointShuffle(eps)
	lb.OnRefresh(eps)
	_, ok := lb.getSelector().(*roundRobinSelector)
	assert.True(t, ok)
	for _, j := range lb.refresher.weightedEpHolders.Load().([]*WeightedEpHolder) {
		// test static weight
		assert.Equal(t, j.staticWeight, staticWeight)
		assert.Equal(t, j.dynamicWeight, int64(0))
		assert.Equal(t, j.getWeight(), staticWeight)
	}

	// test dynamic wight change
	lb.refresher.weightedEpHolders.Load().([]*WeightedEpHolder)[3].ep.(*endpoint.MockDynamicEndpoint).SetWeight(true, 22)
	meta.ClearMetaCache()
	time.Sleep(time.Second * 5)
	//assert.Equal(t, int64(22), lb.refresher.weightedEpHolders.Load().([]*WeightedEpHolder)[3].dynamicWeight)
	//assert.Equal(t, int64(22), lb.refresher.weightedEpHolders.Load().([]*WeightedEpHolder)[3].getWeight())
	_, ok = lb.getSelector().(*weightedRingSelector)
	assert.True(t, ok)
	// test close refresh task
	lb.Destroy()
	time.Sleep(time.Second * 5)
	assert.True(t, lb.refresher.isDestroyed.Load())
}

func TestGetEpWeight(t *testing.T) {
	url := &core.URL{
		Protocol: "motan2",
		Host:     "127.0.0.1",
		Port:     8080,
		Path:     "mockService",
	}
	ep := endpoint.NewMockDynamicEndpoint(url)
	type test struct {
		expectWeight     int64
		fromDynamic      bool
		defaultWeight    int64
		setWeight        bool
		setWeightDynamic bool
		setWeightValue   int64
	}
	testSet := []test{
		// test static weight
		{expectWeight: 11, fromDynamic: false, defaultWeight: 11, setWeight: false},
		{expectWeight: 5, fromDynamic: false, defaultWeight: 5, setWeight: false},
		{expectWeight: 8, fromDynamic: false, defaultWeight: 11, setWeight: true, setWeightDynamic: false, setWeightValue: 8},
		// test dynamic weight
		{expectWeight: 0, fromDynamic: true, defaultWeight: 0, setWeight: false},
		{expectWeight: 5, fromDynamic: true, defaultWeight: 5, setWeight: false},
		{expectWeight: 15, fromDynamic: true, defaultWeight: 11, setWeight: true, setWeightDynamic: true, setWeightValue: 15},
		// test abnormal weight
		{expectWeight: MinEpWeight, fromDynamic: false, defaultWeight: 11, setWeight: true, setWeightDynamic: false, setWeightValue: -8},
		{expectWeight: MaxEpWeight, fromDynamic: false, defaultWeight: 11, setWeight: true, setWeightDynamic: false, setWeightValue: 501},
		{expectWeight: MinEpWeight, fromDynamic: true, defaultWeight: 11, setWeight: true, setWeightDynamic: true, setWeightValue: -1},
		{expectWeight: MaxEpWeight, fromDynamic: true, defaultWeight: 11, setWeight: true, setWeightDynamic: true, setWeightValue: 666},
	}

	for _, j := range testSet {
		if j.setWeight {
			ep.SetWeight(j.setWeightDynamic, j.setWeightValue)
		}
		w, e := getEpWeight(ep, j.fromDynamic, j.defaultWeight)
		assert.Nil(t, e)
		assert.Equal(t, j.expectWeight, w)
		meta.ClearMetaCache()
	}
}

func TestNotifyWeightChange(t *testing.T) {
	type test struct {
		size       int
		sameWeight bool
		maxWeight  int
		selector   string
	}
	testSet := []test{
		// test RR
		{size: 20, sameWeight: true, maxWeight: 8, selector: "roundRobinSelector"},
		{size: 20, sameWeight: true, maxWeight: 0, selector: "roundRobinSelector"},
		{size: 20, sameWeight: true, maxWeight: 101, selector: "roundRobinSelector"},
		{size: 20, sameWeight: true, maxWeight: -10, selector: "roundRobinSelector"}, //abnormal weight
		//// test WR
		{size: 20, sameWeight: false, maxWeight: 8, selector: "weightedRingSelector"},
		{size: wrMaxEpSize, sameWeight: false, maxWeight: 8, selector: "weightedRingSelector"},
		{size: wrMaxEpSize, sameWeight: false, maxWeight: wrMaxTotalWeight / wrMaxEpSize, selector: "weightedRingSelector"},
		// test SWWRR
		{size: wrMaxEpSize + 1, sameWeight: false, maxWeight: 6, selector: "slidingWindowWeightedRoundRobinSelector"},
		{size: 40, sameWeight: false, maxWeight: 800, selector: "slidingWindowWeightedRoundRobinSelector"},
	}
	url := &core.URL{
		Protocol: "motan2",
		Host:     "127.0.0.1",
		Port:     8080,
		Path:     "mockService",
	}
	url.PutParam(core.DynamicMetaKey, "false")
	lb := NewWeightRondRobinLb(url)
	for _, j := range testSet {
		eps := buildTestDynamicEps(j.size, j.sameWeight, int64(j.maxWeight), url)
		eps = core.EndpointShuffle(eps)
		lb.OnRefresh(eps)
		var ok bool
		switch j.selector {
		case "roundRobinSelector":
			_, ok = lb.getSelector().(*roundRobinSelector)
		case "weightedRingSelector":
			_, ok = lb.getSelector().(*weightedRingSelector)
		case "slidingWindowWeightedRoundRobinSelector":
			_, ok = lb.getSelector().(*slidingWindowWeightedRoundRobinSelector)
		}
		assert.True(t, ok)
		meta.ClearMetaCache()
	}
	lb.Destroy()
}

func TestRoundRobinSelector(t *testing.T) {
	url := &core.URL{
		Protocol: "motan2",
		Host:     "127.0.0.1",
		Port:     8080,
		Path:     "mockService",
	}
	url.PutParam(core.DynamicMetaKey, "false")
	lb := NewWeightRondRobinLb(url)
	round := 100
	// small size
	checkRR(t, lb, 20, 8, round, 1, 1, 0, url)
	// large size
	checkRR(t, lb, 500, 8, round, 1, 1, 0, url)
	// some nodes are unavailable
	maxRatio := 0.4
	avgRatio := 0.1
	round = 200
	checkRR(t, lb, 20, 8, round, float64(round)*maxRatio, float64(round)*avgRatio, 2, url)
	checkRR(t, lb, 100, 8, round, float64(round)*maxRatio, float64(round)*avgRatio, 10, url)

	maxRatio = 0.7
	checkRR(t, lb, 300, 8, round, float64(round)*maxRatio, float64(round)*avgRatio, 50, url)
	lb.Destroy()
}

func checkRR(t *testing.T, lb *WeightRoundRobinLB, size int, initialMaxWeight int64,
	round int, expectMaxDelta float64, expectAvgDelta float64, unavailableSize int, url *core.URL) {
	eps := buildTestDynamicEpsWithUnavailable(size, true, initialMaxWeight, true, unavailableSize, url)
	rand.Seed(time.Now().UnixNano())
	eps = core.EndpointShuffle(eps)
	lb.OnRefresh(eps)
	_, ok := lb.getSelector().(*roundRobinSelector)
	assert.True(t, ok)
	processCheck(t, lb, "RR", eps, round, expectMaxDelta, expectAvgDelta, unavailableSize)
}

func TestWeightRingSelector(t *testing.T) {
	url := &core.URL{
		Protocol: "motan2",
		Host:     "127.0.0.1",
		Port:     8080,
		Path:     "mockService",
	}
	url.PutParam(core.DynamicMetaKey, "false")
	lb := NewWeightRondRobinLb(url)
	round := 100
	// small size
	checkKWR(t, lb, 51, 49, round, 1, 1, 0, url)
	// max node size of WR
	checkKWR(t, lb, 256, 15, round, 1, 1, 0, url)

	// same nodes are unavailable
	maxRatio := 0.4
	avgRatio := 0.1
	checkKWR(t, lb, 46, 75, round, float64(round)*maxRatio, float64(round)*avgRatio, 5, url)
	checkKWR(t, lb, 231, 31, round, float64(round)*maxRatio, float64(round)*avgRatio, 35, url)
	maxRatio = 0.6
	//checkKWR(t, lb, 211, 31, round, float64(round)*maxRatio, float64(round)*avgRatio, 45, url)
	lb.Destroy()
}

func checkKWR(t *testing.T, lb *WeightRoundRobinLB, size int, initialMaxWeight int64,
	round int, expectMaxDelta float64, expectAvgDelta float64, unavailableSize int, url *core.URL) {
	eps := buildTestDynamicEpsWithUnavailable(size, false, initialMaxWeight, true, unavailableSize, url)
	rand.Seed(time.Now().UnixNano())
	eps = core.EndpointShuffle(eps)
	lb.OnRefresh(eps)
	_, ok := lb.getSelector().(*weightedRingSelector)
	assert.True(t, ok)
	processCheck(t, lb, "WR", eps, round, expectMaxDelta, expectAvgDelta, unavailableSize)
}

func TestSlidingWindowWeightedRoundRobinSelector(t *testing.T) {
	url := &core.URL{
		Protocol: "motan2",
		Host:     "127.0.0.1",
		Port:     8080,
		Path:     "mockService",
	}
	url.PutParam(core.DynamicMetaKey, "false")
	lb := NewWeightRondRobinLb(url)
	round := 100
	// equals default window size, the accuracy is higher than sliding window
	size := swwrDefaultWindowSize
	checkSWWRR(t, lb, size, int64(wrMaxTotalWeight*3/size), round, 2, 1, 0, url)
	// less than default window size
	size = swwrDefaultWindowSize - 9
	checkSWWRR(t, lb, size, int64(wrMaxTotalWeight*3/size), round, 2, 1, 0, url)

	// greater than default window size
	// sliding windows will reduce the accuracy of WRR, so the threshold should be appropriately increased
	maxRatio := 0.5
	avgRatio := 0.1
	round = 200
	size = 270
	checkSWWRR(t, lb, size, 45, round, float64(round)*maxRatio, float64(round)*avgRatio, 0, url)

	// some nodes are unavailable
	size = 260
	checkSWWRR(t, lb, size, int64(wrMaxTotalWeight*3/size), round, float64(round)*maxRatio, float64(round)*avgRatio, 10, url)
	size = 399
	checkSWWRR(t, lb, size, 67, round, float64(round)*maxRatio, float64(round)*avgRatio, 40, url)
	lb.Destroy()
}

func checkSWWRR(t *testing.T, lb *WeightRoundRobinLB, size int, initialMaxWeight int64,
	round int, expectMaxDelta float64, expectAvgDelta float64, unavailableSize int, url *core.URL) {
	eps := buildTestDynamicEpsWithUnavailable(size, false, initialMaxWeight, true, unavailableSize, url)
	rand.Seed(time.Now().UnixNano())
	eps = core.EndpointShuffle(eps)
	lb.OnRefresh(eps)
	_, ok := lb.getSelector().(*slidingWindowWeightedRoundRobinSelector)
	assert.True(t, ok)
	processCheck(t, lb, "SWWRR", eps, round, expectMaxDelta, expectAvgDelta, unavailableSize)
}

func processCheck(t *testing.T, lb *WeightRoundRobinLB, typ string, eps []core.EndPoint, round int,
	expectMaxDelta float64, expectAvgDelta float64, unavailableSize int) {
	var totalWeight int64 = 0
	for _, ep := range eps {
		if !ep.IsAvailable() {
			continue
		}
		totalWeight += ep.(*endpoint.MockDynamicEndpoint).StaticWeight
	}
	for i := 0; i < int(totalWeight)*round; i++ {
		lb.Select(nil).Call(nil)
	}
	var maxDelta float64 = 0.0
	var totalDelta float64 = 0.0
	unavailableCount := 0
	for _, ep := range eps {
		if !ep.IsAvailable() {
			unavailableCount++
		} else {
			mep := ep.(*endpoint.MockDynamicEndpoint)
			ratio := float64(atomic.LoadInt64(&mep.Count)) / float64(mep.StaticWeight)
			delta := math.Abs(ratio - float64(round))
			if delta > maxDelta {
				maxDelta = delta
			}
			totalDelta += delta
			if delta > expectMaxDelta {
				fmt.Printf("%s: count=%d, staticWeight=%d, ratio=%.2f, delta=%.2f\n", typ, atomic.LoadInt64(&mep.Count), mep.StaticWeight, ratio, delta)
			}
			assert.True(t, delta <= expectMaxDelta) // check max delta
		}
	}
	// avg delta
	avgDelta := totalDelta / float64(len(eps)-unavailableSize)
	assert.True(t, avgDelta-float64(round) < expectAvgDelta)
	fmt.Printf("%s: avgDeltaPercent=%.2f%%, maxDeltaPercent=%.2f%%, avgDelta=%.2f, maxDelta=%.2f\n", typ, avgDelta*float64(100)/float64(round), maxDelta*float64(100)/float64(round), avgDelta, maxDelta)
	if unavailableSize > 0 {
		assert.Equal(t, unavailableSize, unavailableCount)
	}
}

func buildTestDynamicEps(size int, sameStaticWeight bool, maxWeight int64, url *core.URL) []core.EndPoint {
	return buildTestDynamicEpsWithUnavailable(size, sameStaticWeight, maxWeight, false, 0, url)
}

func buildTestDynamicEpsWithUnavailable(size int, sameStaticWeight bool, maxWeight int64, adjust bool, unavailableSize int, url *core.URL) []core.EndPoint {
	return buildTestEps(size, sameStaticWeight, maxWeight, adjust, unavailableSize, "", url)
}

func buildTestEps(size int, sameStaticWeight bool, maxWeight int64, adjust bool, unavailableSize int, group string, url *core.URL) []core.EndPoint {
	rand.Seed(time.Now().UnixNano())
	var res []core.EndPoint
	for i := 0; i < size; i++ {
		weight := maxWeight
		if !sameStaticWeight {
			weight = int64(rand.Float64() * float64(maxWeight))
		}
		if adjust {
			weight = doAdjust(weight)
		}
		curUrl := &core.URL{
			Protocol: "motan2",
			Host:     "127.0.0.1",
			Port:     8080 + i,
			Path:     "mockService",
		}
		if url.GetParam(core.DynamicMetaKey, "") == "false" {
			curUrl.PutParam(core.DynamicMetaKey, "false")
		}
		ep := endpoint.NewMockDynamicEndpointWithWeight(curUrl, weight)
		if i < unavailableSize {
			ep.SetAvailable(false)
		}
		if group != "" {
			ep.URL.PutParam(core.GroupKey, group)
		}
		res = append(res, ep)
	}
	return res
}

func doAdjust(w int64) int64 {
	if w < MinEpWeight {
		return MinEpWeight
	} else if w > MaxEpWeight {
		return MaxEpWeight
	} else {
		return w
	}
}
