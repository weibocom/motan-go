package filter

import (
	"encoding/json"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	"strconv"
	"time"
)

const (
	defaultRole     = "server"
	clientAgentRole = "client-agent"
	serverAgentRole = "server-agent"
)

type AccessLogFilter struct {
	next motan.EndPointFilter
}

func (t *AccessLogFilter) GetRuntimeInfo() map[string]interface{} {
	return GetFilterRuntimeInfo(t)
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
	var start time.Time
	switch caller.(type) {
	case motan.Provider:
		role = serverAgentRole
		ip = request.GetAttachment(motan.HostKey)
		start = request.GetRPCContext(true).RequestReceiveTime
	case motan.EndPoint:
		role = clientAgentRole
		ip = caller.GetURL().Host
		start = time.Now()
	}
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
	metaUpstreamCode, _ := response.GetAttachments().Load(motan.MetaUpstreamCode)
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

	logEntity := vlog.AcquireAccessLogEntity()
	logEntity.FilterName = filterName
	logEntity.Role = role
	logEntity.RequestID = response.GetRequestID()
	logEntity.Service = request.GetServiceName()
	logEntity.Method = request.GetMethod()
	logEntity.RemoteAddress = address
	logEntity.Desc = request.GetMethodDesc()
	logEntity.ReqSize = reqCtx.BodySize
	logEntity.ResSize = resCtx.BodySize
	logEntity.BizTime = response.GetProcessTime() //ms
	logEntity.TotalTime = totalTime               //ms
	logEntity.ResponseCode = responseCode
	logEntity.Success = exception == nil
	logEntity.Exception = string(exceptionData)
	logEntity.UpstreamCode = metaUpstreamCode

	vlog.AccessLog(logEntity)
}
