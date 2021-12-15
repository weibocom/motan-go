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
		vlog.Infof("[rateLimit] %s %s config success", url.GetIdentity(), RateLimit)
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
		if strings.HasPrefix(key, methodConfigPrefix) {
			if temp := strings.Split(key, methodConfigPrefix); len(temp) == 2 {
				if r, err := strconv.ParseFloat(value, 64); err == nil && temp[1] != "" && r > 0 {
					methodRate[temp[1]] = r
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
		//init config timeout
		if strings.HasPrefix(key, methodTimeoutPrefix) {
			if temp := strings.Split(key, methodTimeoutPrefix); len(temp) == 2 {
				if t, err := strconv.ParseInt(value, 10, 64); err == nil && temp[1] != "" && t > 0 {
					methodTimeout[temp[1]] = time.Millisecond * time.Duration(t)
				} else {
					if err != nil {
						vlog.Warningf("[rateLimit] parse %s timeout config error:%s, just use default timeout: %s", key, err.Error(), defaultTimeout.String())
					} else {
						if t <= 0 {
							vlog.Warningf("[rateLimit] parse %s timeout config error: value is 0 or negative", key)
						}
					}
					if temp[1] == "" {
						vlog.Warningf("[rateLimit] parse %s timeout config error: key is empty", key)
					}

				}
			}
		}
		//init config capacity
		if strings.HasPrefix(key, methodCapacityPrefix) {
			if temp := strings.Split(key, methodCapacityPrefix); len(temp) == 2 {
				if c, err := strconv.ParseInt(value, 10, 64); err == nil && temp[1] != "" && c > 0 {
					methodCapacity[temp[1]] = c
				} else {
					if err != nil {
						vlog.Warningf("[rateLimit] parse %s capacity config error:%s, just use default capacity: %d", key, err.Error(), defaultCapacity)
					} else {
						if c <= 0 {
							vlog.Warningf("[rateLimit] parse %s capacity config error: value is 0 or negative", key)
						}
					}
					if temp[1] == "" {
						vlog.Warningf("[rateLimit] parse %s capacity config error: key is empty", key)
					}

				}
			}
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
	}
	//fill default value for missing method timeout
	for key := range methodBuckets {
		if _, ok := methodTimeout[key]; !ok {
			methodTimeout[key] = defaultTimeout
		}
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
