package endpoint

import (
	"testing"

	motan "github.com/weibocom/motan-go/core"
)

func TestGetEndPoint(t *testing.T) {
	ext := &motan.DefaultExtensionFactory{}
	ext.Initialize()
	RegistDefaultEndpoint(ext)
	url := &motan.URL{Protocol: "motan2", Host: "localhost", Port: 8002}
	ep := ext.GetEndPoint(url)
	if ep == nil {
		t.Errorf("get motan2 endpoint fail.")
	}
}
