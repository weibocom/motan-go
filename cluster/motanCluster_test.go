package cluster

import (
	"fmt"
	"testing"

	motan "github.com/weibocom/motan-go/core"

	ha "github.com/weibocom/motan-go/ha"
	lb "github.com/weibocom/motan-go/lb"
)

var (
	RegistryURL = &motan.URL{Protocol: "test", Host: "127.0.0.1", Port: 8001}
)

func TestInitFilter(t *testing.T) {
	cluster := initCluster()
	cluster.url.Parameters["filter"] = "test1,test2,test3,test4,test5,test6"
	cluster.initFilters()
	checkClusterFilter(cluster.clusterFilter, 4, t)
	checkEndpointFilter(cluster.Filters, 3, t)
}

func checkClusterFilter(filter motan.ClusterFilter, expectDeep int, t *testing.T) {
	var deep = 1
	lastFilter := filter
	for lastFilter.HasNext() {

		newfilter := lastFilter.GetNext()
		if newfilter.GetIndex() < lastFilter.GetIndex() {
			t.Fatalf("filter seq not correct. next index %d, last index %d", newfilter.GetIndex(), lastFilter.GetIndex())
		}
		deep++
		lastFilter = newfilter
	}
	if deep != expectDeep {
		t.Fatalf("filter deep not correct. expect deep : %d, real deep : %d", expectDeep, deep)
	}

}

func checkEndpointFilter(filters []motan.Filter, expectSize int, t *testing.T) {
	if len(filters) != expectSize {
		t.Fatalf("filter deep not correct. expect size : %d, real size: %d", expectSize, len(filters))
	}
	for i, f := range filters {
		if i != expectSize-1 {
			if f.GetIndex() < filters[i+1].GetIndex() {
				t.Fatalf("filter seq not correct. index %d, next index %d", f.GetIndex(), filters[i+1].GetIndex())
			}
		}
	}
}

func TestNotify(t *testing.T) {
	cluster := initCluster()
	cluster.InitCluster()
	urls := make([]*motan.URL, 0, 2)
	urls = append(urls, &motan.URL{Host: "127.0.0.1", Port: 8001, Protocol: "test"})
	urls = append(urls, &motan.URL{Host: "127.0.0.1", Port: 8002, Protocol: "test"})
	cluster.Notify(RegistryURL, urls)
	if len(cluster.Refers) != 2 {
		t.Fatalf("cluster notify-refers size not correct. expect :2, refers size:%d", len(cluster.Refers))
	}
	//duplicate notify
	cluster.Notify(RegistryURL, urls)
	//ignore empty urls
	urls = make([]*motan.URL, 0, 2)
	cluster.Notify(RegistryURL, urls)
	if len(cluster.Refers) == 0 {
		t.Fatalf("cluster notify-refers size not correct. expect :2, refers size:%d", len(cluster.Refers))
	}

}

func TestCall(t *testing.T) {
	cluster := initCluster()
	cluster.InitCluster()
	response := cluster.Call(&motan.MotanRequest{})
	fmt.Printf("res:%+v", response)
	if response == nil {
		t.Fatal("test call fail. response is nil")
	}
}

func initCluster() *MotanCluster {
	cluster := &MotanCluster{}
	url := &motan.URL{Parameters: make(map[string]string)}
	url.Protocol = "test"
	url.Parameters[motan.Hakey] = "failover"
	url.Parameters[motan.RegistryKey] = "vintage,consul,direct"
	url.Parameters[motan.Lbkey] = "random"
	cluster.Context = &motan.Context{}
	cluster.SetURL(url)
	cluster.SetExtFactory(getCustomExt())
	return cluster
}

func TestDestroy(t *testing.T) {
	cluster := initCluster()
	cluster.InitCluster()
	urls := make([]*motan.URL, 0, 2)
	urls = append(urls, &motan.URL{Host: "127.0.0.1", Port: 8001, Protocol: "test"})
	urls = append(urls, &motan.URL{Host: "127.0.0.1", Port: 8002, Protocol: "test"})
	cluster.Notify(RegistryURL, urls)
	cluster.Destroy()
	if cluster.closed != true {
		t.Fatalf("cluster destroy fail, closed not false")
	}
}

//-------------test struct--------------------
func getCustomExt() motan.ExtensionFactory {
	ext := &motan.DefaultExtensionFactory{}
	ext.Initialize()
	ha.RegistDefaultHa(ext)
	lb.RegistDefaultLb(ext)
	ext.RegistExtFilter("test1", func() motan.Filter {
		return &motan.TestFilter{Index: 1}
	})
	ext.RegistExtFilter("test2", func() motan.Filter {
		return &motan.TestFilter{Index: 2}
	})
	ext.RegistExtFilter("test3", func() motan.Filter {
		return &motan.TestFilter{Index: 3}
	})
	ext.RegistExtFilter("test4", func() motan.Filter {
		return &motan.TestEndPointFilter{Index: 4}
	})
	ext.RegistExtFilter("test5", func() motan.Filter {
		return &motan.TestEndPointFilter{Index: 5}
	})
	ext.RegistExtFilter("test6", func() motan.Filter {
		return &motan.TestEndPointFilter{Index: 6}
	})

	ext.RegistExtLb("test", func(url *motan.URL) motan.LoadBalance {
		return &motan.TestLoadBalance{}
	})
	ext.RegistExtRegistry("test", func(url *motan.URL) motan.Registry {
		return &motan.TestRegistry{URL: url}
	})

	ext.RegistExtEndpoint("test", func(url *motan.URL) motan.EndPoint {
		return &motan.TestEndPoint{URL: url}
	})
	return ext
}
