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

func startServer(t *testing.T, path string, port int) motan.ExtensionFactory {
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
    registry: direct
    serialization: simple
    ref : "serviceID"
    export: "motan2:%d"
`
	cfgText = fmt.Sprintf(cfgText, path, port)
	assert := assert2.New(t)
	conf, err := config.NewConfigFromReader(bytes.NewReader([]byte(cfgText)))
	assert.Nil(err)
	ext := GetDefaultExtFactory()
	mscontext := NewMotanServerContextFromConfig(conf)
	err = mscontext.RegisterService(&HelloService{}, "serviceID")
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
