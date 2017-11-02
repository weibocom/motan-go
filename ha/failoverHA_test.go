package ha

import (
	"testing"

	motan "github.com/weibocom/motan-go/core"
)

func TestGetName(t *testing.T) {
	params := make(map[string]string)
	url := &motan.URL{Protocol: "motan", Path: "test/path", Parameters: params}
	ha := &FailOverHA{url: url}
	haName := ha.GetName()
	if haName != "failover" {
		t.Error("Test Case failed.")
	}
	request := &motan.MotanRequest{ServiceName: "test", Method: "test"}
	request.Attachment = make(map[string]string, 0)
	nlb := &motan.TestLoadBalance{}
	res := ha.Call(request, nlb)
	if res == nil {
		t.Errorf("ha call fail. res:%+v", res)
	}
}
