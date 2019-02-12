package filter

import (
	"fmt"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

type AccessLogFilter struct {
	next motan.EndPointFilter
}

func (t *AccessLogFilter) GetIndex() int {
	return 1
}

func (t *AccessLogFilter) GetName() string {
	return AccessLog
}

func (t *AccessLogFilter) NewFilter(url *motan.URL) motan.Filter {
	return &AccessLogFilter{}
}

func (t *AccessLogFilter) Filter(caller motan.Caller, request motan.Request) motan.Response {
	role := "server"
	var ip string
	switch caller.(type) {
	case motan.Provider:
		role = "server-agent"
		ip = request.GetAttachment(motan.HostKey)
	case motan.EndPoint:
		role = "client-agent"
		ip = caller.GetURL().Host
	}
	start := time.Now()
	response := t.GetNext().Filter(caller, request)
	success := true
	l := 0
	if response.GetValue() != nil {
		if b, ok := response.GetValue().([]byte); ok {
			l = len(b)
		}
		if s, ok := response.GetValue().(string); ok {
			l = len(s)
		}
	}
	if response.GetException() != nil {
		success = false
	}
	writeLog("accessLog--%s:%s,%d,pt:%d,size:%d,req:%s,%s,%s,%d,res:%d,%t,%+v\n", role, ip, caller.GetURL().Port, response.GetProcessTime(), l, request.GetServiceName(), request.GetMethod(), request.GetMethodDesc(), request.GetRequestID(), time.Since(start)/1000000, success, response.GetException())
	return response
}

func (t *AccessLogFilter) HasNext() bool {
	return t.next != nil
}

func (t *AccessLogFilter) SetNext(nextFilter motan.EndPointFilter) {
	t.next = nextFilter
}

func (t *AccessLogFilter) GetNext() motan.EndPointFilter {
	return t.next
}

func (t *AccessLogFilter) GetType() int32 {
	return motan.EndPointFilterType
}

// Temporarily solved the vlog asynchronous output.
const DefaultOutPutChanSize = 1000

var outputChan chan string

func init() {
	outputChan = make(chan string, DefaultOutPutChanSize)
	outputLoop()
}

func outputLoop() {
	go func() {
		for {
			select {
			case logStr := <-outputChan:
				vlog.Infof(logStr)
			}
		}
	}()
}

func writeLog(format string, args ...interface{}) {
	outputChan <- fmt.Sprintf(format, args...)
}
