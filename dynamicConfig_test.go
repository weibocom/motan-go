package motan

import (
	"bytes"
	"github.com/weibocom/motan-go/cluster"
	motan "github.com/weibocom/motan-go/core"
	mserver "github.com/weibocom/motan-go/server"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDynamicConfigurerHandler_readURLs(t *testing.T) {
	body1 := `{"protocol":"motan2","host":"10.10.64.11","port":1880,"path":"com.company.HelloService","group":"hello","parameters":{"conf-id":"com.company.HelloService","export":"motan2:1880","nodeType":"service","proxyRegistry":"direct://127.0.0.1:1880","ref":"com.company.HelloService","registry":"mesh-registry","requestTimeout":"600000","serialization":"breeze"}}`
	body2 := `{"protocol":"motan2","host":"10.10.64.11","port":1880,"path":"com.company.HelloService","group":"hello,hello1,hello2","parameters":{"conf-id":"com.company.HelloService","export":"motan2:1880","nodeType":"service","proxyRegistry":"direct://127.0.0.1:1880","ref":"com.company.HelloService","registry":"mesh-registry","requestTimeout":"600000","serialization":"breeze"}}`
	d := &DynamicConfigurerHandler{}
	req1 := httptest.NewRequest("POST", "/register", bytes.NewBufferString(body1))
	req2 := httptest.NewRequest("POST", "/register", bytes.NewBufferString(body2))
	req3 := httptest.NewRequest("POST", "/register", bytes.NewBufferString("}"))
	urls, err := d.readURLsFromRequest(req1)
	assert.Equal(t, len(urls), 1)
	assert.Nil(t, err)
	urls, err = d.readURLsFromRequest(req2)
	assert.Equal(t, len(urls), 3)
	assert.Nil(t, err)
	assert.Equal(t, urls[0].Group, "hello")
	assert.Equal(t, urls[1].Group, "hello1")
	assert.Equal(t, urls[2].Group, "hello2")
	_, err = d.readURLsFromRequest(req3)
	assert.NotNil(t, err)
}

func TestDynamicConfigurerMultiRegistry(t *testing.T) {
	a := NewAgent(nil)
	a.Context = &motan.Context{
		RegistryURLs:     make(map[string]*motan.URL),
		RefersURLs:       make(map[string]*motan.URL),
		BasicReferURLs:   make(map[string]*motan.URL),
		ServiceURLs:      make(map[string]*motan.URL),
		BasicServiceURLs: make(map[string]*motan.URL),
	}
	configurer := &DynamicConfigurer{
		agent:          a,
		registerNodes:  make(map[string]*motan.URL),
		subscribeNodes: make(map[string]*motan.URL),
	}
	urls := []*motan.URL{
		{
			Protocol: "motan2",
			Host:     "127.0.0.1",
			Port:     1910,
			Path:     "test_path",
			Group:    "test_group",
			Parameters: map[string]string{
				"registry": "r1",
			},
		},
		{
			Protocol: "motan2",
			Host:     "127.0.0.1",
			Port:     1910,
			Path:     "test_path",
			Group:    "test_group",
			Parameters: map[string]string{
				"registry": "r2",
			},
		},
		{
			Protocol: "motan2",
			Host:     "127.0.0.1",
			Port:     1910,
			Path:     "test_path",
			Group:    "test_group1",
			Parameters: map[string]string{
				"registry": "r1",
			},
		},
		{
			Protocol: "motan2",
			Host:     "127.0.0.1",
			Port:     1910,
			Path:     "test_path",
			Group:    "test_group1",
			Parameters: map[string]string{
				"registry": "r2",
			},
		},
		// 增加一个重复的
		{
			Protocol: "motan2",
			Host:     "127.0.0.1",
			Port:     1910,
			Path:     "test_path",
			Group:    "test_group1",
			Parameters: map[string]string{
				"registry": "r2",
			},
		},
	}
	for _, j := range urls {
		err := configurer.doRegister(j)
		assert.Nil(t, err)
		exporter, ok := a.serviceExporters.Load(j.GetIdentityWithRegistry())
		assert.True(t, ok)
		defaultExporter, ok := exporter.(*mserver.DefaultExporter)
		assert.True(t, ok)
		assert.Equal(t, defaultExporter.GetURL().GetIdentityWithRegistry(), j.GetIdentityWithRegistry())
	}
	assert.Equal(t, len(configurer.registerNodes), 4)

	// test RegGroupSuffix env
	regGroupSuffix := "-test"
	configurer2 := &DynamicConfigurer{
		agent:          a,
		registerNodes:  make(map[string]*motan.URL),
		subscribeNodes: make(map[string]*motan.URL),
	}
	url := &motan.URL{
		Protocol: "motan2",
		Host:     "127.0.0.1",
		Port:     1910,
		Path:     "test_path_env",
		Group:    "test_group_env",
		Parameters: map[string]string{
			"registry": "r1",
		},
	}
	urlCopy := url.Copy()

	os.Setenv(motan.RegGroupSuffix, regGroupSuffix)
	err := configurer2.doRegister(url)
	assert.Nil(t, err)
	assert.Equal(t, len(configurer2.registerNodes), 1)

	urlCopy.Group += regGroupSuffix
	regUrl, ok := configurer2.registerNodes[url.GetIdentityWithRegistry()]
	assert.True(t, ok)
	assert.Equal(t, urlCopy.Group, regUrl.Group)

	exporter, ok := a.serviceExporters.Load(urlCopy.GetIdentityWithRegistry())
	assert.True(t, ok)
	defaultExporter, ok := exporter.(*mserver.DefaultExporter)
	assert.True(t, ok)
	assert.Equal(t, defaultExporter.GetURL().GetIdentityWithRegistry(), urlCopy.GetIdentityWithRegistry())
	os.Unsetenv(motan.RegGroupSuffix)
}

func TestDynamicConfigurer_Subscribe(t *testing.T) {
	a := NewAgent(nil)
	a.Context = &motan.Context{
		RegistryURLs:     make(map[string]*motan.URL),
		RefersURLs:       make(map[string]*motan.URL),
		BasicReferURLs:   make(map[string]*motan.URL),
		ServiceURLs:      make(map[string]*motan.URL),
		BasicServiceURLs: make(map[string]*motan.URL),
	}
	a.agentURL = &motan.URL{}
	configure := &DynamicConfigurer{
		agent:          a,
		registerNodes:  make(map[string]*motan.URL),
		subscribeNodes: make(map[string]*motan.URL),
	}
	urls := []*motan.URL{
		{
			Protocol: "motan2",
			Path:     "test_path",
			Group:    "test_group",
			Parameters: map[string]string{
				"registry": "r1",
			},
		},
	}

	subGroupSuffix := "-test"
	os.Setenv(motan.SubGroupSuffix, subGroupSuffix)
	for _, url := range urls {
		err := configure.Subscribe(url)
		assert.Nil(t, err)
		key := getClusterKey(url.Group, url.GetStringParamsWithDefault(motan.VersionKey, motan.DefaultReferVersion), url.Protocol, url.Path)
		c, ok := a.clusterMap.Load(key)
		assert.True(t, ok)
		motanCluster, ok := c.(*cluster.MotanCluster)
		assert.True(t, ok)
		assert.Equal(t, motanCluster.GetURL().GetIdentityWithRegistry(), url.GetIdentityWithRegistry())

		// test SubGroupSuffix env
		copyUrl := url.Copy()
		copyUrl.Group += subGroupSuffix
		key2 := getClusterKey(copyUrl.Group, copyUrl.GetStringParamsWithDefault(motan.VersionKey, motan.DefaultReferVersion), copyUrl.Protocol, copyUrl.Path)
		c2, ok := a.clusterMap.Load(key2)
		assert.True(t, ok)
		motanCluster2, ok := c2.(*cluster.MotanCluster)
		assert.True(t, ok)
		assert.Equal(t, motanCluster2.GetURL().GetIdentityWithRegistry(), copyUrl.GetIdentityWithRegistry())
	}
	os.Unsetenv(motan.SubGroupSuffix)
}
