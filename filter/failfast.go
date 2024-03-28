package filter

import (
	"sync"
	"sync/atomic"
	"time"

	motan "github.com/weibocom/motan-go/core"
)

const (
	failureCount = 10
	nextTestTime = 10 * time.Second
)

type FailfastFilter struct {
	URL     *motan.URL
	next    motan.EndPointFilter
	address string
	eps     *endpointStatus
}

func (e *FailfastFilter) GetRuntimeInfo() map[string]interface{} {
	return GetFilterRuntimeInfo(e)
}

type endpointStatus struct {
	available                 bool
	availMutex                sync.RWMutex
	errorCount                uint32
	succeededOrLastTestedTime int64
}

var epsMap = make(map[string]*endpointStatus)
var epsMutex sync.RWMutex

func (e *FailfastFilter) NewFilter(url *motan.URL) motan.Filter {
	address := url.GetAddressStr()
	epsPointer, _ := getEps(address)
	return &FailfastFilter{URL: url, address: address, eps: epsPointer}
}

func getEps(address string) (*endpointStatus, bool) {
	epsMutex.RLock()
	_, ok := epsMap[address]
	if !ok {
		epsMutex.RUnlock()
		epsMutex.Lock()
		defer epsMutex.Unlock()
		if eps, ok := epsMap[address]; ok {
			return eps, false
		}
		epsMap[address] = &endpointStatus{available: true, succeededOrLastTestedTime: time.Now().UnixNano()}
	} else {
		defer epsMutex.RUnlock()
	}
	return epsMap[address], !ok
}

func (e *FailfastFilter) Filter(caller motan.Caller, request motan.Request) motan.Response {
	response := e.GetNext().Filter(caller, request)
	if response == nil || response.GetException() != nil {
		if count := atomic.AddUint32(&e.eps.errorCount, 1); count >= failureCount {
			e.eps.setAvailable(false)
		}
	} else {
		atomic.StoreUint32(&e.eps.errorCount, 0)
		if available := e.eps.getAvailable(); !available {
			e.eps.setAvailable(true)
		}
	}
	atomic.StoreInt64(&e.eps.succeededOrLastTestedTime, time.Now().UnixNano())
	return response
}

func (e *FailfastFilter) IsAvailable() bool {
	var result bool
	available := e.eps.getAvailable()
	result = available
	if !available {
		succeededOrLastTestedTime := atomic.LoadInt64(&e.eps.succeededOrLastTestedTime)
		if succeededOrLastTestedTime+nextTestTime.Nanoseconds() < time.Now().UnixNano() {
			swapped := atomic.CompareAndSwapInt64(&e.eps.succeededOrLastTestedTime, succeededOrLastTestedTime, time.Now().UnixNano())
			result = swapped
		}
	}

	return result
}

func (e *FailfastFilter) GetIndex() int {
	return 1
}

func (e *FailfastFilter) GetName() string {
	return FailFast
}

func (e *FailfastFilter) HasNext() bool {
	return e.next != nil
}

func (e *FailfastFilter) SetNext(nextFilter motan.EndPointFilter) {
	e.next = nextFilter
}

func (e *FailfastFilter) GetNext() motan.EndPointFilter {
	return e.next
}

func (e *FailfastFilter) GetType() int32 {
	return motan.EndPointFilterType
}

func (e *endpointStatus) setAvailable(value bool) {
	e.availMutex.Lock()
	defer e.availMutex.Unlock()
	e.available = value
}

func (e *endpointStatus) getAvailable() bool {
	e.availMutex.RLock()
	defer e.availMutex.RUnlock()
	return e.available
}
