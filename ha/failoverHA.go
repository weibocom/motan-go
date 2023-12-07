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
	return FailOver
}

func (f *FailOverHA) GetURL() *motan.URL {
	return f.url
}

func (f *FailOverHA) SetURL(url *motan.URL) {
	f.url = url
}

func (f *FailOverHA) Call(request motan.Request, loadBalance motan.LoadBalance) motan.Response {
	retries := f.url.GetMethodPositiveIntValue(request.GetMethod(), request.GetMethodDesc(), motan.RetriesKey, defaultRetries)
	var lastErr *motan.Exception
	var response motan.Response
	for i := 0; i <= int(retries); i++ {
		ep := loadBalance.Select(request)
		if ep == nil {
			return getErrorResponseWithCode(request.GetRequestID(), motan.ENoEndpoints,
				fmt.Sprintf("No refers for request, RequestID: %d, Request info: %+v",
					request.GetRequestID(), request.GetAttachments().RawMap()))
		}
		response = ep.Call(request)
		if response != nil {
			if response.GetException() == nil || response.GetException().ErrType == motan.BizException {
				return response
			}
			lastErr = response.GetException()
		}
		vlog.Warningf("FailOverHA call fail! url:%s, err:%+v", ep.GetURL().GetIdentity(), lastErr)
	}
	if response != nil { // last response
		return response
	}
	var errMsg string
	if lastErr != nil {
		errMsg = lastErr.ErrMsg
	}
	errorResponse := getErrorResponseWithCode(request.GetRequestID(), 500, fmt.Sprintf("FailOverHA call fail %d times. Exception: %s", retries+1, errMsg))
	return errorResponse
}

func getErrorResponse(requestID uint64, errMsg string) *motan.MotanResponse {
	return getErrorResponseWithCode(requestID, 400, errMsg)
}

func getErrorResponseWithCode(requestID uint64, errCode int, errMsg string) *motan.MotanResponse {
	return motan.BuildExceptionResponse(requestID, &motan.Exception{ErrCode: errCode, ErrMsg: errMsg, ErrType: motan.ServiceException})
}
