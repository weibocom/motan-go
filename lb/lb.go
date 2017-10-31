package lb

import (
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
)

var (
	lbmutex sync.Mutex
)

func RegistDefaultLb(extFactory motan.ExtentionFactory) {
	extFactory.RegistExtLb(Random, NewWeightLbFunc(func(url *motan.Url) motan.LoadBalance {
		return &RandomLB{url: url}
	}))

	extFactory.RegistExtLb(Roundrobin, NewWeightLbFunc(func(url *motan.Url) motan.LoadBalance {
		return &RoundrobinLB{url: url}
	}))
}

// support multi group weighted LB
type WeightedLbWraper struct {
	url          *motan.Url
	weightstring string
	refers       innerRefers
	newLb        motan.NewLbFunc
}

func NewWeightLbFunc(newLb motan.NewLbFunc) motan.NewLbFunc {
	return func(url *motan.Url) motan.LoadBalance {
		return &WeightedLbWraper{url: url, newLb: newLb, refers: &singleGroupRefers{lb: newLb(url)}}
	}
}

func (w *WeightedLbWraper) OnRefresh(endpoints []motan.EndPoint) {
	if w.weightstring == "" { //not weighted lb
		if sgr, ok := w.refers.(*singleGroupRefers); ok {
			sgr.lb.OnRefresh(endpoints)
		} else {
			lb := w.newLb(w.url)
			lb.OnRefresh(endpoints)
			w.refers = &singleGroupRefers{lb: lb}
		}
		return
	}

	// weighted lb
	lbmutex.Lock()
	defer lbmutex.Unlock()
	groupEp := make(map[string][]motan.EndPoint)
	for _, ep := range endpoints {
		ges := groupEp[ep.GetUrl().Group]
		if ges == nil {
			ges = make([]motan.EndPoint, 0, 32)
		}
		groupEp[ep.GetUrl().Group] = append(ges, ep)
	}

	weights := strings.Split(w.weightstring, ",")
	gws := make(map[string]int)
	for _, w := range weights {
		if w != "" {
			groupWeight := strings.Split(w, ":")
			if len(groupWeight) == 1 {
				gws[groupWeight[0]] = defaultWeight
			} else {
				w, err := strconv.Atoi(groupWeight[1])
				if err == nil {
					gws[groupWeight[0]] = w
				} else {
					gws[groupWeight[0]] = defaultWeight
				}
			}
		}
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
		if wi < 1 || wi > 100 { //weight normalization
			wi = defaultWeight
		}
		wr.groupWeight[g] = wi
		weightsArray = append(weightsArray, wi)
	}

	gcd := findGcd(weightsArray)
	ring := make([]string, 0, 128)
	for k, v := range wr.groupWeight {
		ringweight := v / gcd
		wr.groupWeight[k] = ringweight
		for i := 0; i < ringweight; i++ {
			ring = append(ring, k)
		}
	}
	wr.weightRing = motan.Slice_shuffle(ring)
	wr.ringSize = len(wr.weightRing)
	w.refers = wr
}

func (w *WeightedLbWraper) Select(request motan.Request) motan.EndPoint {
	return w.refers.selectNext(request)
}

func (w *WeightedLbWraper) SelectArray(request motan.Request) []motan.EndPoint {
	return w.refers.selectNextArray(request)
}

func (w *WeightedLbWraper) SetWeight(weight string) {
	w.weightstring = weight
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
	} else {
		return basegcd(m, n%m)
	}
}
