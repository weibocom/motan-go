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
	url *motan.URL
}

func (f *FailOverHA) GetName() string {
	return "failover"
}
func (f *FailOverHA) GetURL() *motan.URL {
	return f.url
}
func (f *FailOverHA) SetURL(url *motan.URL) {
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
			return getErrorResponse(request.GetRequestID(), fmt.Sprintf("No referers for request, RequestID: %d, Request info: %+v",
				request.GetRequestID(), request.GetAttachments()))
		}
		respnose := ep.Call(request)
		if respnose.GetException() == nil || respnose.GetException().ErrType == motan.BizException {
			return respnose
		}
		lastErr = respnose.GetException()
		vlog.Warningf("FailOverHA call fail! url:%s, err:%+v\n", f.url.GetIdentity(), lastErr)
	}
	return getErrorResponse(request.GetRequestID(), fmt.Sprintf("call fail over %d times.Exception:%s", retries, lastErr.ErrMsg))

}

func getErrorResponse(requestid uint64, errmsg string) *motan.MotanResponse {
	return motan.BuildExceptionResponse(requestid, &motan.Exception{ErrCode: 400, ErrMsg: errmsg, ErrType: motan.ServiceException})
}
