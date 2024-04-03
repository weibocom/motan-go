package endpoint

import (
	motan "github.com/weibocom/motan-go/core"
	mpro "github.com/weibocom/motan-go/protocol"
	"strconv"
	"sync"
	"sync/atomic"
)

type MockDynamicEndpoint struct {
	URL           *motan.URL
	available     bool
	DynamicWeight int64
	StaticWeight  int64
	Count         int64
	dynamicMeta   sync.Map
}

func (m *MockDynamicEndpoint) GetName() string {
	return "mockEndpoint"
}

func (m *MockDynamicEndpoint) GetURL() *motan.URL {
	return m.URL
}

func (m *MockDynamicEndpoint) SetURL(url *motan.URL) {
	m.URL = url
}

func (m *MockDynamicEndpoint) IsAvailable() bool {
	return m.available
}

func (m *MockDynamicEndpoint) SetAvailable(a bool) {
	m.available = a
}

func (m *MockDynamicEndpoint) SetProxy(proxy bool) {}

func (m *MockDynamicEndpoint) SetSerialization(s motan.Serialization) {}

func (m *MockDynamicEndpoint) Call(request motan.Request) motan.Response {
	if isMetaServiceRequest(request) {
		resMap := make(map[string]string)
		m.dynamicMeta.Range(func(key, value interface{}) bool {
			resMap[key.(string)] = value.(string)
			return true
		})
		atomic.AddInt64(&m.Count, 1)
		return &motan.MotanResponse{ProcessTime: 1, Value: resMap}
	}
	atomic.AddInt64(&m.Count, 1)
	return &motan.MotanResponse{ProcessTime: 1, Value: "ok"}
}

func (m *MockDynamicEndpoint) Destroy() {}

func (m *MockDynamicEndpoint) GetRuntimeInfo() map[string]interface{} {
	return make(map[string]interface{})
}

func (m *MockDynamicEndpoint) SetWeight(isDynamic bool, weight int64) {
	if isDynamic {
		m.DynamicWeight = weight
		m.dynamicMeta.Store(motan.DefaultMetaPrefix+motan.WeightMetaSuffixKey, strconv.Itoa(int(weight)))
	} else {
		m.StaticWeight = weight
		m.URL.PutParam(motan.DefaultMetaPrefix+motan.WeightMetaSuffixKey, strconv.Itoa(int(weight)))
	}
}

func NewMockDynamicEndpoint(url *motan.URL) *MockDynamicEndpoint {
	return &MockDynamicEndpoint{
		URL:       url,
		available: true,
	}
}

func NewMockDynamicEndpointWithWeight(url *motan.URL, staticWeight int64) *MockDynamicEndpoint {
	res := NewMockDynamicEndpoint(url)
	res.StaticWeight = staticWeight
	res.URL.PutParam(motan.DefaultMetaPrefix+motan.WeightMetaSuffixKey, strconv.Itoa(int(staticWeight)))
	return res
}

func isMetaServiceRequest(request motan.Request) bool {
	return request != nil && "com.weibo.api.motan.runtime.meta.MetaService" == request.GetServiceName() &&
		"getDynamicMeta" == request.GetMethod() && "y" == request.GetAttachment(mpro.MFrameworkService)
}
