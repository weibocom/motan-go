package filter

import (
	"testing"

	motan "github.com/weibocom/motan-go/core"
)

func TestClusterFilter(t *testing.T) {
	factory := initFactory()
	f := factory.GetFilter(ClusterAccessLog)
	if f == nil {
		t.Fatal("can not find clusterAccessLog filter!")
	}
	url := mockURL()
	f = f.NewFilter(url)
	ef := f.(motan.ClusterFilter)
	ef.SetNext(motan.GetLastClusterFilter())
	request := defaultRequest()
	res := ef.Filter(&MockHA{}, &MockLB{}, request)
	t.Logf("res:%+v", res)
}

type MockHA struct{}

func (m *MockHA) GetName() string { return "MockHA" }

func (m *MockHA) GetURL() *motan.URL { return nil }

func (m *MockHA) SetURL(url *motan.URL) {}

func (m *MockHA) Call(motan.Request, motan.LoadBalance) motan.Response { return &motan.MotanResponse{} }

type MockLB struct{}

func (r *MockLB) OnRefresh(endpoints []motan.EndPoint) {}

func (r *MockLB) Select(request motan.Request) motan.EndPoint { return nil }

func (r *MockLB) SelectArray(request motan.Request) []motan.EndPoint { return nil }

func (r *MockLB) SetWeight(weight string) {}
