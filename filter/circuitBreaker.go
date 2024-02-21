package filter

import (
	"errors"
	"github.com/afex/hystrix-go/hystrix"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	"strconv"
)

const (
	RequestVolumeThresholdField = "circuitBreaker.requestThreshold"
	SleepWindowField            = "circuitBreaker.sleepWindow"  //ms
	ErrorPercentThreshold       = "circuitBreaker.errorPercent" //%
	MaxConcurrentField          = "circuitBreaker.maxConcurrent"
	IncludeBizException         = "circuitBreaker.bizException"
	defaultMaxConcurrent        = 5000
)

type CircuitBreakerFilter struct {
	url                 *motan.URL
	next                motan.EndPointFilter
	circuitBreaker      *hystrix.CircuitBreaker
	includeBizException bool
}

func (c *CircuitBreakerFilter) GetIndex() int {
	return 20
}

func (c *CircuitBreakerFilter) GetName() string {
	return CircuitBreaker
}

func (c *CircuitBreakerFilter) NewFilter(url *motan.URL) motan.Filter {
	bizException := newCircuitBreaker(c.GetName(), url)
	return &CircuitBreakerFilter{url: url, includeBizException: bizException}
}

func (c *CircuitBreakerFilter) Filter(caller motan.Caller, request motan.Request) motan.Response {
	var response motan.Response
	err := hystrix.Do(c.url.GetIdentity(), func() error {
		response = c.GetNext().Filter(caller, request)
		return checkException(response, c.includeBizException)
	}, nil)
	if err != nil {
		return defaultErrMotanResponse(request, err.Error())
	}
	return response
}

func (c *CircuitBreakerFilter) HasNext() bool {
	return c.next != nil
}

func (c *CircuitBreakerFilter) SetNext(nextFilter motan.EndPointFilter) {
	c.next = nextFilter
}

func (c *CircuitBreakerFilter) GetNext() motan.EndPointFilter {
	return c.next
}

func (c *CircuitBreakerFilter) GetType() int32 {
	return motan.EndPointFilterType
}

func newCircuitBreaker(filterName string, url *motan.URL) bool {
	bizExceptionStr := url.GetParam(IncludeBizException, "true")
	bizException, err := strconv.ParseBool(bizExceptionStr)
	if err != nil {
		bizException = true
		vlog.Warningf("[%s] parse config %s error, use default: true", filterName, IncludeBizException)
	}
	commandConfig := buildCommandConfig(url)
	hystrix.ConfigureCommand(url.GetIdentity(), *commandConfig)
	if _, _, err = hystrix.GetCircuit(url.GetIdentity()); err != nil {
		vlog.Errorf("[%s] new circuit fail. err:%s, url:%v, config{%s}", err.Error(), filterName, url.GetIdentity(), getConfigStr(commandConfig)+"bizException:"+bizExceptionStr)
	} else {
		vlog.Infof("[%s] new circuit success. url:%v, config{%s}", filterName, url.GetIdentity(), getConfigStr(commandConfig)+"bizException:"+bizExceptionStr)
	}
	return bizException
}

func getConfigValue(url *motan.URL, key string, defaultValue int) int {
	if v, ok := url.Parameters[key]; ok {
		if temp, _ := strconv.Atoi(v); temp > 0 {
			if key == ErrorPercentThreshold {
				if temp <= 100 {
					return temp
				}
			} else {
				return temp
			}
		}
	}
	vlog.Warningf("[%s] parse config %s error, use default: %d", CircuitBreaker, key, defaultValue)
	return defaultValue
}

func buildCommandConfig(url *motan.URL) *hystrix.CommandConfig {
	commandConfig := &hystrix.CommandConfig{}
	commandConfig.RequestVolumeThreshold = getConfigValue(url, RequestVolumeThresholdField, hystrix.DefaultVolumeThreshold)
	commandConfig.SleepWindow = getConfigValue(url, SleepWindowField, hystrix.DefaultSleepWindow)
	commandConfig.ErrorPercentThreshold = getConfigValue(url, ErrorPercentThreshold, hystrix.DefaultErrorPercentThreshold)
	commandConfig.MaxConcurrentRequests = getConfigValue(url, MaxConcurrentField, defaultMaxConcurrent)
	commandConfig.Timeout = getConfigValue(url, motan.TimeOutKey, hystrix.DefaultTimeout)
	return commandConfig
}

func defaultErrMotanResponse(request motan.Request, errMsg string) motan.Response {
	response := &motan.MotanResponse{
		RequestID:   request.GetRequestID(),
		Attachment:  motan.NewStringMap(motan.DefaultAttachmentSize),
		ProcessTime: 0,
		Exception: &motan.Exception{
			ErrCode: 400,
			ErrMsg:  errMsg,
			ErrType: motan.ServiceException},
	}
	return response
}

func getConfigStr(config *hystrix.CommandConfig) string {
	var ret string
	ret += "requestThreshold:" + strconv.Itoa(config.RequestVolumeThreshold) + " "
	ret += "sleepWindow:" + strconv.Itoa(config.SleepWindow) + " "
	ret += "errorPercent:" + strconv.Itoa(config.ErrorPercentThreshold) + " "
	ret += "maxConcurrent:" + strconv.Itoa(config.MaxConcurrentRequests) + " "
	ret += "timeout:" + strconv.Itoa(config.Timeout) + "ms "
	return ret
}

func checkException(response motan.Response, includeBizException bool) error {
	if ex := response.GetException(); ex != nil && (includeBizException || ex.ErrType != motan.BizException) {
		return errors.New(ex.ErrMsg)
	}
	return nil
}
