package metrics_callback

import (
	"sync"
)

var (
	statusSamplerRegisterLock sync.Mutex
	statusSamplers            = make(map[string]StatusSampler)
)

type StatusSampler interface {
	Sample() int64
}

type StatusSampleFunc func() int64

func (f StatusSampleFunc) Sample() int64 {
	return f()
}

func RegisterStatusSampleFunc(key string, sf func() int64) {
	RegisterStatusSampler(key, StatusSampleFunc(sf))
}

func RegisterStatusSampler(key string, sampler StatusSampler) {
	statusSamplerRegisterLock.Lock()
	defer statusSamplerRegisterLock.Unlock()
	statusSamplers[key] = sampler
}

func UnregisterStatusSampler(key string) {
	statusSamplerRegisterLock.Lock()
	defer statusSamplerRegisterLock.Unlock()
	delete(statusSamplers, key)
}

func SamplerRangeDo(f func(key string, value StatusSampler) bool) {
	statusSamplerRegisterLock.Lock()
	defer statusSamplerRegisterLock.Unlock()
	for k, e := range statusSamplers {
		if !f(k, e) {
			break
		}
	}
}
