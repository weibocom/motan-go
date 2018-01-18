package filter

import (
	"time"
	"fmt"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

type AccessLogEndPointFilter struct {
	next motan.EndPointFilter
}

func (t *AccessLogEndPointFilter) GetIndex() int {
	return 11
}

func (t *AccessLogEndPointFilter) GetName() string {
	return "accessLog"
}

func (t *AccessLogEndPointFilter) NewFilter(url *motan.URL) motan.Filter {
	return &AccessLogEndPointFilter{}
}

// Filter : Filter
func (t *AccessLogEndPointFilter) Filter(caller motan.Caller, request motan.Request) motan.Response {
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
		//l = len(response.GetValue().([]byte))
		if b, ok := response.GetValue().([]byte);ok{
			l = len(b)
		}
		if b, ok := response.GetValue().(string);ok{
			l = len(b)
		}
	}
	if response.GetException() != nil {
		success = false
	}
	vlog.Infof("access log--%s:%s,%d,pt:%d,size:%d,req:%s,%s,%s,%d, res:%d,%t,%+v\n", role, ip, caller.GetURL().Port, time.Since(start)/1000000,l, request.GetServiceName(),request.GetMethod(), request.GetMethodDesc(), request.GetRequestID(), response.GetProcessTime(), success, response.GetException())
	return response
}

func (t *AccessLogEndPointFilter) HasNext() bool {
	return t.next != nil
}

func (t *AccessLogEndPointFilter) SetNext(nextFilter motan.EndPointFilter) {
	t.next = nextFilter
}

func (t *AccessLogEndPointFilter) GetNext() motan.EndPointFilter {
	return t.next
}

func (t *AccessLogEndPointFilter) GetType() int32 {
	return motan.EndPointFilterType
}
