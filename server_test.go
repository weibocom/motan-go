package motan

import (
	"bytes"
	"fmt"
	assert2 "github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/config"
	motan "github.com/weibocom/motan-go/core"
	"testing"
	"time"
)

func TestServerContextConfig(t *testing.T) {
	assert := assert2.New(t)
	cfgTpl := `
motan-server:
  log_dir: "stdout"
  application: "app-golang" # server identify.

motan-registry:
  direct:
    protocol: direct

#conf of services
motan-service:
  mytest-motan2:
    path: %s
    group: bj
    protocol: motan2
    registry: direct
    serialization: simple
    ref : "serviceID"
    export: "motan2:%d"
`
	path := "helloService"
	port := 64332
	cfgText := fmt.Sprintf(cfgTpl, path, port)
	conf, err := config.NewConfigFromReader(bytes.NewReader([]byte(cfgText)))
	assert.Nil(err)

	logFilterCallerTrueConfig, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-server:
  log_filter_caller: true
`)))
	assert.Nil(err)
	conf.Merge(logFilterCallerTrueConfig)
	section, err := conf.GetSection("motan-server")
	assert.Nil(err)
	assert.Equal(true, section["log_filter_caller"].(bool))

	ext := startServerFromConfig(assert, conf, path, port)
	clientExt := GetDefaultExtFactory()
	u := motan.FromExtInfo("motan2://127.0.0.1:64332/helloService?serialization=simple")
	assert.NotNil(u)
	ep := clientExt.GetEndPoint(u)
	assert.NotNil(ep)
	ep.SetSerialization(motan.GetSerialization(u, ext))
	motan.Initialize(ep)
	// wait ha
	time.Sleep(time.Second * 1)
	request := newRequest("helloService", "hello", "Ray")
	request.Attachment = motan.NewStringMap(motan.DefaultAttachmentSize)

	resp := ep.Call(request)
	assert.Nil(resp.GetException())
	assert.Equal("Hello Ray from motan server", resp.GetValue())

	port = 64222
	logFilterCallerFalseConfig, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-server:
  log_filter_caller: false
`)))
	assert.Nil(err)
	cfgText = fmt.Sprintf(cfgTpl, path, port)
	conf, err = config.NewConfigFromReader(bytes.NewReader([]byte(cfgText)))
	assert.Nil(err)
	conf.Merge(logFilterCallerFalseConfig)
	section, err = conf.GetSection("motan-server")
	assert.Nil(err)
	assert.Equal(false, section["log_filter_caller"].(bool))

	ext = startServerFromConfig(assert, conf, path, port)
	u = motan.FromExtInfo("motan2://127.0.0.1:64222/helloService?serialization=simple")
	assert.NotNil(u)
	ep = clientExt.GetEndPoint(u)
	assert.NotNil(ep)
	ep.SetSerialization(motan.GetSerialization(u, ext))
	motan.Initialize(ep)
	// wait ha
	time.Sleep(time.Second * 1)
	request = newRequest("helloService", "hello", "Ray")
	request.Attachment = motan.NewStringMap(motan.DefaultAttachmentSize)
}

func TestNewMotanServerContextFromConfig(t *testing.T) {
	assert := assert2.New(t)

	ext := startServer(t, "helloService", 64532)
	clientExt := GetDefaultExtFactory()
	u := motan.FromExtInfo("motan2://127.0.0.1:64532/helloService?serialization=simple")
	assert.NotNil(u)
	ep := clientExt.GetEndPoint(u)
	assert.NotNil(ep)
	ep.SetSerialization(motan.GetSerialization(u, ext))
	motan.Initialize(ep)
	// wait ha
	time.Sleep(time.Second * 1)
	request := newRequest("helloService", "hello", "Ray")
	request.Attachment = motan.NewStringMap(motan.DefaultAttachmentSize)

	resp := ep.Call(request)
	assert.Nil(resp.GetException())
	assert.Equal("Hello Ray from motan server", resp.GetValue())
}

func startServer(t *testing.T, path string, port int, unixSock ...string) motan.ExtensionFactory {
	cfgText := `
motan-server:
  log_dir: "stdout"
  application: "app-golang" # server identify.

motan-registry:
  direct:
    protocol: direct

#conf of services
motan-service:
  mytest-motan2:
    path: %s
    group: bj
    protocol: motan2
    provider: default
    registry: direct
    serialization: simple
    ref : "serviceID"
    export: "motan2:%d"
    unixSock: "%s"
`
	unixSock0 := ""
	if len(unixSock) > 0 && unixSock[0] != "" {
		unixSock0 = unixSock[0]
		port = 0
	}
	cfgText = fmt.Sprintf(cfgText, path, port, unixSock0)
	assert := assert2.New(t)
	conf, err := config.NewConfigFromReader(bytes.NewReader([]byte(cfgText)))
	assert.Nil(err)
	return startServerFromConfig(assert, conf, path, port)
}

func startServerFromConfig(assert *assert2.Assertions, conf *config.Config, path string, port int) motan.ExtensionFactory {
	ext := GetDefaultExtFactory()
	mscontext := NewMotanServerContextFromConfig(conf)
	err := mscontext.RegisterService(&HelloService{}, "serviceID")
	assert.Nil(err)
	mscontext.Start(ext)
	mscontext.ServicesAvailable()

	service := motan.FromExtInfo(fmt.Sprintf("motan2://127.0.0.1:%d/%s?serialization=simple", port, path))
	assert.NotNil(service)

	return ext
}

type HelloService struct{}

func (m *HelloService) Hello(name string) string {
	return fmt.Sprintf("Hello %s from motan server", name)
}
