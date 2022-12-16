package vlog

import (
	assert2 "github.com/stretchr/testify/assert"
	"testing"
)

func Test_trace(t *testing.T) {
	assert := assert2.New(t)
	traceObj := NewIndicatorTrace()
	TracePolicy = traceObj.Trace
	callAllTraceOnce()
	traceObj.indicatorMap.Range(func(key, value interface{}) bool {
		v := value.(*int64)
		assert.Equal(int64(1), *v)
		return true
	})
}

func callAllTraceOnce() {
	Trace(EntryPointAccessFilterDoLog)
	Trace(EntryPointAccessFilterProviderCall)
	Trace(EntryPointAccessFilterNotProviderCall)
	Trace(EntryPointAccessClusterFilter)
	Trace(EntrypointMetricsCall)
	MetricsLog("test")
	AccessLog(&AccessLogEntity{})
	Infof("", "")
	Infoln("")
	Warningf("", "")
	Warningln("")
	Errorf("", "")
	Errorln("")
	Fatalf("", "")
	Fatalln("")
}
