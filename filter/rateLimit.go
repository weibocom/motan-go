package filter

import (
	"strconv"
	"strings"

	"github.com/juju/ratelimit"
	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	defaultCapacity    = 5 //todo：初始最大调用次数
	methodConfigPrefix = "rateLimit."
)

type RateLimitFilter struct {
	available     bool
	bucket        *ratelimit.Bucket            //limit each service
	methodBuckets map[string]*ratelimit.Bucket //limit each method
	next          core.EndPointFilter
}

func (r *RateLimitFilter) NewFilter(url *core.URL) core.Filter {
	ret := &RateLimitFilter{}

	//init bucket
	rate, err := strconv.ParseFloat(url.GetParam(RateLimit, ""), 64)
	if err != nil {
		vlog.Errorf("[rateLimit] parse %s config error:%v\n", RateLimit, err)
		return nil
	}

	//init methodBucket
	methodBuckets := make(map[string]*ratelimit.Bucket)
	for key, value := range url.Parameters {
		if temp := strings.Split(key, methodConfigPrefix); len(temp) == 2 {
			if methodRate, err := strconv.ParseFloat(value, 64); err == nil && temp[1] != "" {
				methodBuckets[temp[1]] = ratelimit.NewBucketWithRate(methodRate, defaultCapacity)
			} else {
				vlog.Errorf("[rateLimit] parse %s config error:%s\n", key, err.Error())
				return nil
			}
		}
	}

	ret.bucket = ratelimit.NewBucketWithRate(rate, defaultCapacity)
	ret.methodBuckets = methodBuckets
	ret.available = true
	return ret
}

func (r *RateLimitFilter) Filter(caller core.Caller, request core.Request) core.Response {
	if r.available { //todo: 如果配置错误或没配置，如何处理，现在是不限流
		r.bucket.Wait(1)
		if methodBucket, ok := r.methodBuckets[request.GetMethod()]; ok {
			methodBucket.Wait(1)
		}
	}
	return r.GetNext().Filter(caller, request)
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
	if r.next != nil {
		return true
	}
	return false
}

func (r *RateLimitFilter) GetIndex() int {
	return 3
}

func (r *RateLimitFilter) GetType() int32 {
	return core.EndPointFilterType
}
