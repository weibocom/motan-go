package filter

import (
	"strconv"
	"strings"

	"github.com/juju/ratelimit"
	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	defaultCapacity    = 1000
	methodConfigPrefix = "rateLimit."
)

type RateLimitFilter struct {
	switcher      *core.Switcher
	bucket        *ratelimit.Bucket            //limit service
	methodBuckets map[string]*ratelimit.Bucket //limit method
	next          core.EndPointFilter
}

func (r *RateLimitFilter) NewFilter(url *core.URL) core.Filter {
	ret := &RateLimitFilter{}

	//init bucket
	if rate, err := strconv.ParseFloat(url.GetParam(RateLimit, ""), 64); err == nil {
		ret.bucket = ratelimit.NewBucketWithRate(rate, defaultCapacity)
	} else {
		vlog.Warningf("[rateLimit] parse %s config error:%v", RateLimit, err)
	}

	//init methodBucket
	methodBuckets := make(map[string]*ratelimit.Bucket)
	for key, value := range url.Parameters {
		if temp := strings.Split(key, methodConfigPrefix); len(temp) == 2 {
			if methodRate, err := strconv.ParseFloat(value, 64); err == nil && temp[1] != "" {
				methodBuckets[temp[1]] = ratelimit.NewBucketWithRate(methodRate, defaultCapacity)
			} else {
				vlog.Warningf("[rateLimit] parse %s config error:%s", key, err.Error())
			}
		} else {
			vlog.Warningf("[rateLimit] parse %s config error", key)
		}
	}
	ret.methodBuckets = methodBuckets

	//init switcher
	switcherName := GetRateLimitSwitcherName(url)
	core.GetSwitcherManager().Register(switcherName, true)
	ret.switcher = core.GetSwitcherManager().GetSwitcher(switcherName)

	return ret
}

func (r *RateLimitFilter) Filter(caller core.Caller, request core.Request) core.Response {
	if r.switcher.IsOpen() {
		if r.bucket != nil {
			r.bucket.Wait(1)
		}
		if methodBucket, ok := r.methodBuckets[request.GetMethod()]; ok {
			methodBucket.Wait(1)
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
