package filter

import (
	"errors"
	"strconv"

	"github.com/afex/hystrix-go/hystrix"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	CircuitBreakerTimeoutField  = "circuitBreaker.timeout" //ms
	RequestVolumeThresholdField = "circuitBreaker.requestVolume"
	SleepWindowField            = "circuitBreaker.sleepWindow" //ms
	ErrorPercentThreshold       = "circuitBreaker.errorPercent"
)

type CircuitBreakerFilter struct {
	url            *motan.URL
	next           motan.EndPointFilter
	available      bool
	circuitBreaker *hystrix.CircuitBreaker
}

func (c *CircuitBreakerFilter) GetIndex() int {
	return 20
}

func (c *CircuitBreakerFilter) GetName() string {
	return CircuitBreaker
}

func (c *CircuitBreakerFilter) NewFilter(url *motan.URL) motan.Filter {
	available, circuitBreaker := newCircuitBreaker(url)
	return &CircuitBreakerFilter{url: url, available: available, circuitBreaker: circuitBreaker}
}

func (c *CircuitBreakerFilter) Filter(caller motan.Caller, request motan.Request) motan.Response {
	var response motan.Response
	if c.available {
		hystrix.Do(c.url.GetIdentity(), func() error {
			response = c.GetNext().Filter(caller, request)
			if ex := response.GetException(); ex != nil {
				return errors.New(ex.ErrMsg)
			}
			return nil
		}, func(err error) error {
			if response == nil {
				response = defaultErrMotanResponse(request, err.Error())
			}
			return err
		})
	} else {
		response = c.GetNext().Filter(caller, request)
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

func newCircuitBreaker(url *motan.URL) (bool, *hystrix.CircuitBreaker) {
	available := true
	commandConfig := buildCommandConfig(url)
	hystrix.ConfigureCommand(url.GetIdentity(), *commandConfig)
	circuitBreaker, _, err := hystrix.GetCircuit(url.GetIdentity())
	if err != nil {
		available = false
		vlog.Errorf("[circuitBreaker] New circuit error: %s\n", err.Error())
	} else {
		vlog.Infof("[circuitBreaker] New circuit success. url:%v, config{%s}\n", url.GetIdentity(), getConfigStr(commandConfig))
	}
	return available, circuitBreaker
}

func buildCommandConfig(url *motan.URL) *hystrix.CommandConfig {
	hystrix.DefaultMaxConcurrent = 1000
	commandConfig := &hystrix.CommandConfig{}
	if v, ok := url.Parameters[CircuitBreakerTimeoutField]; ok {
		commandConfig.Timeout, _ = strconv.Atoi(v)
	}
	if v, ok := url.Parameters[RequestVolumeThresholdField]; ok {
		commandConfig.RequestVolumeThreshold, _ = strconv.Atoi(v)
	}
	if v, ok := url.Parameters[SleepWindowField]; ok {
		commandConfig.SleepWindow, _ = strconv.Atoi(v)
	}
	if v, ok := url.Parameters[ErrorPercentThreshold]; ok {
		commandConfig.ErrorPercentThreshold, _ = strconv.Atoi(v)
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
		Value: make([]byte, 0),
	}
	return response
}

func getConfigStr(config *hystrix.CommandConfig) string {
	var ret string
	if config.Timeout != 0 {
		ret += "timeout:" + strconv.Itoa(config.Timeout) + " "
	}
	if config.RequestVolumeThreshold != 0 {
		ret += "requestVolume:" + strconv.Itoa(config.RequestVolumeThreshold) + " "
	}
	if config.SleepWindow != 0 {
		ret += "sleepWindow:" + strconv.Itoa(config.SleepWindow) + " "
	}
	if config.ErrorPercentThreshold != 0 {
		ret += "errorPercent:" + strconv.Itoa(config.ErrorPercentThreshold) + " "
	}
	return ret
}
