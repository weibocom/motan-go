package lb

import (
	vlog "github.com/weibocom/motan-go/log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	motan "github.com/weibocom/motan-go/core"
)

// ext name
const (
	Random     = "random"
	Roundrobin = "roundrobin"
)

const (
	MaxSelectArraySize = 3
	defaultWeight      = 1
	maxWeight          = 100
)

var (
	lbmutex sync.Mutex
)

func RegistDefaultLb(extFactory motan.ExtensionFactory) {
	extFactory.RegistExtLb(Random, NewWeightLbFunc(func(url *motan.URL) motan.LoadBalance {
		return &RandomLB{url: url}
	}))

	extFactory.RegistExtLb(Roundrobin, NewWeightLbFunc(func(url *motan.URL) motan.LoadBalance {
		return &RoundrobinLB{url: url}
	}))
}

// WeightedLbWrapper support multi group weighted LB
type WeightedLbWrapper struct {
	url          *motan.URL
	weightString string
	refers       innerRefers
	newLb        motan.NewLbFunc
}

func NewWeightLbFunc(newLb motan.NewLbFunc) motan.NewLbFunc {
	return func(url *motan.URL) motan.LoadBalance {
		return &WeightedLbWrapper{url: url, newLb: newLb, refers: &singleGroupRefers{lb: newLb(url)}}
	}
}

func (w *WeightedLbWrapper) OnRefresh(endpoints []motan.EndPoint) {
	if w.weightString == "" { //not weighted lb
		vlog.Infof("WeightedLbWrapper: %s - OnRefresh:not have weight", w.url.GetIdentity())
		w.onRefreshSingleGroup(endpoints)
		return
	}

	weights := strings.Split(w.weightString, ",")
	mixMode := true
	gws := make(map[string]int)
	for _, w := range weights {
		if w != "" {
			groupWeight := strings.Split(w, ":")
			if len(groupWeight) == 1 {
				gws[groupWeight[0]] = defaultWeight
			} else {
				mixMode = false // not mix groups if weight is set
				w, err := strconv.Atoi(groupWeight[1])
				if err == nil {
					//weight normalization
					if w < 1 {
						w = defaultWeight
					} else if w > maxWeight {
						w = maxWeight
					}
					gws[groupWeight[0]] = w
				} else {
					gws[groupWeight[0]] = defaultWeight
				}
			}
		}
	}

	if mixMode {
		vlog.Infof("WeightedLbWrapper: %s - OnRefresh:use mix mode. weight:%s", w.url.GetIdentity(), w.weightString)
		w.onRefreshSingleGroup(endpoints)
		return
	}

	// weighted lb
	lbmutex.Lock()
	defer lbmutex.Unlock()
	groupEp := make(map[string][]motan.EndPoint)
	for _, ep := range endpoints {
		ges := groupEp[ep.GetURL().Group]
		if ges == nil {
			ges = make([]motan.EndPoint, 0, 32)
		}
		groupEp[ep.GetURL().Group] = append(ges, ep)
	}

	weightsArray := make([]int, 0, 16)
	wr := newWeightRefers()
	for g, e := range groupEp {
		//build lb
		lb := w.newLb(w.url)
		lb.OnRefresh(e)
		wr.groupLb[g] = lb
		//build real weight
		wi := gws[g]
		wr.groupWeight[g] = wi
		weightsArray = append(weightsArray, wi)
	}

	gcd := findGcd(weightsArray)
	ring := make([]string, 0, 128)
	for k, v := range wr.groupWeight {
		ringWeight := v / gcd
		wr.groupWeight[k] = ringWeight
		for i := 0; i < ringWeight; i++ {
			ring = append(ring, k)
		}
	}
	wr.weightRing = motan.SliceShuffle(ring)
	wr.ringSize = len(wr.weightRing)
	w.refers = wr
	vlog.Infof("WeightedLbWrapper: %s - OnRefresh: weight:%s", w.url.GetIdentity(), w.weightString)
}

func (w *WeightedLbWrapper) onRefreshSingleGroup(endpoints []motan.EndPoint) {
	if sgr, ok := w.refers.(*singleGroupRefers); ok {
		sgr.lb.OnRefresh(endpoints)
	} else {
		lb := w.newLb(w.url)
		lb.OnRefresh(endpoints)
		w.refers = &singleGroupRefers{lb: lb}
	}
}

func (w *WeightedLbWrapper) Select(request motan.Request) motan.EndPoint {
	return w.refers.selectNext(request)
}

func (w *WeightedLbWrapper) SelectArray(request motan.Request) []motan.EndPoint {
	return w.refers.selectNextArray(request)
}

func (w *WeightedLbWrapper) SetWeight(weight string) {
	w.weightString = weight
}

type innerRefers interface {
	selectNext(request motan.Request) motan.EndPoint
	selectNextArray(request motan.Request) []motan.EndPoint
}

type singleGroupRefers struct {
	lb motan.LoadBalance
}

func (s *singleGroupRefers) selectNext(request motan.Request) motan.EndPoint {
	return s.lb.Select(request)
}

func (s *singleGroupRefers) selectNextArray(request motan.Request) []motan.EndPoint {
	return s.lb.SelectArray(request)
}

type weightedRefers struct {
	groupWeight map[string]int
	weightRing  []string //real group ring according to endpoints.
	ringSize    int
	groupLb     map[string]motan.LoadBalance
	index       uint32
}

func (w *weightedRefers) selectNext(request motan.Request) motan.EndPoint {
	nextIndex := atomic.AddUint32(&w.index, 1)
	g := w.weightRing[nextIndex%uint32(w.ringSize)]
	return w.groupLb[g].Select(request)
}

func (w *weightedRefers) selectNextArray(request motan.Request) []motan.EndPoint {
	nextIndex := atomic.AddUint32(&w.index, 1)
	g := w.weightRing[nextIndex%uint32(w.ringSize)]
	return w.groupLb[g].SelectArray(request)
}

func newWeightRefers() *weightedRefers {
	wr := &weightedRefers{}
	wr.groupWeight = make(map[string]int, 16)
	wr.weightRing = make([]string, 0, 32)
	wr.groupLb = make(map[string]motan.LoadBalance, 16)
	return wr
}

func findGcd(v []int) int {
	var gcd int
	gcd = v[0]
	for i := 1; i < len(v); i++ {
		gcd = basegcd(gcd, v[i])
	}
	return gcd
}

func basegcd(n int, m int) int {
	if n == 0 || m == 0 {
		return m + n
	}
	return basegcd(m, n%m)
}

func SelectArrayFromIndex(endpoints []motan.EndPoint, fromIndex int) []motan.EndPoint {
	if len(endpoints) == 0 || fromIndex < 0 {
		return make([]motan.EndPoint, 0)
	}
	epsLen := len(endpoints)
	epList := make([]motan.EndPoint, 0, MaxSelectArraySize)
	for idx := 0; idx < epsLen && len(epList) < MaxSelectArraySize; idx++ {
		if ep := endpoints[(fromIndex+idx)%epsLen]; ep.IsAvailable() {
			epList = append(epList, ep)
		}
	}
	return epList
}

// SelectOneAtRandom to prevent put pressure to the next node when a node being unavailable, then need to do two random
func SelectOneAtRandom(endpoints []motan.EndPoint) (int, motan.EndPoint) {
	epsLen := len(endpoints)
	if epsLen == 0 {
		return -1, nil
	}
	index := rand.Intn(epsLen)
	if endpoints[index].IsAvailable() {
		return index, endpoints[index]
	}
	random := rand.Intn(epsLen)
	for idx := 0; idx < epsLen; idx++ {
		if rndIndex := (random + idx) % epsLen; endpoints[rndIndex].IsAvailable() {
			return rndIndex, endpoints[rndIndex]
		}
	}
	return -1, nil
}
