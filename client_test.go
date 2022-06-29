package motan

import (
	"bytes"
	"fmt"
	assert2 "github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/config"
	"testing"
	"time"
)

func TestNewClientContextFromConfig(t *testing.T) {
	assert := assert2.New(t)
	StartServer()
	cfgText := `
motan-client:
  log_dir: "stdout"
  application: "app-golang"
motan-registry:
  direct:
    protocol: direct
    host: 127.0.0.1
    port: 64531
motan-refer:
  mytest:
    path: helloService
    group: bj
    protocol: motan2
    registry: direct
    serialization: simple
`
	conf, err := config.NewConfigFromReader(bytes.NewReader([]byte(cfgText)))
	assert.Nil(err)
	ext := GetDefaultExtFactory()
	mccontext := NewClientContextFromConfig(conf)
	mccontext.Start(ext)
	time.Sleep(time.Second)
	mclient := mccontext.GetClient("mytest")
	var reply string
	req := mclient.BuildRequest("hello", []interface{}{"Ray"})
	err = mclient.BaseCall(req, &reply) // sync call
	assert.Nil(err)
	assert.Equal("Hello Ray", reply)

	confWithCallertrue, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-client:
  log_filter_caller: true
`)))
	assert.Nil(err)
	conf.Merge(confWithCallertrue)
	section, err := conf.GetSection("motan-client")
	assert.Nil(err)
	assert.Equal(true, section["log_filter_caller"].(bool))
	mccontext = NewClientContextFromConfig(conf)
	mccontext.Start(ext)
	time.Sleep(time.Second)
	mclient = mccontext.GetClient("mytest")
	req = mclient.BuildRequest("hello", []interface{}{"Ray"})
	err = mclient.BaseCall(req, &reply) // sync call
	assert.Nil(err)
	assert.Equal("Hello Ray", reply)

	confWithCallerFalse, err := config.NewConfigFromReader(bytes.NewReader([]byte(`
motan-client:
  log_filter_caller: false
`)))
	assert.Nil(err)
	conf.Merge(confWithCallerFalse)
	section, err = conf.GetSection("motan-client")
	assert.Nil(err)
	assert.Equal(false, section["log_filter_caller"].(bool))
	mccontext = NewClientContextFromConfig(conf)
	mccontext.Start(ext)
	time.Sleep(time.Second)
	mclient = mccontext.GetClient("mytest")
	req = mclient.BuildRequest("hello", []interface{}{"Ray"})
	err = mclient.BaseCall(req, &reply) // sync call
	assert.Nil(err)
	assert.Equal("Hello Ray", reply)
}

func StartServer() {
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
    path: helloService
    group: bj
    protocol: motan2
    registry: direct
    serialization: simple
    ref : "serviceID"
    export: "motan2:64531"
`
	conf, _ := config.NewConfigFromReader(bytes.NewReader([]byte(cfgText)))
	ext := GetDefaultExtFactory()
	mscontext := NewMotanServerContextFromConfig(conf)
	mscontext.RegisterService(&HelloService1{}, "serviceID")
	mscontext.Start(ext)
	mscontext.ServicesAvailable()
	time.Sleep(time.Second * 3)
}

type HelloService1 struct{}

func (m *HelloService1) Hello(name string) string {
	return fmt.Sprintf("Hello %s", name)
}
