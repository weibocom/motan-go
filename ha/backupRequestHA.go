package ha

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	metric "github.com/rcrowley/go-metrics"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
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
	// metric
	registry   metric.Registry
	samples    map[string]metric.Sample
	sampleLock sync.RWMutex
	// counter
	curRoundTotalCount int
	curRoundRetryCount int
	lastResetTime      int64
	counterLock        sync.Mutex
}

func (br *BackupRequestHA) Initialize() {
	br.registry = metric.NewRegistry()
	br.samples = make(map[string]metric.Sample)
	br.lastResetTime = time.Now().UnixNano()
}

func (br *BackupRequestHA) GetName() string {
	return "backupRequestHA"
}

func (br *BackupRequestHA) GetURL() *motan.URL {
	return br.url
}

func (br *BackupRequestHA) SetURL(url *motan.URL) {
	br.url = url
}

func (br *BackupRequestHA) Call(request motan.Request, loadBalance motan.LoadBalance) motan.Response {

	epList := loadBalance.SelectArray(request)
	if len(epList) == 0 {
		return getErrorResponse(request.GetRequestID(), fmt.Sprintf("call backup request fail: %s", "no endpoints"))
	}

	retries := br.url.GetMethodIntValue(request.GetMethod(), request.GetMethodDesc(), "retries", 0)
	if retries == 0 {
		return br.doCall(request, epList[0])
	}

	var resp motan.Response
	backupRequestDelayRatio := br.url.GetMethodPositiveIntValue(request.GetMethod(), request.GetMethodDesc(), "backupRequestDelayRatio", defaultBackupRequestDelayRatio)
	backupRequestMaxRetryRatio := br.url.GetMethodPositiveIntValue(request.GetMethod(), request.GetMethodDesc(), "backupRequestMaxRetryRatio", defaultBackupRequestMaxRetryRatio)
	requestTimeout := br.url.GetMethodPositiveIntValue(request.GetMethod(), request.GetMethodDesc(), "requestTimeout", defaultRequestTimeout)

	successCh := make(chan motan.Response, retries+1)
	methodKey := br.getMethodKey(request)
	histogram := metric.GetOrRegisterHistogram(methodKey, br.registry, br.getSample(methodKey))
	delay := int(histogram.Percentile(float64(backupRequestDelayRatio) / 100.0))
	if delay < 10 {
		delay = 10 // min 10ms
	}

	gTimer := time.NewTimer(time.Duration(requestTimeout) * time.Millisecond)
	defer gTimer.Stop()

	var lastErrorCh chan motan.Response

	for i := 0; i <= int(retries) && i < len(epList); i++ {
		ep := epList[i]
		if i == 0 {
			br.updateCallRecord(counterRoundCount)
		}
		if i > 0 && !br.tryAcquirePermit(int(backupRequestMaxRetryRatio)) {
			vlog.Warningf("The permit is used up, request id: %d\n", request.GetRequestID())
			break
		}
		// log backup request
		if i > 0 {
			vlog.Infof("[backup request ha] delay %s request id: %d, service: %s, method: %s\n", strconv.Itoa(delay), request.GetRequestID(), request.GetServiceName(), methodKey)
		}
		lastErrorCh = make(chan motan.Response, 1)
		go func(endpoint motan.EndPoint, errorCh chan motan.Response) {
			start := time.Now().UnixNano()
			respnose := br.doCall(request, endpoint)
			if respnose != nil && (respnose.GetException() == nil || respnose.GetException().ErrType == motan.BizException) {
				successCh <- respnose
				histogram.Update((time.Now().UnixNano() - start) / 1e6)
			} else {
				errorCh <- respnose
			}
		}(ep, lastErrorCh)

		timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
		defer timer.Stop()
		select {
			case resp = <-successCh:
				return resp
			case <-lastErrorCh:
			case <-timer.C:
			case <-gTimer.C:
				vlog.Warningf("call backup request fail: %s", "global timeout")
				return getErrorResponse(request.GetRequestID(), fmt.Sprintf("call backup request fail: %s", "global timeout"))
		}
	}

	// 没有最终超时的时候，再使用剩下的时间进行等待，取最快的那个返回
	select {
		case resp = <-successCh:
			return resp
		case resp = <-lastErrorCh:
		case <-gTimer.C:
	}
	vlog.Warningf("call backup request fail: %s", "last timeout")

	return getErrorResponse(request.GetRequestID(), fmt.Sprintf("call backup request fail: %s", "last timeout"))

}

func (br *BackupRequestHA) doCall(request motan.Request, endpoint motan.EndPoint) motan.Response {
	defer func() {
		if err := recover(); err != nil {
			vlog.Warningf("BackupRequestHA call encount panic! url:%s, err:%v\n", br.url.GetIdentity(), err)
		}
	}()
	respnose := endpoint.Call(request)
	if respnose.GetException() == nil || respnose.GetException().ErrType == motan.BizException {
		return respnose
	}
	vlog.Warningf("BackupRequestHA call fail! url:%s, err:%+v\n", endpoint.GetURL().GetIdentity(), respnose.GetException())
	return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 400, ErrMsg: fmt.Sprintf(
		"call backup request fail.Exception:%s", respnose.GetException().ErrMsg), ErrType: motan.ServiceException})
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

func (br *BackupRequestHA) getMethodKey(request motan.Request) string {
	return fmt.Sprintf("%s(%s)", request.GetMethod(), request.GetMethodDesc())
}

func (br *BackupRequestHA) getSample(key string) metric.Sample {
	br.sampleLock.RLock()
	if sample, ok := br.samples[key]; ok {
		br.sampleLock.RUnlock()
		return sample
	}
	br.sampleLock.RUnlock()
	br.sampleLock.Lock()
	newSample := metric.NewExpDecaySample(1028, 0)
	br.samples[key] = newSample
	br.sampleLock.Unlock()
	return newSample
}
