package filter

import (
	"fmt"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	defaultRole     = "server"
	clientAgentRole = "client-agent"
	serverAgentRole = "server-agent"
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
	role := defaultRole
	var ip string
	switch caller.(type) {
	case motan.Provider:
		role = serverAgentRole
		ip = request.GetAttachment(motan.HostKey)
	case motan.EndPoint:
		role = clientAgentRole
		ip = caller.GetURL().Host
	}
	start := time.Now()
	response := t.GetNext().Filter(caller, request)
	doAccessLog(role, ip+":"+caller.GetURL().GetPortStr(), t.GetName(), start, request, response)
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

func doAccessLog(role string, address string, filterName string, start time.Time, request motan.Request, response motan.Response) {
	writeLog("[%s] %s,%s,pt:%d,size:%d/%d,req:%s,%s,%s,%d,res:%d,%t,%+v\n",
		filterName, role, address, response.GetProcessTime(),
		request.GetRPCContext(true).BodySize, response.GetRPCContext(true).BodySize,
		request.GetServiceName(), request.GetMethod(), request.GetMethodDesc(), response.GetRequestID(),
		time.Since(start)/1000000, response.GetException() == nil, response.GetException())
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
