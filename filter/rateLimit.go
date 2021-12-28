package filter

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/ratelimit"
	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	defaultCapacity      int64 = 1000
	defaultTimeout             = 1000 * time.Millisecond
	timeoutKey                 = "timeout"
	capacity                   = "capacity"
	methodConfigPrefix         = "rateLimit."
	methodCapacityPrefix       = "capacity."
	methodTimeoutPrefix        = "timeout."
)

type RateLimitFilter struct {
	switcher           *core.Switcher
	serviceBucket      *ratelimit.Bucket //limit service
	serviceMaxDuration time.Duration
	methodBuckets      map[string]*ratelimit.Bucket //limit method
	methodMaxDurations map[string]time.Duration
	next               core.EndPointFilter
}

func (r *RateLimitFilter) NewFilter(url *core.URL) core.Filter {
	ret := &RateLimitFilter{}
	//init bucket
	rlp := url.GetParam(RateLimit, "")
	if rate, err := strconv.ParseFloat(rlp, 64); err == nil {
		capacity := url.GetPositiveIntValue(capacity, defaultCapacity)
		if rate <= 0 || rate > float64(capacity) {
			vlog.Warningf("[rateLimit] %s: service rateLimit config invalid, rate: %f, capacity:%d", url.GetIdentity(), rate, capacity)
		}
		ret.serviceBucket = ratelimit.NewBucketWithRate(rate, capacity)
		ret.serviceMaxDuration = url.GetTimeDuration(timeoutKey, time.Millisecond, defaultTimeout)
		vlog.Infof("[rateLimit] %s %s service config success, rate:%f, capacity:%d, wait max duration:%s", url.GetIdentity(), RateLimit, rate, capacity, ret.serviceMaxDuration.String())
	} else {
		if rlp != "" {
			vlog.Warningf("[rateLimit] parse %s config error:%v", RateLimit, err)
		}

	}
	//read config
	methodBuckets := make(map[string]*ratelimit.Bucket)
	methodTimeout := make(map[string]time.Duration)
	methodCapacity := make(map[string]int64)
	methodRate := make(map[string]float64)
	for key, value := range url.Parameters {
		if k, v, ok := getKeyValue(key, value, methodConfigPrefix); ok {
			methodRate[k] = v
		}
		if k, v, ok := getKeyValue(key, value, methodTimeoutPrefix); ok {
			methodTimeout[k] = time.Millisecond * time.Duration(v)
		}
		if k, v, ok := getKeyValue(key, value, methodCapacityPrefix); ok {
			methodCapacity[k] = int64(v)
		}
	}
	//init methodBucket
	for key, value := range methodRate {
		capacity := defaultCapacity
		if c, ok := methodCapacity[key]; ok {
			capacity = c
		}
		if value > float64(capacity) {
			vlog.Warningf("[rateLimit] method %s init failed: config is invalid, rate should less than capacity, rate: %f, capacity:%d", key, value, capacity)
			continue
		}
		methodBuckets[key] = ratelimit.NewBucketWithRate(value, capacity)
		//fill default value for missing timeout
		if _, ok := methodTimeout[key]; !ok {
			methodTimeout[key] = defaultTimeout
		}
		vlog.Infof("[rateLimit] %s %s method: %s config success, rate:%f, capacity:%d, wait max duration:%s", url.GetIdentity(), RateLimit, key, value, capacity, methodTimeout[key].String())
	}

	if len(methodBuckets) == 0 && ret.serviceBucket == nil {
		vlog.Warningf("[rateLimit] %s: no service or method rateLimit takes effect", url.GetIdentity())
	}
	ret.methodBuckets = methodBuckets
	ret.methodMaxDurations = methodTimeout
	//init switcher
	switcherName := GetRateLimitSwitcherName(url)
	core.GetSwitcherManager().Register(switcherName, true)
	ret.switcher = core.GetSwitcherManager().GetSwitcher(switcherName)

	return ret
}

func getKeyValue(key, value, prefix string) (string, float64, bool) {
	if strings.HasPrefix(key, prefix) {
		if temp := strings.Split(key, prefix); len(temp) == 2 {
			if r, err := strconv.ParseFloat(value, 64); err == nil && temp[1] != "" && r > 0 {
				return temp[1], r, true
			} else {
				if err != nil {
					vlog.Warningf("[rateLimit] parse %s config error:%s", key, err.Error())
				} else {
					if r <= 0 {
						vlog.Warningf("[rateLimit] parse %s config error: value is 0 or negative", key)
					}
				}
				if temp[1] == "" {
					vlog.Warningf("[rateLimit] parse %s config error: key is empty", key)
				}
			}
		}
	}
	return "", 0, false
}

func (r *RateLimitFilter) Filter(caller core.Caller, request core.Request) core.Response {
	if r.switcher.IsOpen() {
		if r.serviceBucket != nil {
			callable := r.serviceBucket.WaitMaxDuration(1, r.serviceMaxDuration)
			if !callable {
				return defaultErrMotanResponse(request, fmt.Sprintf("[rateLimit] wait time exceed timeout(%s)", r.serviceMaxDuration.String()))
			}
		}
		if methodBucket, ok := r.methodBuckets[request.GetMethod()]; ok {
			methodTimeout := r.methodMaxDurations[request.GetMethod()]
			callable := methodBucket.WaitMaxDuration(1, methodTimeout)
			if !callable {
				return defaultErrMotanResponse(request, fmt.Sprintf("[rateLimit] method %s wait time exceed timeout(%s)", request.GetMethod(), methodTimeout.String()))
			}
		}
	}
	return r.GetNext().Filter(caller, request)
}

func GetRateLimitSwitcherName(url *core.URL) string {
	return url.GetParam("conf-id", "") + "_rateLimit"
}

func (r *RateLimitFilter) SetNext(nextFilter core.EndPointFilter) {
	r.next = nextFilter
}

func (r *RateLimitFilter) GetNext() core.EndPointFilter {
	return r.next
}

func (r *RateLimitFilter) GetName() string {
	return RateLimit
}

func (r *RateLimitFilter) HasNext() bool {
	return r.next != nil
}

func (r *RateLimitFilter) GetIndex() int {
	return 3
}

func (r *RateLimitFilter) GetType() int32 {
	return core.EndPointFilterType
}
