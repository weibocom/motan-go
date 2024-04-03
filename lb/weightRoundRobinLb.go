package lb

import (
	motan "github.com/weibocom/motan-go/core"
	vlog "github.com/weibocom/motan-go/log"
	"math/rand"
	"sync"
	"sync/atomic"
)

type WeightRoundRobinLB struct {
	selector  Selector
	mutex     sync.RWMutex
	refresher *WeightedEpRefresher
}

func NewWeightRondRobinLb(url *motan.URL) *WeightRoundRobinLB {
	lb := &WeightRoundRobinLB{}
	lb.refresher = NewWeightEpRefresher(url, lb)
	return lb
}

func (r *WeightRoundRobinLB) OnRefresh(endpoints []motan.EndPoint) {
	r.refresher.RefreshWeightedHolders(endpoints)
}

func (r *WeightRoundRobinLB) Select(request motan.Request) motan.EndPoint {
	selector := r.getSelector()
	if selector == nil {
		return nil
	}
	return selector.(Selector).DoSelect(request)
}

func (r *WeightRoundRobinLB) SelectArray(request motan.Request) []motan.EndPoint {
	// cannot use select array, return nil
	return nil
}

func (r *WeightRoundRobinLB) SetWeight(weight string) {}

func (r *WeightRoundRobinLB) Destroy() {
	r.refresher.Destroy()
}

func (r *WeightRoundRobinLB) getSelector() Selector {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.selector
}

func (r *WeightRoundRobinLB) setSelector(s Selector) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.selector = s
}

func (r *WeightRoundRobinLB) NotifyWeightChange() {
	var tempHolders []*WeightedEpHolder
	h := r.refresher.weightedEpHolders.Load()
	if h != nil {
		tempHolders = h.([]*WeightedEpHolder)
	} else {
		// fast stop
		return
	}
	weights := make([]int, len(tempHolders))
	haveSameWeight := true
	totalWeight := 0
	for i := 0; i < len(tempHolders); i++ {
		weights[i] = int(tempHolders[i].getWeight())
		totalWeight += weights[i]
		if weights[i] != weights[0] {
			haveSameWeight = false
		}
	}
	// if all eps have the same weight, then use RoundRobinSelector
	if haveSameWeight { // use RoundRobinLoadBalance
		selector := r.getSelector()
		if selector != nil {
			if v, ok := selector.(*roundRobinSelector); ok { // reuse the RoundRobinSelector
				v.refresh(tempHolders)
				return
			}
		}
		// new RoundRobinLoadBalance
		r.setSelector(newRoundRobinSelector(tempHolders))
		vlog.Infoln("WeightRoundRobinLoadBalance use RoundRobinSelector. url:" + r.getURLLogInfo())
		return
	}
	// find the GCD and divide the weights
	gcd := findGcd(weights)
	if gcd > 1 {
		totalWeight = 0 // recalculate totalWeight
		for i := 0; i < len(weights); i++ {
			weights[i] /= gcd
			totalWeight += weights[i]
		}
	}
	// Check whether it is suitable to use WeightedRingSelector
	if len(weights) <= wrMaxEpSize && totalWeight <= wrMaxTotalWeight {
		r.setSelector(newWeightedRingSelector(tempHolders, totalWeight, weights))
		vlog.Infoln("WeightRoundRobinLoadBalance use WeightedRingSelector. url:" + r.getURLLogInfo())
		return
	}
	r.setSelector(newSlidingWindowWeightedRoundRobinSelector(tempHolders, weights))
	vlog.Infoln("WeightRoundRobinLoadBalance use SlidingWindowWeightedRoundRobinSelector. url:" + r.getURLLogInfo())
}

func (r *WeightRoundRobinLB) getURLLogInfo() string {
	url := r.refresher.url
	if url == nil {
		holders := r.refresher.weightedEpHolders.Load()
		if v, ok := holders.([]*WeightedEpHolder); ok {
			if len(v) > 0 {
				url = v[0].ep.GetURL()
			}
		}
	}
	if url == nil {
		return ""
	}
	return url.GetIdentity()
}

type Selector interface {
	DoSelect(request motan.Request) motan.EndPoint
}

type roundRobinSelector struct {
	weightHolders atomic.Value
	idx           int64
}

func newRoundRobinSelector(holders []*WeightedEpHolder) *roundRobinSelector {
	res := &roundRobinSelector{}
	res.weightHolders.Store(holders)
	return res
}

func (r *roundRobinSelector) DoSelect(request motan.Request) motan.EndPoint {
	temp := r.weightHolders.Load()
	if temp == nil {
		return nil
	}
	tempHolders := temp.([]*WeightedEpHolder)
	ep := tempHolders[int(motan.GetNonNegative(atomic.AddInt64(&r.idx, 1)))%len(tempHolders)].ep
	if ep.IsAvailable() {
		return ep
	}
	// if the ep is not available, loop section from random position
	start := rand.Intn(len(tempHolders))
	for i := 0; i < len(tempHolders); i++ {
		ep = tempHolders[(i+start)%len(tempHolders)].ep
		if ep.IsAvailable() {
			return ep
		}
	}
	return nil
}

func (r *roundRobinSelector) refresh(holders []*WeightedEpHolder) {
	r.weightHolders.Store(holders)
}

const (
	wrMaxEpSize      = 256
	wrMaxTotalWeight = 256 * 20
)

type weightedRingSelector struct {
	weightHolders []*WeightedEpHolder
	idx           int64
	weights       []int
	weightRing    []byte
}

func newWeightedRingSelector(holders []*WeightedEpHolder, totalWeight int, weights []int) *weightedRingSelector {
	wrs := &weightedRingSelector{
		weightHolders: holders,
		weightRing:    make([]byte, totalWeight),
		weights:       weights,
	}
	wrs.initWeightRing()
	return wrs
}

func (r *weightedRingSelector) initWeightRing() {
	ringIndex := 0
	for i := 0; i < len(r.weights); i++ {
		for j := 0; j < r.weights[i]; j++ {
			r.weightRing[ringIndex] = byte(i)
			ringIndex++
		}
	}
	if ringIndex != len(r.weightRing) {
		vlog.Warningf("WeightedRingSelector initWeightRing with wrong totalWeight. expect:%d, actual: %d", len(r.weightRing), ringIndex)
	}
	r.weightRing = motan.ByteSliceShuffle(r.weightRing)

}

func (r *weightedRingSelector) DoSelect(request motan.Request) motan.EndPoint {
	ep := r.weightHolders[r.getHolderIndex(int(motan.GetNonNegative(atomic.AddInt64(&r.idx, 1))))].ep
	if ep.IsAvailable() {
		return ep
	}
	// If the ep is not available, loop selection from random position
	start := rand.Intn(len(r.weightRing))
	for i := 0; i < len(r.weightRing); i++ {
		// byte could indicate 0~255
		ep = r.weightHolders[r.getHolderIndex(start+i)].getEp()
		if ep.IsAvailable() {
			return ep
		}
	}
	return nil
}

func (r *weightedRingSelector) getHolderIndex(ringIndex int) int {
	holderIndex := int(r.weightRing[ringIndex%len(r.weightRing)])
	return holderIndex
}

const (
	swwrDefaultWindowSize = 50
)

type slidingWindowWeightedRoundRobinSelector struct {
	idx        int64
	windowSize int
	items      []*selectorItem
}

func newSlidingWindowWeightedRoundRobinSelector(holders []*WeightedEpHolder, weights []int) *slidingWindowWeightedRoundRobinSelector {
	windowSize := len(weights)
	if windowSize > swwrDefaultWindowSize {
		windowSize = swwrDefaultWindowSize
		// The window size cannot be divided by the number of referers, which ensures that the starting position
		// of the window will gradually change during sliding
		for len(weights)%windowSize == 0 {
			windowSize--
		}
	}
	items := make([]*selectorItem, 0, len(holders))
	for i := 0; i < len(weights); i++ {
		items = append(items, newSelectorItem(holders[i].getEp(), weights[i]))
	}
	return &slidingWindowWeightedRoundRobinSelector{
		items:      items,
		windowSize: windowSize,
	}
}

func (r *slidingWindowWeightedRoundRobinSelector) DoSelect(request motan.Request) motan.EndPoint {
	windowStartIndex := motan.GetNonNegative(atomic.AddInt64(&r.idx, int64(r.windowSize)))
	totalWeight := 0
	var sMaxWeight int64 = 0
	maxWeightIndex := 0
	// Use SWRR(https://github.com/nginx/nginx/commit/52327e0627f49dbda1e8db695e63a4b0af4448b1) to select referer from the current window.
	// In order to reduce costs, do not limit concurrency in the entire selection process,
	// and only use atomic updates for the current weight.
	// Since concurrent threads will execute Select in different windows,
	// the problem of instantaneous requests increase on one node due to concurrency will not be serious.
	// And because the referers used on different client sides are shuffled,
	// the impact of high instantaneous concurrent selection on the server side will be further reduced.
	for i := 0; i < r.windowSize; i++ {
		idx := (int(windowStartIndex) + i) % len(r.items)
		item := r.items[idx]
		if item.ep.IsAvailable() {
			currentWeight := atomic.AddInt64(&item.currentWeight, int64(item.weight))
			totalWeight += item.weight
			if currentWeight > sMaxWeight {
				sMaxWeight = currentWeight
				maxWeightIndex = idx
			}
		}
	}
	if sMaxWeight > 0 {
		item := r.items[maxWeightIndex]
		atomic.AddInt64(&item.currentWeight, int64(-totalWeight))
		if item.ep.IsAvailable() {
			return item.ep
		}
	}
	// If no suitable node is selected or the node is unavailable,
	// then select an available referer from a random index
	var idx = int(windowStartIndex) + rand.Intn(r.windowSize)
	for i := 1; i < len(r.items); i++ {
		item := r.items[(idx+i)%len(r.items)]
		if item.ep.IsAvailable() {
			return item.ep
		}
	}
	return nil
}

type selectorItem struct {
	ep            motan.EndPoint
	weight        int
	currentWeight int64
}

func newSelectorItem(ep motan.EndPoint, weight int) *selectorItem {
	return &selectorItem{
		weight: weight,
		ep:     ep,
	}
}
