package endpoint

import (
	motan "github.com/weibocom/motan-go/core"
	mpro "github.com/weibocom/motan-go/protocol"
)

// ext name
const (
	Grpc   = "grpc"
	Motan2 = "motan2"
	Mock   = "mockEndpoint"
)

func RegistDefaultEndpoint(extFactory motan.ExtentionFactory) {
	extFactory.RegistExtEndpoint(Motan2, func(url *motan.URL) motan.EndPoint {
		return &MotanEndpoint{url: url}
	})

	extFactory.RegistExtEndpoint(Grpc, func(url *motan.URL) motan.EndPoint {
		return &GrpcEndPoint{url: url}
	})

	extFactory.RegistExtEndpoint(Mock, func(url *motan.URL) motan.EndPoint {
		return &MockEndpoint{URL: url}
	})
}

func GetRequestGroup(r motan.Request) string {
	group := r.GetAttachment(mpro.MGroup)
	if group == "" {
		group = r.GetAttachment(motan.GroupKey)
	}
	return group
}

type MockEndpoint struct {
	URL          *motan.URL
	MockResponse motan.Response
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
