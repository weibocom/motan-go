package ha

import (
	"bytes"
	"testing"
	"time"

	"github.com/weibocom/motan-go/config"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/filter"
	"github.com/weibocom/motan-go/lb"
	"github.com/weibocom/motan-go/metrics"
	"github.com/weibocom/motan-go/protocol"
)

const (
	TestGroup   = "motan/test.group"
	TestService = "motan/test.service"
	TestMethod  = "motan/test.method"
)

func TestBackupRequestHA_Call(t *testing.T) {
	params := make(map[string]string)
	params[motan.RetriesKey] = "2"
	url := &motan.URL{Protocol: "motan2", Path: TestService, Parameters: params}
	ha := &BackupRequestHA{url: url}
	ha.Initialize()
	haName := ha.GetName()
	if haName != BackupRequest {
		t.Error("Test Case failed.")
	}
	request := &motan.MotanRequest{ServiceName: TestService, Method: TestMethod}
	request.SetAttachment(protocol.MGroup, TestGroup)
	request.SetAttachment(protocol.MPath, TestService)
	nlb := &lb.RoundrobinLB{}

	ctx := &motan.Context{}
	config, _ := config.NewConfigFromReader(bytes.NewReader([]byte(`
metrics:
  period: 1
`)))
	ctx.Config = config
	metrics.StartReporter(ctx)

	//init histogram
	nlb.OnRefresh([]motan.EndPoint{getEP(1)})
	res := ha.Call(request, nlb)
	time.Sleep(10*time.Millisecond + 1*time.Second)
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

func TestBackupRequestHA_Call2(t *testing.T) {
	params := make(map[string]string)
	params[motan.RetriesKey] = "1"
	params["backupRequestInitDelayTime"] = "1"
	url := &motan.URL{Protocol: "motan2", Path: TestService, Parameters: params}
	ha := &BackupRequestHA{url: url}
	ha.Initialize()
	haName := ha.GetName()
	if haName != BackupRequest {
		t.Error("Test Case failed.")
	}
	request := &motan.MotanRequest{ServiceName: TestService, Method: TestMethod}
	request.SetAttachment(protocol.MGroup, TestGroup)
	request.SetAttachment(protocol.MPath, TestService)
	nlb := &lb.RoundrobinLB{}

	ctx := &motan.Context{}
	config, _ := config.NewConfigFromReader(bytes.NewReader([]byte(``)))
	ctx.Config = config
	//init histogram
	nlb.OnRefresh([]motan.EndPoint{getEP(1)})
	res := ha.Call(request, nlb)
	time.Sleep(10*time.Millisecond + 1*time.Second)
	ep1 := getEP(1) // third round
	testEndpoints := []motan.EndPoint{ep1}
	nlb.OnRefresh(testEndpoints)
	res = ha.Call(request, nlb)
	if res.GetProcessTime() != 1 {
		t.Errorf("backupRequest call fail. res:%+v", res)
	}
}
func TestBackupRequestHA_Call3(t *testing.T) {
	params := make(map[string]string)
	params[motan.RetriesKey] = "1"
	//params["backupRequestInitDelayTime"] = "1"
	url := &motan.URL{Protocol: "motan2", Path: TestService, Parameters: params}
	ha := &BackupRequestHA{url: url}
	ha.Initialize()
	haName := ha.GetName()
	if haName != BackupRequest {
		t.Error("Test Case failed.")
	}
	request := &motan.MotanRequest{ServiceName: TestService, Method: TestMethod}
	request.SetAttachment(protocol.MGroup, TestGroup)
	request.SetAttachment(protocol.MPath, TestService)
	nlb := &lb.RoundrobinLB{}

	ctx := &motan.Context{}
	config, _ := config.NewConfigFromReader(bytes.NewReader([]byte(``)))
	ctx.Config = config
	//init histogram
	nlb.OnRefresh([]motan.EndPoint{getEP(1)})
	res := ha.Call(request, nlb)
	time.Sleep(10*time.Millisecond + 1*time.Second)
	ep1 := getEP(1) // third round
	testEndpoints := []motan.EndPoint{ep1}
	nlb.OnRefresh(testEndpoints)
	res = ha.Call(request, nlb)
	if res.GetProcessTime() != 1 {
		t.Errorf("backupRequest call fail. res:%+v", res)
	}
}

func getEP(processTime int64) motan.EndPoint {
	caller := &motan.TestEndPoint{ProcessTime: processTime}
	caller.SetURL(&motan.URL{})
	motan.Initialize(caller)
	fep := &motan.FilterEndPoint{Caller: caller}
	metricsFilter := &filter.MetricsFilter{}
	mf := metricsFilter.NewFilter(nil).(*filter.MetricsFilter)
	mf.SetContext(&motan.Context{Config: config.NewConfig()})
	mf.SetNext(motan.GetLastEndPointFilter())
	fep.Filter = mf
	return fep
}
