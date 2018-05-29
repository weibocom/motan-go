package ha

import (
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/lb"
	"testing"
)

func TestBackupRequestHA_Call(t *testing.T) {
	params := make(map[string]string)
	params["retries"] = "2"
	url := &motan.URL{Protocol: "motan", Path: "test/path", Parameters: params}
	ha := &BackupRequestHA{url: url}
	ha.Initialize()
	haName := ha.GetName()
	if haName != "backupRequestHA" {
		t.Error("Test Case failed.")
	}
	request := &motan.MotanRequest{ServiceName: "test", Method: "test"}
	request.Attachment = motan.NewConcurrentStringMap()
	nlb := &lb.RoundrobinLB{}
	ep1 := &motan.TestEndPoint{}
	ep1.SetProcessTime(1) // third round
	ep2 := &motan.TestEndPoint{}
	ep2.SetProcessTime(40) // first round
	ep3 := &motan.TestEndPoint{}
	ep3.SetProcessTime(20) // second round
	testEndpoints := []motan.EndPoint{ep1, ep2, ep3}
	nlb.OnRefresh(testEndpoints)
	res := ha.Call(request, nlb)
	if res.GetProcessTime() != 1 {
		t.Errorf("backupRequest call fail. res:%+v", res)
	}
}
