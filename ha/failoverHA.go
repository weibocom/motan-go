package ha

import (
	"fmt"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	defaultRetries = 0
)

type FailOverHA struct {
	url *motan.Url
}

func (f *FailOverHA) GetName() string {
	return "failover"
}
func (f *FailOverHA) GetUrl() *motan.Url {
	return f.url
}
func (f *FailOverHA) SetUrl(url *motan.Url) {
	f.url = url
}
func (f *FailOverHA) Call(request motan.Request, loadBalance motan.LoadBalance) motan.Response {
	defer func() {
		if err := recover(); err != nil {
			vlog.Warningf("FailOverHA call encount panic! url:%s, err:%v\n", f.url.GetIdentity(), err)
		}
	}()
	retries := f.url.GetMethodPositiveIntValue(request.GetMethod(), request.GetMethodDesc(), "retries", defaultRetries)
	var lastErr *motan.Exception
	for i := 0; i <= int(retries); i++ {
		ep := loadBalance.Select(request)
		if ep == nil {
			return getErrorResponse(request.GetRequestId(), fmt.Sprintf("No referers for request, RequestID: %d, Request info: %+v",
				request.GetRequestId(), request.GetAttachments()))
		}
		respnose := ep.Call(request)
		if respnose.GetException() == nil || respnose.GetException().ErrType == motan.BizException {
			return respnose
		}
		lastErr = respnose.GetException()
		vlog.Warningf("FailOverHA call fail! url:%s, err:%+v\n", f.url.GetIdentity(), lastErr)
	}
	return getErrorResponse(request.GetRequestId(), fmt.Sprintf("call fail over %d times.Exception:%s", retries, lastErr.ErrMsg))

}

func getErrorResponse(requestid uint64, errmsg string) *motan.MotanResponse {
	return motan.BuildExceptionResponse(requestid, &motan.Exception{ErrCode: 400, ErrMsg: errmsg, ErrType: motan.ServiceException})
}
