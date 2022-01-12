package filter

import (
	"fmt"
	mpro "github.com/weibocom/motan-go/protocol"
	"strconv"
	"strings"
	"time"

	"github.com/juju/ratelimit"
	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	capacityKey        = "capacity"
	methodConfigPrefix = "rateLimit."
	defaultTimeout     = 1000
	maxCapacity        = 5000
	minCapacity        = 1000
)

type RateLimitFilter struct {
	switcher           *core.Switcher
	serviceBucket      *ratelimit.Bucket //limit service
	serviceMaxDuration time.Duration
	methodBuckets      map[string]*ratelimit.Bucket //limit method
	next               core.EndPointFilter
	url                *core.URL
}

func (r *RateLimitFilter) NewFilter(url *core.URL) core.Filter {
	ret := &RateLimitFilter{}
	//init bucket
	rlp := url.GetParam(RateLimit, "")
	if rlp == "" {
		vlog.Warningf("[rateLimit] limit rate config is empty, service %s initialize failed", RateLimit)
	} else {
		if rate, err := strconv.ParseFloat(rlp, 64); err == nil {
			if rate <= 0 {
				vlog.Warningf("[rateLimit] %s: service rateLimit config invalid, rate: %f", url.GetIdentity(), rate)
			} else {
				capacity := getCapacity(url, rate)
				ret.serviceBucket = ratelimit.NewBucketWithRate(rate, capacity)
				ret.serviceMaxDuration = url.GetTimeDuration(core.TimeOutKey, time.Millisecond, time.Duration(defaultTimeout)*time.Millisecond)
				vlog.Infof("[rateLimit] %s %s service config success, rate:%f, capacity:%d, wait max duration:%s", url.GetIdentity(), RateLimit, rate, capacity, ret.serviceMaxDuration.String())
			}
		} else {
			vlog.Warningf("[rateLimit] parse %s config error:%v", RateLimit, err)
		}
	}

	//read config
	methodBuckets := make(map[string]*ratelimit.Bucket)
	methodRate := make(map[string]float64)
	for key, value := range url.Parameters {
		if k, v, ok := getKeyValue(key, value, methodConfigPrefix); ok {
			methodRate[k] = v
		}
	}
	//init methodBucket
	for key, value := range methodRate {
		capacity := getMethodCapacity(url, value, key)
		methodBuckets[key] = ratelimit.NewBucketWithRate(value, capacity)
		vlog.Infof("[rateLimit] %s %s method: %s config success, rate:%f, capacity:%d", url.GetIdentity(), RateLimit, key, value, capacity)
	}

	if len(methodBuckets) == 0 && ret.serviceBucket == nil {
		vlog.Warningf("[rateLimit] %s: no service or method rateLimit takes effect", url.GetIdentity())
	}
	ret.methodBuckets = methodBuckets
	//init switcher
	switcherName := GetRateLimitSwitcherName(url)
	core.GetSwitcherManager().Register(switcherName, true)
	ret.switcher = core.GetSwitcherManager().GetSwitcher(switcherName)
	ret.url = url
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
			var methodTimeout time.Duration
			tRequest := request.GetAttachment(mpro.MTimeout)
			if tRequest != "" {
				if v, err := strconv.ParseInt(tRequest, 10, 64); err == nil {
					methodTimeout = time.Duration(v) * time.Millisecond
				}
			} else {
				v := r.url.GetMethodPositiveIntValue(request.GetMethod(), request.GetMethodDesc(), core.TimeOutKey, defaultTimeout)
				methodTimeout = time.Duration(v) * time.Millisecond
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
	return url.GetParam(core.URLConfKey, "") + "_rateLimit"
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

func getCapacity(url *core.URL, rate float64) int64 {
	return url.GetPositiveIntValue(capacityKey, getDefaultCapacity(rate))
}

func getMethodCapacity(url *core.URL, rate float64, methodName string) int64 {
	return url.GetMethodPositiveIntValue(methodName, "", capacityKey, getDefaultCapacity(rate))
}

// dynamically get
func getDefaultCapacity(rate float64) (res int64) {
	capicity := rate * 2
	if capicity >= minCapacity && capicity <= maxCapacity {
		res = int64(capicity)
		return
	}
	if capicity < minCapacity {
		res = minCapacity
		return
	}
	res = maxCapacity
	return
}
