package lb

import (
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/meta"
	mpro "github.com/weibocom/motan-go/protocol"
	"strconv"
	"sync"
	"sync/atomic"
)

type MockDynamicEndpoint struct {
	URL           *motan.URL
	available     bool
	dynamicWeight int64
	staticWeight  int64
	count         int64
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
		atomic.AddInt64(&m.count, 1)
		return &motan.MotanResponse{ProcessTime: 1, Value: resMap}
	}
	atomic.AddInt64(&m.count, 1)
	return &motan.MotanResponse{ProcessTime: 1, Value: "ok"}
}

func (m *MockDynamicEndpoint) Destroy() {}

func (m *MockDynamicEndpoint) SetWeight(isDynamic bool, weight int64) {
	if isDynamic {
		m.dynamicWeight = weight
		m.dynamicMeta.Store(motan.DefaultMetaPrefix+motan.WeightMetaSuffixKey, strconv.Itoa(int(weight)))
	} else {
		m.staticWeight = weight
		m.URL.PutParam(motan.DefaultMetaPrefix+motan.WeightMetaSuffixKey, strconv.Itoa(int(weight)))
	}
}

func newMockDynamicEndpoint(url *motan.URL) *MockDynamicEndpoint {
	return &MockDynamicEndpoint{
		URL:       url,
		available: true,
	}
}

func newMockDynamicEndpointWithWeight(url *motan.URL, staticWeight int64) *MockDynamicEndpoint {
	res := newMockDynamicEndpoint(url)
	res.staticWeight = staticWeight
	res.URL.PutParam(motan.DefaultMetaPrefix+motan.WeightMetaSuffixKey, strconv.Itoa(int(staticWeight)))
	return res
}

func isMetaServiceRequest(request motan.Request) bool {
	return request != nil && meta.MetaServiceName == request.GetServiceName() &&
		meta.MetaMethodName == request.GetMethod() && "y" == request.GetAttachment(mpro.MFrameworkService)
}
