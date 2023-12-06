package ha

import (
	"fmt"
	"sync"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	"github.com/weibocom/motan-go/metrics"
	"github.com/weibocom/motan-go/protocol"
)

const (
	counterRoundCount                 = 100      // 默认每100次请求为一个计数周期
	counterScaleThreshold             = 20 * 1e9 // 如果20s都没有经历一个循环周期则重置，防止过度饥饿
	defaultBackupRequestDelayRatio    = 90       // 默认的请求延迟的水位线，P90
	defaultBackupRequestMaxRetryRatio = 15       // 最大重试比例
	defaultRequestTimeout             = 1000
)

type BackupRequestHA struct {
	url *motan.URL
	// counter
	curRoundTotalCount int
	curRoundRetryCount int
	lastResetTime      int64
	counterLock        sync.Mutex
}

func (br *BackupRequestHA) Initialize() {
	br.lastResetTime = time.Now().UnixNano()
}

func (br *BackupRequestHA) GetName() string {
	return BackupRequest
}

func (br *BackupRequestHA) GetURL() *motan.URL {
	return br.url
}

func (br *BackupRequestHA) SetURL(url *motan.URL) {
	br.url = url
}

func (br *BackupRequestHA) Call(request motan.Request, loadBalance motan.LoadBalance) motan.Response {
	ep := loadBalance.Select(request)
	if ep == nil {
		return getErrorResponseWithCode(request.GetRequestID(), motan.ENoEndpoints, fmt.Sprintf("call backup request fail: %s", "no endpoints"))
	}

	retries := br.url.GetMethodIntValue(request.GetMethod(), request.GetMethodDesc(), motan.RetriesKey, 0)
	if retries <= 0 {
		return br.doCall(request, ep)
	}

	var resp motan.Response
	backupRequestDelayRatio := br.url.GetMethodPositiveIntValue(request.GetMethod(), request.GetMethodDesc(), "backupRequestDelayRatio", defaultBackupRequestDelayRatio)
	backupRequestMaxRetryRatio := br.url.GetMethodPositiveIntValue(request.GetMethod(), request.GetMethodDesc(), "backupRequestMaxRetryRatio", defaultBackupRequestMaxRetryRatio)
	delay := int(br.url.GetMethodIntValue(request.GetMethod(), request.GetMethodDesc(), "backupRequestDelayTime", 0))
	requestTimeout := br.url.GetMethodPositiveIntValue(request.GetMethod(), request.GetMethodDesc(), "requestTimeout", defaultRequestTimeout)

	deadline := time.NewTimer(time.Duration(requestTimeout) * time.Millisecond)
	defer deadline.Stop()

	successCh := make(chan motan.Response, retries+1)
	if delay <= 0 { //no delay time configuration
		// TODO: we should use metrics of the cluster, with traffic control the group may changed
		item := metrics.GetStatItem(metrics.Escape(request.GetAttachment(protocol.MGroup)), metrics.Escape(request.GetAttachment(protocol.MPath)))
		if item == nil || item.LastSnapshot() == nil {
			initDelay := int(br.url.GetMethodPositiveIntValue(request.GetMethod(), request.GetMethodDesc(), "backupRequestInitDelayTime", 0))
			if initDelay == 0 {
				// request LastSnapshot is nil and no init delay time configuration, we just do common call.
				return br.doCall(request, ep)
			}
			// request LastSnapshot is nil and has init delay time configuration, we use init delay as delay time
			delay = initDelay
		} else {
			// use pXX as delay time
			delay = (int)(item.LastSnapshot().Percentile(getKey(request), float64(backupRequestDelayRatio)/100.0))
		}
		if delay < 10 {
			delay = 10 // min 10ms
		}
	}
	var lastErrorCh chan motan.Response

	for i := 0; i <= int(retries); i++ {
		if i == 0 {
			br.updateCallRecord(counterRoundCount)
		} else if ep = loadBalance.Select(request); ep == nil {
			continue
		}
		if i > 0 && !br.tryAcquirePermit(int(backupRequestMaxRetryRatio)) {
			vlog.Warningf("The permit is used up, request id: %d", request.GetRequestID())
			break
		}
		// log & clone backup request
		pr := request
		if i > 0 {
			vlog.Infof("[backup request ha] delay %d request id: %d, service: %s, method: %s", delay, request.GetRequestID(), request.GetServiceName(), request.GetMethod())
			pr = request.Clone().(motan.Request)
		}
		lastErrorCh = make(chan motan.Response, 1)
		go func(postRequest motan.Request, endpoint motan.EndPoint, errorCh chan motan.Response) {
			defer motan.HandlePanic(nil)
			response := br.doCall(postRequest, endpoint)
			if response != nil && (response.GetException() == nil || response.GetException().ErrType == motan.BizException) {
				successCh <- response
			} else {
				errorCh <- response
			}
		}(pr, ep, lastErrorCh)

		timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
		defer timer.Stop()
		select {
		case resp = <-successCh:
			return resp
		case <-lastErrorCh:
		case <-timer.C:
		case <-deadline.C:
			goto BREAK
		}
	}
	select {
	case resp = <-successCh:
		return resp
	case resp = <-lastErrorCh:
	case <-deadline.C:
	}
BREAK:
	return getErrorResponse(request.GetRequestID(), fmt.Sprintf("call backup request fail: %s", "timeout"))
}

func (br *BackupRequestHA) doCall(request motan.Request, endpoint motan.EndPoint) motan.Response {
	response := endpoint.Call(request)
	if response.GetException() == nil || response.GetException().ErrType == motan.BizException {
		return response
	}
	vlog.Warningf("BackupRequestHA call fail! url:%s, err:%+v", endpoint.GetURL().GetIdentity(), response.GetException())
	return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 400, ErrMsg: fmt.Sprintf(
		"call backup request fail.Exception:%s", response.GetException().ErrMsg), ErrType: motan.ServiceException})
}

func (br *BackupRequestHA) updateCallRecord(thresholdLimit int) {
	br.counterLock.Lock()
	defer br.counterLock.Unlock()
	if br.curRoundTotalCount > thresholdLimit || (time.Now().UnixNano()-br.lastResetTime) >= counterScaleThreshold {
		br.curRoundTotalCount = 1
		br.curRoundRetryCount = 0
		br.lastResetTime = time.Now().UnixNano()
	} else {
		br.curRoundTotalCount++
	}
}

func (br *BackupRequestHA) tryAcquirePermit(thresholdLimit int) bool {
	br.counterLock.Lock()
	defer br.counterLock.Unlock()
	if br.curRoundRetryCount >= thresholdLimit {
		return false
	}
	br.curRoundRetryCount++
	return true
}

func getKey(request motan.Request) string {
	role := "motan-client"
	ctx := request.GetRPCContext(false)
	if ctx != nil && ctx.Proxy {
		role = "motan-client-agent"
	}
	return metrics.Escape(role) +
		":" + metrics.Escape(request.GetAttachment(protocol.MSource)) +
		":" + metrics.Escape(request.GetMethod())
}
