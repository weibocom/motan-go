package motan

import (
	"bytes"
	motan "github.com/weibocom/motan-go/core"
	"net/http/httptest"
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
		configurer.doRegister(j)
	}
	assert.Equal(t, len(configurer.registerNodes), 4)
}
