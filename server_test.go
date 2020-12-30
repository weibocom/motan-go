package motan

import (
	"bytes"
	"fmt"
	assert2 "github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/config"
	motan "github.com/weibocom/motan-go/core"
	"math/rand"
	"testing"
	"time"
)

func TestNewMotanServerContextFromConfig(t *testing.T) {
	assert := assert2.New(t)
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
    export: "motan2:64532"
`
	conf, err := config.NewConfigFromReader(bytes.NewReader([]byte(cfgText)))
	assert.Nil(err)
	ext := GetDefaultExtFactory()
	mscontext := NewMotanServerContextFromConfig(conf)
	err = mscontext.RegisterService(&HelloService{}, "serviceID")
	assert.Nil(err)
	mscontext.Start(ext)
	mscontext.ServicesAvailable()

	clientExt := GetDefaultExtFactory()
	u := motan.FromExtInfo("motan2://127.0.0.1:64532/helloService?serialization=simple")
	assert.NotNil(u)
	ep := clientExt.GetEndPoint(u)
	assert.NotNil(ep)
	ep.SetSerialization(motan.GetSerialization(u, ext))
	motan.Initialize(ep)
	// wait ha
	time.Sleep(time.Second * 1)
	request := &motan.MotanRequest{}
	request.RequestID = rand.Uint64()
	request.ServiceName = "helloService"
	request.Method = "hello"
	request.Attachment = motan.NewStringMap(motan.DefaultAttachmentSize)
	request.Arguments = []interface{}{"Ray"}

	resp := ep.Call(request)
	assert.Nil(resp.GetException())
	assert.Equal("Hello Ray from motan server", resp.GetValue())
}

type HelloService struct{}

func (m *HelloService) Hello(name string) string {
	return fmt.Sprintf("Hello %s from motan server", name)
}
