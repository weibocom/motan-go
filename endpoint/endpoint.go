package endpoint

import (
	"sync/atomic"
	"time"

	motan "github.com/weibocom/motan-go/core"
	mpro "github.com/weibocom/motan-go/protocol"
)

// ext name
const (
	Grpc   = "grpc"
	Motan2 = "motan2"
	// Motan1 endpoint is to support dynamic configuration. Golang cannot build motan1 request
	Motan1 = "motan"
	Local  = "local"
	Mock   = "mockEndpoint"
)

const (
	pMask = 0xfffffffffff00000
	sMask = 0x000fffff
)

var idOffset uint64 // id generator offset

func RegistDefaultEndpoint(extFactory motan.ExtensionFactory) {
	extFactory.RegistExtEndpoint(Motan2, func(url *motan.URL) motan.EndPoint {
		return &MotanCommonEndpoint{url: url}
	})

	extFactory.RegistExtEndpoint(Grpc, func(url *motan.URL) motan.EndPoint {
		return &GrpcEndPoint{url: url}
	})

	extFactory.RegistExtEndpoint(Local, func(url *motan.URL) motan.EndPoint {
		return &LocalEndpoint{url: url}
	})

	extFactory.RegistExtEndpoint(Mock, func(url *motan.URL) motan.EndPoint {
		return &MockEndpoint{URL: url}
	})

	extFactory.RegistExtEndpoint(Motan1, func(url *motan.URL) motan.EndPoint {
		return &MotanCommonEndpoint{url: url}
	})
}

func GetRequestGroup(r motan.Request) string {
	group := r.GetAttachment(mpro.MGroup)
	if group == "" {
		group = r.GetAttachment(motan.GroupKey)
	}
	return group
}

func GenerateRequestID() uint64 {
	ms := uint64(time.Now().UnixNano())
	offset := atomic.AddUint64(&idOffset, 1)
	return (ms & pMask) | (offset & sMask)
}

type MockEndpoint struct {
	URL          *motan.URL
	MockResponse motan.Response
}

func (m *MockEndpoint) GetRuntimeInfo() map[string]interface{} {
	return map[string]interface{}{
		motan.RuntimeNameKey: m.GetName(),
	}
}

func (m *MockEndpoint) GetName() string {
	return "mockEndpoint"
}

func (m *MockEndpoint) GetURL() *motan.URL {
	return m.URL
}

func (m *MockEndpoint) SetURL(url *motan.URL) {
	m.URL = url
}

func (m *MockEndpoint) IsAvailable() bool {
	return true
}

func (m *MockEndpoint) SetProxy(proxy bool) {}

func (m *MockEndpoint) SetSerialization(s motan.Serialization) {}

func (m *MockEndpoint) Call(request motan.Request) motan.Response {
	if m.MockResponse != nil {
		return m.MockResponse
	}
	return &motan.MotanResponse{ProcessTime: 1, Value: "ok"}
}

func (m *MockEndpoint) Destroy() {}
