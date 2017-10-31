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
	extFactory.RegistExtEndpoint(Motan2, func(url *motan.Url) motan.EndPoint {
		return &MotanEndpoint{url: url}
	})

	extFactory.RegistExtEndpoint(Grpc, func(url *motan.Url) motan.EndPoint {
		return &GrpcEndPoint{url: url}
	})

	extFactory.RegistExtEndpoint(Mock, func(url *motan.Url) motan.EndPoint {
		return &MockEndpoint{Url: url}
	})
}

func GetRequestGroup(r motan.Request) string {
	group := r.GetAttachment(mpro.M_group)
	if group == "" {
		group = r.GetAttachment(motan.GroupKey)
	}
	return group
}

type MockEndpoint struct {
	Url          *motan.Url
	MockResponse motan.Response
}

func (m *MockEndpoint) GetName() string {
	return "mockEndpoint"
}

func (m *MockEndpoint) GetUrl() *motan.Url {
	return m.Url
}

func (m *MockEndpoint) SetUrl(url *motan.Url) {
	m.Url = url
}

func (m *MockEndpoint) IsAvailable() bool {
	return true
}

func (m *MockEndpoint) SetProxy(proxy bool) {}

func (m *MockEndpoint) SetSerialization(s motan.Serialization) {}

func (m *MockEndpoint) Call(request motan.Request) motan.Response {
	if m.MockResponse != nil {
		return m.MockResponse
	} else {
		return &motan.MotanResponse{ProcessTime: 1, Value: "ok"}
	}
}

func (m *MockEndpoint) Destroy() {}
