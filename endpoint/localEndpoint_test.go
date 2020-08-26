package endpoint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/serialize"
)

func TestLocalEndpoint_Call(t *testing.T) {
	service := "testService"
	url := &motan.URL{Protocol: motan.ProtocolLocal, Path: service}
	motan.RegistLocalProvider(service, &motan.TestProvider{})
	ep := &LocalEndpoint{}
	ep.SetURL(url)
	ep.SetProxy(true)
	ep.SetSerialization(&serialize.SimpleSerialization{})
	ep.Initialize()
	request := &motan.MotanRequest{ServiceName: service, Method: "test"}
	response := ep.Call(request)
	assert.Nil(t, response.GetException())
}
