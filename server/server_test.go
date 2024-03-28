package server

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/weibocom/motan-go/cluster"
	"github.com/weibocom/motan-go/core"
	"sync"
	"testing"
)

type APITestSuite struct {
	suite.Suite
	motanServer               *MotanServer
	motanServerUrl            *core.URL
	motanServerMessageHandler core.MessageHandler
	httpProxyServerUrl        *core.URL
	httpProxyServer           *HTTPProxyServer
}

func (s *APITestSuite) SetupTest() {
	s.motanServerUrl = &core.URL{
		Protocol: "motan2",
		Host:     "127.0.0.1",
		Port:     20243,
		Path:     "testpath",
		Group:    "testgroup",
	}
	s.motanServerMessageHandler = &DefaultMessageHandler{}
	server := &MotanServer{URL: s.motanServerUrl}
	server.Open(false, false, s.motanServerMessageHandler, nil)
	s.motanServer = server

	s.httpProxyServerUrl = &core.URL{
		Host:  "127.0.0.1",
		Port:  20241,
		Path:  "test.path",
		Group: "test.group",
	}
	s.httpProxyServer = NewHTTPProxyServer(s.httpProxyServerUrl)
	s.httpProxyServer.Open(false, false, &testHttpClusterGetter{}, nil)
}

func (s *APITestSuite) TearDownTest() {
	s.motanServer.Destroy()
	s.httpProxyServer.Destroy()
}

func TestAPITestSuite(t *testing.T) {
	suite.Run(t, new(APITestSuite))
}

func (s *APITestSuite) TestMotanServerRuntimeInfo() {
	info := s.motanServer.GetRuntimeInfo()
	assert.NotNil(s.T(), info)
	urlInfo, ok := info[core.RuntimeUrlKey]
	assert.True(s.T(), ok)
	assert.Equal(s.T(), s.motanServerUrl.ToExtInfo(), urlInfo.(string))

	heartbeatEnabled, ok := info[core.RuntimeHeartbeatEnabledKey]
	assert.True(s.T(), ok)
	assert.Equal(s.T(), s.motanServer.heartbeatEnabled, heartbeatEnabled)

	proxy, ok := info[core.RuntimeProxyKey]
	assert.True(s.T(), ok)
	assert.Equal(s.T(), false, proxy)

	maxContentLength, ok := info[core.RuntimeMaxContentLengthKey]
	assert.True(s.T(), ok)
	assert.Equal(s.T(), core.DefaultMaxContentLength, maxContentLength)

	messageHandlerInfo, ok := info[core.RuntimeMessageHandlerKey]
	assert.True(s.T(), ok)

	assert.True(s.T(), ok)
	assert.Equal(s.T(), s.motanServerMessageHandler.GetRuntimeInfo(), messageHandlerInfo)
}

func (s *APITestSuite) TestHttpProxyServerRuntimeInfo() {
	info := s.httpProxyServer.GetRuntimeInfo()
	assert.NotNil(s.T(), info)

	urlInfo, ok := info[core.RuntimeUrlKey]
	assert.True(s.T(), ok)
	assert.Equal(s.T(), s.httpProxyServerUrl.ToExtInfo(), urlInfo.(string))

	deny, ok := info[core.RuntimeDenyKey]
	assert.True(s.T(), ok)
	assert.Equal(s.T(), s.httpProxyServer.deny, deny)

	keepalive, ok := info[core.RuntimeKeepaliveKey]
	assert.True(s.T(), ok)
	assert.Equal(s.T(), s.httpProxyServer.keepalive, keepalive)

	domain, ok := info[core.RuntimeDefaultDomainKey]
	assert.True(s.T(), ok)
	assert.Equal(s.T(), s.httpProxyServer.defaultDomain, domain)
}

type testHttpClusterGetter struct {
	cluster *cluster.HTTPCluster
	once    sync.Once
}

func (t *testHttpClusterGetter) GetHTTPCluster(host string) *cluster.HTTPCluster {
	t.once.Do(func() {
		t.cluster = cluster.NewHTTPCluster(&core.URL{}, false, nil, nil)
	})
	return t.cluster
}
