package filter

import (
	"strconv"

	"github.com/afex/hystrix-go/hystrix"
	"github.com/pkg/errors"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	CircuitBreakerEnable        = "circuitBreakerEnable"
	CircuitBreakerTimeoutField  = "circuitBreakerTimeout"
	MaxConcurrentRequestsField  = "maxConcurrentRequests"
	RequestVolumeThresholdField = "requestVolumeThreshold"
	SleepWindowField            = "sleepWindow"
	ErrorPercentThreshold       = "errorPercentThreshold"
)

var (
	errExecuteFailure = errors.New("grpc: invoke error")
)

type CircuitBreakerEndPointFilter struct {
	URL                  *motan.URL
	next                 motan.EndPointFilter
	circuitBreakerEnable bool
	circuitBreaker       *hystrix.CircuitBreaker
}

func (t *CircuitBreakerEndPointFilter) GetIndex() int {
	return 2
}

func (t *CircuitBreakerEndPointFilter) GetName() string {
	return "circuitbreaker"
}

func (t *CircuitBreakerEndPointFilter) NewFilter(url *motan.URL) motan.Filter {
	circuitBreakerEnable, commandConfig := buildComandConfig(url)
	var circuitBreaker *hystrix.CircuitBreaker
	if circuitBreakerEnable {
		hystrix.ConfigureCommand(url.GetIdentity(), *commandConfig)
		var err error
		circuitBreaker, _, err = hystrix.GetCircuit(url.GetIdentity())
		if err != nil {
			circuitBreakerEnable = false
			vlog.Errorf("CircuitBreaker not available! err %s\n", err.Error())
		} else {
			vlog.Infof("CircuitBreaker: %v = %+v \n", url.GetIdentity(), commandConfig)
		}
	}
	return &CircuitBreakerEndPointFilter{URL: url, circuitBreakerEnable: circuitBreakerEnable, circuitBreaker: circuitBreaker}
}

func (t *CircuitBreakerEndPointFilter) Filter(caller motan.Caller, request motan.Request) motan.Response {
	var response motan.Response
	if t.circuitBreakerEnable {
		hystrix.Do(t.URL.GetIdentity(), func() error {
			response = t.GetNext().Filter(caller, request)
			if response.GetException() != nil {
				return errExecuteFailure
			}
			return nil
		}, func(err error) error {
			response = &motan.MotanResponse{
				RequestID:   request.GetRequestID(),
				Attachment:  motan.NewConcurrentStringMap(),
				ProcessTime: 0,
				// todo exception 后续统一规划
				Exception: &motan.Exception{ErrCode: 400, ErrMsg: err.Error(), ErrType: motan.ServiceException},
				Value:     make([]byte, 0)}
			return err
		})
	} else {
		response = t.GetNext().Filter(caller, request)
	}
	return response
}

func (t *CircuitBreakerEndPointFilter) HasNext() bool {
	return t.next != nil
}

func (t *CircuitBreakerEndPointFilter) SetNext(nextFilter motan.EndPointFilter) {
	t.next = nextFilter
}

func (t *CircuitBreakerEndPointFilter) GetNext() motan.EndPointFilter {
	return t.next
}

func (t *CircuitBreakerEndPointFilter) GetType() int32 {
	return motan.EndPointFilterType
}

func buildComandConfig(url *motan.URL) (bool, *hystrix.CommandConfig) {
	var circuitBreakerEnable bool
	var commandConfig *hystrix.CommandConfig = &hystrix.CommandConfig{}
	if v, ok := url.Parameters[CircuitBreakerEnable]; ok {
		circuitBreakerEnable, _ = strconv.ParseBool(v)
	}
	if v, ok := url.Parameters[CircuitBreakerTimeoutField]; ok {
		commandConfig.Timeout, _ = strconv.Atoi(v)
	}
	if v, ok := url.Parameters[MaxConcurrentRequestsField]; ok {
		commandConfig.MaxConcurrentRequests, _ = strconv.Atoi(v)
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
	return circuitBreakerEnable, commandConfig
}
