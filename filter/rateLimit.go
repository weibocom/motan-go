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
	Capacity                   = "capacity"
	methodConfigPrefix         = "rateLimit."
	methodCapacityPrefix       = "capacity."
	methodTimeoutPrefix        = "timeout."
)

type RateLimitFilter struct {
	switcher      *core.Switcher
	bucket        *ratelimit.Bucket //limit service
	timeout       time.Duration
	methodBuckets map[string]*ratelimit.Bucket //limit method
	methodTimeout map[string]time.Duration
	next          core.EndPointFilter
}

func (r *RateLimitFilter) NewFilter(url *core.URL) core.Filter {
	ret := &RateLimitFilter{}
	//init bucket
	rlp := url.GetParam(RateLimit, "")
	if rate, err := strconv.ParseFloat(rlp, 64); err == nil {
		capacity := url.GetIntValue(Capacity, defaultCapacity)
		ret.bucket = ratelimit.NewBucketWithRate(rate, capacity)
		ret.timeout = url.GetTimeDuration(timeoutKey, time.Millisecond, defaultTimeout)
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
				if r, err := strconv.ParseFloat(value, 64); err == nil && temp[1] != "" {
					methodRate[temp[1]] = r
				} else {
					if err != nil {
						vlog.Warningf("[rateLimit] parse %s config error:%s", key, err.Error())
					} else {
						vlog.Warningf("[rateLimit] parse %s config error: key is empty", key)
					}

				}
			}
		}
		//init config timeout
		if strings.HasPrefix(key, methodTimeoutPrefix) {
			if temp := strings.Split(key, methodTimeoutPrefix); len(temp) == 2 {
				if t, err := strconv.ParseInt(value, 10, 64); err == nil && temp[1] != "" {
					methodTimeout[temp[1]] = time.Millisecond * time.Duration(t)
				} else {
					if err != nil {
						vlog.Warningf("[rateLimit] parse %s timeout config error:%s, just use default timeout", key, err.Error())
					} else {
						vlog.Warningf("[rateLimit] parse %s timeout config error: key is empty", key)
					}

				}
			}
		}
		//init config capacity
		if strings.HasPrefix(key, methodCapacityPrefix) {
			if temp := strings.Split(key, methodCapacityPrefix); len(temp) == 2 {
				if c, err := strconv.ParseInt(value, 10, 64); err == nil && temp[1] != "" {
					methodCapacity[temp[1]] = c
				} else {
					if err != nil {
						vlog.Warningf("[rateLimit] parse %s capacity config error:%s, just use default capacity", key, err.Error())
					} else {
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
		methodBuckets[key] = ratelimit.NewBucketWithRate(value, capacity)
	}
	ret.methodBuckets = methodBuckets
	ret.methodTimeout = methodTimeout
	//init switcher
	switcherName := GetRateLimitSwitcherName(url)
	core.GetSwitcherManager().Register(switcherName, true)
	ret.switcher = core.GetSwitcherManager().GetSwitcher(switcherName)

	return ret
}

func (r *RateLimitFilter) Filter(caller core.Caller, request core.Request) core.Response {
	if r.switcher.IsOpen() {
		if r.bucket != nil {
			callable := r.bucket.WaitMaxDuration(1, r.timeout)
			if !callable {
				return defaultErrMotanResponse(request, fmt.Sprintf("[rateLimit] wait time exceed timeout(%s)", r.timeout.String()))
			}
		}
		if methodBucket, ok := r.methodBuckets[request.GetMethod()]; ok {
			methodTimeout := defaultTimeout
			if t, ok := r.methodTimeout[request.GetMethod()]; ok {
				methodTimeout = t
			}
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
