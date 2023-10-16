package filter

import (
	"encoding/json"
	"strconv"
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
	address := ip + ":" + caller.GetURL().GetPortStr()
	if _, ok := caller.(motan.Provider); ok {
		reqCtx := request.GetRPCContext(true)
		resCtx := response.GetRPCContext(true)
		resCtx.AddFinishHandler(motan.FinishHandleFunc(func() {
			totalTime := reqCtx.ResponseSendTime.Sub(reqCtx.RequestReceiveTime).Nanoseconds() / 1e6
			doAccessLog(t.GetName(), role, address, totalTime, request, response)
		}))
	} else {
		doAccessLog(t.GetName(), role, address, time.Now().Sub(start).Nanoseconds()/1e6, request, response)
	}
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

func doAccessLog(filterName string, role string, address string, totalTime int64, request motan.Request, response motan.Response) {
	exception := response.GetException()
	reqCtx := request.GetRPCContext(true)
	resCtx := response.GetRPCContext(true)
	// response code should be same as upstream
	responseCode := ""
	metaUpstreamCode := resCtx.Meta.LoadOrEmpty(motan.MetaUpstreamCode)
	if resCtx.Meta != nil {
		responseCode = resCtx.Meta.LoadOrEmpty(motan.MetaUpstreamCode)
	}
	var exceptionData []byte
	if exception != nil {
		exceptionData, _ = json.Marshal(exception)
		responseCode = strconv.Itoa(exception.ErrCode)
	} else {
		// default success response code use http ok status code
		if responseCode == "" {
			responseCode = "200"
		}
	}
	vlog.AccessLog(&vlog.AccessLogEntity{
		FilterName:    filterName,
		Role:          role,
		RequestID:     response.GetRequestID(),
		Service:       request.GetServiceName(),
		Method:        request.GetMethod(),
		RemoteAddress: address,
		Desc:          request.GetMethodDesc(),
		ReqSize:       reqCtx.BodySize,
		ResSize:       resCtx.BodySize,
		BizTime:       response.GetProcessTime(), //ms
		TotalTime:     totalTime,                 //ms
		ResponseCode:  responseCode,
		Success:       exception == nil,
		Exception:     string(exceptionData),
		UpstreamCode:  metaUpstreamCode,
	})
}
