package filter

import (
	"errors"
	"strconv"

	"github.com/afex/hystrix-go/hystrix"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	RequestVolumeThresholdField = "circuitBreaker.requestThreshold"
	SleepWindowField            = "circuitBreaker.sleepWindow"  //ms
	ErrorPercentThreshold       = "circuitBreaker.errorPercent" //%
)

type CircuitBreakerFilter struct {
	url            *motan.URL
	next           motan.EndPointFilter
	circuitBreaker *hystrix.CircuitBreaker
}

func (c *CircuitBreakerFilter) GetIndex() int {
	return 20
}

func (c *CircuitBreakerFilter) GetName() string {
	return CircuitBreaker
}

func (c *CircuitBreakerFilter) NewFilter(url *motan.URL) motan.Filter {
	circuitBreaker := newCircuitBreaker(c.GetName(), url)
	return &CircuitBreakerFilter{url: url, circuitBreaker: circuitBreaker}
}

func (c *CircuitBreakerFilter) Filter(caller motan.Caller, request motan.Request) motan.Response {
	var response motan.Response
	err := hystrix.Do(c.url.GetIdentity(), func() error {
		response = c.GetNext().Filter(caller, request)
		if ex := response.GetException(); ex != nil {
			return errors.New(ex.ErrMsg)
		}
		return nil
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

func newCircuitBreaker(filterName string, url *motan.URL) *hystrix.CircuitBreaker {
	commandConfig := buildCommandConfig(filterName, url)
	hystrix.ConfigureCommand(url.GetIdentity(), *commandConfig)
	circuitBreaker, _, _ := hystrix.GetCircuit(url.GetIdentity())
	vlog.Infof("[%s] new circuit success. url:%v, config{%s}\n", filterName, url.GetIdentity(), getConfigStr(commandConfig))
	return circuitBreaker
}

func buildCommandConfig(filterName string, url *motan.URL) *hystrix.CommandConfig {
	hystrix.DefaultMaxConcurrent = 1000
	hystrix.DefaultTimeout = int(url.GetPositiveIntValue(motan.TimeOutKey, int64(hystrix.DefaultTimeout))) * 2
	commandConfig := &hystrix.CommandConfig{}
	if v, ok := url.Parameters[RequestVolumeThresholdField]; ok {
		if temp, _ := strconv.Atoi(v); temp > 0 {
			commandConfig.RequestVolumeThreshold = temp
		} else {
			vlog.Warningf("[%s] parse config %s error, use default", filterName, RequestVolumeThresholdField)
		}
	}
	if v, ok := url.Parameters[SleepWindowField]; ok {
		if temp, _ := strconv.Atoi(v); temp > 0 {
			commandConfig.SleepWindow = temp
		} else {
			vlog.Warningf("[%s] parse config %s error, use default", filterName, SleepWindowField)
		}
	}
	if v, ok := url.Parameters[ErrorPercentThreshold]; ok {
		if temp, _ := strconv.Atoi(v); temp > 0 && temp <= 100 {
			commandConfig.ErrorPercentThreshold = temp
		} else {
			vlog.Warningf("[%s] parse config %s error, use default", filterName, ErrorPercentThreshold)
		}
	}
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
	if config.RequestVolumeThreshold != 0 {
		ret += "requestThreshold:" + strconv.Itoa(config.RequestVolumeThreshold) + " "
	} else {
		ret += "requestThreshold:" + strconv.Itoa(hystrix.DefaultVolumeThreshold) + " "
	}
	if config.SleepWindow != 0 {
		ret += "sleepWindow:" + strconv.Itoa(config.SleepWindow) + " "
	} else {
		ret += "sleepWindow:" + strconv.Itoa(hystrix.DefaultSleepWindow) + " "
	}
	if config.ErrorPercentThreshold != 0 {
		ret += "errorPercent:" + strconv.Itoa(config.ErrorPercentThreshold) + " "
	} else {
		ret += "errorPercent:" + strconv.Itoa(hystrix.DefaultErrorPercentThreshold) + " "
	}
	return ret
}
