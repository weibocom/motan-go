package vlog

import (
	"sync"
	"sync/atomic"
)

var (
	TracePolicy TracePolicyFunc = NoTrace
)

type TracePolicyFunc func(entrypoint Entrypoint)

func NoTrace(entrypoint Entrypoint) {
	return
}

func Trace(entrypoint Entrypoint) {
	TracePolicy(entrypoint)
}

type IndicatorTrace struct {
	indicatorMap sync.Map
}

func NewIndicatorTrace() *IndicatorTrace {
	i := &IndicatorTrace{}
	entrypoints := []Entrypoint{
		EntrypointInfof,
		EntrypointWarningln,
		EntrypointWarningf,
		EntrypointErrorln,
		EntrypointErrorf,
		EntrypointFatalln,
		EntrypointFatalf,
		EntrypointAccessLog,
		EntrypointMetricsLog,
		EntryPointAccessFilterDoLog,
		EntryPointAccessFilterProviderCall,
		EntryPointAccessFilterNotProviderCall,
		EntryPointAccessClusterFilter,
	}
	for _, entrypoint := range entrypoints {
		initValue := int64(0)
		i.indicatorMap.Store(entrypoint.String(), &initValue)
	}
	return i
}

func (t *IndicatorTrace) Trace(entrypoint Entrypoint) {
	if i, ok := t.indicatorMap.Load(entrypoint.String()); ok {
		v := i.(*int64)
		atomic.AddInt64(v, 1)
	}
}

func (t *IndicatorTrace) Format() map[string]int64 {
	result := map[string]int64{}
	t.indicatorMap.Range(func(key, value interface{}) bool {
		k := key.(string)
		v := value.(*int64)
		result[k] = *v
		return true
	})
	return result
}
