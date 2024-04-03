package lb

import (
	"errors"
	motan "github.com/weibocom/motan-go/core"
	vlog "github.com/weibocom/motan-go/log"
	"github.com/weibocom/motan-go/meta"
	"go.uber.org/atomic"
	"golang.org/x/net/context"
	"strconv"
	"sync"
	"time"
)

const (
	defaultEpWeight                  = 10
	MinEpWeight                      = 1
	MaxEpWeight                      = 500 //protective restriction
	defaultWeightRefreshPeriodSecond = 3
)

// WeightedEpRefresher is held by load balancer who needs dynamic endpoint weights
type WeightedEpRefresher struct {
	url                  *motan.URL
	refreshCanceler      context.CancelFunc
	supportDynamicWeight bool
	weightedEpHolders    atomic.Value
	weightLB             motan.WeightLoadBalance
	isDestroyed          atomic.Bool
	mutex                sync.Mutex
}

func NewWeightEpRefresher(url *motan.URL, lb motan.WeightLoadBalance) *WeightedEpRefresher {
	refreshPeriod := url.GetIntValue(motan.WeightRefreshPeriodSecondKey, defaultWeightRefreshPeriodSecond)
	refreshCtx, refreshCancel := context.WithCancel(context.Background())
	wer := &WeightedEpRefresher{
		url:                  url,
		supportDynamicWeight: url.GetBoolValue(motan.DynamicMetaKey, motan.DefaultDynamicMeta),
		refreshCanceler:      refreshCancel,
		weightLB:             lb,
	}
	go wer.doRefresh(refreshCtx, refreshPeriod)
	return wer
}

func (w *WeightedEpRefresher) Destroy() {
	w.refreshCanceler()
}

func (w *WeightedEpRefresher) notifyWeightChange() {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	vlog.Infoln("weight has changed, url: " + w.url.GetIdentity())
	w.weightLB.NotifyWeightChange()
}

func (w *WeightedEpRefresher) RefreshWeightedHolders(eps []motan.EndPoint) {
	var newHolder []*WeightedEpHolder
	var oldHolder []*WeightedEpHolder
	if t := w.weightedEpHolders.Load(); t != nil {
		oldHolder = t.([]*WeightedEpHolder)
	}
	allHolder := make([]*WeightedEpHolder, 0, len(eps))
	for _, ep := range eps {
		var holder *WeightedEpHolder
		if oldHolder != nil {
			// Check whether referer can be reused
			for _, tempHolder := range oldHolder {
				if ep == tempHolder.getEp() {
					holder = tempHolder
					break
				}
			}
		}
		if holder == nil {
			staticWeight, _ := getEpWeight(ep, false, defaultEpWeight)
			holder = BuildWeightedEpHolder(ep, staticWeight)
			newHolder = append(newHolder, holder)
		}
		allHolder = append(allHolder, holder)
	}
	// only refresh new holders' dynamic weight
	if len(newHolder) != 0 {
		refreshDynamicWeight(newHolder, 15*1000)
	}
	w.weightedEpHolders.Store(allHolder)
	w.notifyWeightChange()
}

func refreshDynamicWeight(holders []*WeightedEpHolder, taskTimeout int64) bool {
	needNotify := atomic.NewBool(false)
	wg := sync.WaitGroup{}
	wg.Add(len(holders))
	for _, h := range holders {
		if h.supportDynamicWeight {
			go func(holder *WeightedEpHolder) {
				defer wg.Done()
				oldWeight := holder.dynamicWeight
				var err error
				dw, err := getEpWeight(holder.getEp(), true, 0)
				if err != nil {
					if errors.Is(err, meta.ServiceNotSupportError) {
						holder.supportDynamicWeight = false
					} else {
						vlog.Warningf("refresh dynamic weight fail! url:%s, error: %s\n", holder.getEp().GetURL().GetIdentity(), err.Error())
					}
					return
				}
				holder.dynamicWeight = dw
				if oldWeight != holder.dynamicWeight {
					needNotify.Store(true)
				}
			}(h)
		} else {
			wg.Done()
		}
	}
	// just wait certain amount of time
	timer := time.NewTimer(time.Millisecond * time.Duration(taskTimeout))
	finishChan := make(chan struct{})
	defer close(finishChan)
	go func() {
		wg.Wait()
		finishChan <- struct{}{}
	}()
	select {
	case <-timer.C:
	case <-finishChan:
		timer.Stop()
	}
	return needNotify.Load()
}

func getEpWeight(ep motan.EndPoint, fromDynamicMeta bool, defaultWeight int64) (int64, error) {
	var metaMap map[string]string
	var err error
	if fromDynamicMeta {
		metaMap, err = meta.GetEpDynamicMeta(ep)
	} else {
		metaMap = meta.GetEpStaticMeta(ep)
	}
	if err != nil {
		return defaultWeight, err
	}
	weightStr := meta.GetMetaValue(metaMap, motan.WeightMetaSuffixKey)
	return adjustWeight(ep, weightStr, defaultWeight), nil
}

func adjustWeight(ep motan.EndPoint, weight string, defaultWeight int64) int64 {
	res := defaultWeight
	if weight != "" {
		temp, err := strconv.ParseInt(weight, 10, 64)
		if err != nil {
			vlog.Warningf("WeightedRefererHolder parse weight fail. %s, use default weight %d, org weight: %s, err: %s\n", ep.GetURL().GetIdentity(), defaultWeight, weight, err.Error())
			return defaultWeight
		}
		if temp < MinEpWeight {
			temp = MinEpWeight
		} else if temp > MaxEpWeight {
			temp = MaxEpWeight
		}
		res = temp
	}
	return res
}

func (w *WeightedEpRefresher) doRefresh(ctx context.Context, refreshPeriod int64) {
	ticker := time.NewTicker(time.Second * time.Duration(refreshPeriod))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.isDestroyed.Store(true)
			return
		case <-ticker.C:
			if w.supportDynamicWeight {
				w.refreshHolderDynamicWeightTask()
			}
		}
	}
}

func (w *WeightedEpRefresher) refreshHolderDynamicWeightTask() {
	// The reference of holders might be changed during the loop
	// Only refresh historical holders
	var tempLoaders []*WeightedEpHolder
	if t := w.weightedEpHolders.Load(); t != nil {
		tempLoaders = t.([]*WeightedEpHolder)
	}
	if refreshDynamicWeight(tempLoaders, 30*1000) {
		w.notifyWeightChange()
	}
}

type WeightedEpHolder struct {
	ep                   motan.EndPoint
	staticWeight         int64
	supportDynamicWeight bool
	dynamicWeight        int64
}

func BuildWeightedEpHolder(ep motan.EndPoint, staticWeight int64) *WeightedEpHolder {
	return &WeightedEpHolder{
		ep:                   ep,
		staticWeight:         staticWeight,
		supportDynamicWeight: ep.GetURL().GetBoolValue(motan.DynamicMetaKey, motan.DefaultDynamicMeta),
	}
}

func (w *WeightedEpHolder) getWeight() int64 {
	if w.dynamicWeight > 0 {
		return w.dynamicWeight
	}
	return w.staticWeight
}

func (w *WeightedEpHolder) getEp() motan.EndPoint {
	return w.ep
}
