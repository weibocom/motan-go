package filter

import (
	"encoding/json"
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
	doAccessLog(t.GetName(), role, ip+":"+caller.GetURL().GetPortStr(), start, request, response)
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

func doAccessLog(filterName string, role string, address string, start time.Time, request motan.Request, response motan.Response) {
	exception := response.GetException()
	var exceptionData []byte
	if exception != nil {
		exceptionData, _ = json.Marshal(exception)
	}
	vlog.AccessLog(&vlog.AccessLogEntity{
		FilterName:    filterName,
		Role:          role,
		RequestID:     response.GetRequestID(),
		Service:       request.GetServiceName(),
		Method:        request.GetMethod(),
		RemoteAddress: address,
		Desc:          request.GetMethodDesc(),
		ReqSize:       request.GetRPCContext(true).BodySize,
		ResSize:       response.GetRPCContext(true).BodySize,
		BizTime:       response.GetProcessTime(),             //ms
		TotalTime:     time.Since(start).Nanoseconds() / 1e6, //ms
		Success:       exception == nil,
		Exception:     string(exceptionData)})
}
