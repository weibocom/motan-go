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
	res := ef.Filter(&motan.TestHaStrategy{}, &motan.TestLoadBalance{}, request)
	t.Logf("res:%+v", res)
}
