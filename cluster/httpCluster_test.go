package cluster

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/core"
)

type HTTPTestRegistry struct {
	core.TestRegistry
}

func (t *HTTPTestRegistry) Discover(url *core.URL) []*core.URL {
	return []*core.URL{url}
}

func TestHTTPCluster_Call(t *testing.T) {
	domain := "test.domain"
	cfgFile := filepath.Join("testdata", "httpCluster.yaml")
	context := &core.Context{}
	context.ConfigFile = cfgFile
	context.Initialize()
	extFactory := getCustomExt()
	extFactory.RegistExtRegistry("test", func(url *core.URL) core.Registry {
		registry := &HTTPTestRegistry{}
		registry.URL = url
		registry.GroupService = map[string][]string{domain: {"test"}}
		return registry
	})
	uri := "/2/test"
	httpCluster := NewHTTPCluster(context.HTTPClientURLs[domain], true, context, extFactory)
	service, canServe := httpCluster.CanServe(uri)
	assert.True(t, canServe)
	request := &core.MotanRequest{}
	request.ServiceName = service
	request.Method = uri
	response := httpCluster.Call(request)
	assert.True(t, response != nil)

	// runtime info
	info := httpCluster.GetRuntimeInfo()
	assert.NotNil(t, info)

	urlInfo, ok := info[core.RuntimeUrlKey]
	assert.True(t, ok)
	assert.Equal(t, httpCluster.url.ToExtInfo(), urlInfo)

	name, ok := info[core.RuntimeNameKey]
	assert.True(t, ok)
	assert.Equal(t, httpCluster.GetName(), name)

	httpCluster.Destroy()
}
