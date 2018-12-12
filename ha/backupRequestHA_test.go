package ha

import (
	"testing"
	"time"

	"github.com/weibocom/motan-go/config"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/filter"
	"github.com/weibocom/motan-go/lb"
	"github.com/weibocom/motan-go/metrics"
)

func TestBackupRequestHA_Call(t *testing.T) {
	params := make(map[string]string)
	params["retries"] = "2"
	url := &motan.URL{Protocol: "motan", Path: "test/path", Parameters: params}
	ha := &BackupRequestHA{url: url}
	ha.Initialize()
	haName := ha.GetName()
	if haName != BackupRequest {
		t.Error("Test Case failed.")
	}
	request := &motan.MotanRequest{ServiceName: "test", Method: "test"}
	request.SetAttachment("M_g", "group")
	request.SetAttachment("M_p", "service")
	nlb := &lb.RoundrobinLB{}

	//init histogram
	nlb.OnRefresh([]motan.EndPoint{getEP(1)})
	res := ha.Call(request, nlb)
	time.Sleep(10 * time.Millisecond)
	metrics.GetStatItem("group", "service").SnapshotAndClear()

	ep1 := getEP(1)  // third round
	ep2 := getEP(40) // first round
	ep3 := getEP(20) // second round
	testEndpoints := []motan.EndPoint{ep1, ep2, ep3}
	nlb.OnRefresh(testEndpoints)
	res = ha.Call(request, nlb)
	if res.GetProcessTime() != 1 {
		t.Errorf("backupRequest call fail. res:%+v", res)
	}
}

func getEP(processTime int64) motan.EndPoint {
	fep := &motan.FilterEndPoint{Caller: &motan.TestEndPoint{ProcessTime: processTime}}
	mf := &filter.MetricsFilter{}
	mf.SetContext(&motan.Context{Config: config.NewConfig()})
	mf.SetNext(motan.GetLastEndPointFilter())
	fep.Filter = mf
	return fep
}
