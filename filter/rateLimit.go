package filter

import (
	"strconv"
	"strings"

	"github.com/juju/ratelimit"
	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	"github.com/weibocom/motan-go/protocol"
)

const (
	defaultCapacity    = 500 //todo：初始可以调用次数
	methodConfigPrefix = "rateLimit."
	applicationWeight  = "appWeight." //todo：权重取值范围是1～9，默认是5，权重越大，调用概率越高
	defaultWeight      = 5            //todo: 默认权重
)

type RateLimitFilter struct {
	available     bool                         //todo: 如果配置错误或没配置，如何处理，现在是不限流
	bucket        *ratelimit.Bucket            //limit each service
	methodBuckets map[string]*ratelimit.Bucket //limit each method
	appWeights    map[string]int64             //todo: 运行中只有读操作，所以不需要锁
	next          core.EndPointFilter
}

var zha = 0
var ray = 0

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
				methodBuckets[temp[1]] = ratelimit.NewBucketWithRate(methodRate*defaultWeight, defaultCapacity)
			} else {
				vlog.Errorf("[rateLimit] parse %s config error:%s\n", key, err.Error())
				return nil
			}
		}
	}

	//init application weight
	appWeights := make(map[string]int64)
	for key, value := range url.Parameters {
		if temp := strings.Split(key, applicationWeight); len(temp) == 2 {
			if weight, err := strconv.ParseInt(value, 0, 64); err == nil && temp[1] != "" {
				appWeights[temp[1]] = 10 - weight
			} else {
				vlog.Errorf("[rateLimit] parse %s config error:%s\n", key, err.Error())
				return nil
			}
		}
	}

	ret.bucket = ratelimit.NewBucketWithRate(rate, defaultCapacity)
	ret.appWeights = appWeights
	ret.methodBuckets = methodBuckets
	ret.available = true
	return ret
}

func (r *RateLimitFilter) Filter(caller core.Caller, request core.Request) core.Response {
	if r.available {
		r.bucket.Wait(1)
		if methodBucket, ok := r.methodBuckets[request.GetMethod()]; ok {
			if app := request.GetAttachment(protocol.MSource); app != "" {
				if appWeight, ok := r.appWeights[app]; ok {
					methodBucket.Wait(appWeight)

					//todo: 测试，次数相同，响应时间有差异>>>>>>>>>>>>>>>
					if app == "zha" {
						zha++
						println("zha:", zha)
					}
					if app == "ray" {
						ray++
						println("ray:", ray)
					}
					//todo: <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<
				}
			}
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
