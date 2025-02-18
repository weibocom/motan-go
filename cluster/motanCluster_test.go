package cluster

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/registry"
	"os"
	"testing"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/ha"
	"github.com/weibocom/motan-go/lb"
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
	// check runtime info filters
	info := cluster.GetRuntimeInfo()
	filters, ok := info[motan.RuntimeFiltersKey]
	assert.True(t, ok)
	assert.Equal(t, 3, len(filters.([]interface{})))

	clusterFilters, ok := info[motan.RuntimeClusterFiltersKey]
	assert.True(t, ok)
	assert.Equal(t, 4, len(clusterFilters.([]interface{})))
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
	urls = append(urls, &motan.URL{Host: "127.0.0.1", Port: 8001, Protocol: "test"})
	var destroyEndpoint motan.EndPoint
	for _, j := range cluster.Refers {
		if j.GetURL().Port == 8002 {
			destroyEndpoint = j
		}
	}
	if destroyEndpoint == nil {
		t.Fatalf("cluster endpoint is nil")
	}
	if !destroyEndpoint.IsAvailable() {
		t.Fatalf("cluster endpoint should be not available")
	}
	cluster.Notify(RegistryURL, urls)
	time.Sleep(time.Second * 3)
	if len(cluster.Refers) != 1 {
		t.Fatalf("cluster notify-refers size not correct. expect :2, refers size:%d", len(cluster.Refers))
	}
	if destroyEndpoint.IsAvailable() {
		t.Fatalf("cluster endpoint should not be available")
	}

	// check runtime info
	info := cluster.GetRuntimeInfo()
	assert.NotNil(t, info)

	name, ok := info[motan.RuntimeNameKey]
	assert.True(t, ok)
	assert.Equal(t, cluster.GetName(), name)

	refersSize, ok := info[motan.RuntimeRefererSizeKey]
	assert.True(t, ok)
	assert.Equal(t, len(cluster.Refers), refersSize)

	urlInfo, ok := info[motan.RuntimeUrlKey]
	assert.True(t, ok)
	assert.NotNil(t, urlInfo)

	refers, ok := info[motan.RuntimeReferersKey]
	assert.True(t, ok)
	assert.NotNil(t, refers)

	available, ok := refers.(map[string]interface{})[motan.RuntimeAvailableKey]
	assert.True(t, ok)
	assert.NotNil(t, available)

	unavailable, ok := refers.(map[string]interface{})[motan.RuntimeUnavailableKey]
	assert.True(t, ok)
	assert.NotNil(t, unavailable)

	registries, ok := info[motan.RuntimeRegistriesKey]
	assert.True(t, ok)
	assert.NotNil(t, registries)
}

func TestRefersFilters(t *testing.T) {
	cluster := initCluster()
	urls := make([]*motan.URL, 0, 2)
	urls = append(urls, &motan.URL{Host: "127.0.0.1", Port: 8001, Protocol: "test"})
	urls = append(urls, &motan.URL{Host: "110.0.0.1", Port: 8002, Protocol: "test"})
	urls = append(urls, &motan.URL{Host: "139.0.0.1", Port: 8003, Protocol: "test"})
	cluster.Notify(RegistryURL, urls)
	if len(cluster.Refers) != 3 {
		t.Fatalf("cluster notify-refers size not correct. expect :2, refers size:%d", len(cluster.Refers))
	}
	cases := []struct {
		desc          string
		filter        RefersFilter
		expectEpCount int
	}{
		{
			desc: "include ip prefix",
			filter: func() RefersFilter {
				return NewDefaultRefersFilter([]RefersFilterConfig{
					{
						Mode: FilterModeInclude,
						Rule: "127,110",
					},
				})
			}(),
			expectEpCount: 2,
		},
		{
			desc:          "reset filter empty",
			filter:        nil,
			expectEpCount: 3,
		},
		{
			desc: "exclude ip prefix",
			filter: func() RefersFilter {
				return NewDefaultRefersFilter([]RefersFilterConfig{
					{
						Mode: FilterModeExclude,
						Rule: "127,110",
					},
				})
			}(),
			expectEpCount: 1,
		},
		{
			desc:          "reset empty",
			filter:        nil,
			expectEpCount: 3,
		},
	}
	newRefers := make([]motan.EndPoint, 0, 32)
	for _, v := range cluster.registryRefers {
		for _, e := range v {
			newRefers = append(newRefers, e)
		}
	}
	for _, c := range cases {
		t.Logf("test case:%s start", c.desc)
		cluster.SetRefersFilter(c.filter)
		assert.Equal(t, c.expectEpCount, len(cluster.filterRefers(newRefers)))
		t.Logf("test case:%s finsh", c.desc)
	}
}

func TestCall(t *testing.T) {
	cluster := initCluster()
	response := cluster.Call(&motan.MotanRequest{})
	fmt.Printf("res:%+v", response)
	if response == nil {
		t.Fatal("test call fail. response is nil")
	}
}

func initCluster() *MotanCluster {
	return initClusterWithPathGroup("", "")
}

func initClusterWithPathGroup(path string, group string) *MotanCluster {
	url := &motan.URL{Parameters: make(map[string]string)}
	url.Path = path
	url.Group = group
	url.Protocol = "test"
	url.Parameters[motan.Hakey] = "failover"
	url.Parameters[motan.RegistryKey] = "vintage,consul,direct"
	url.Parameters[motan.Lbkey] = "random"
	return NewCluster(&motan.Context{}, getCustomExt(), url, false)
}

func TestDestroy(t *testing.T) {
	cluster := initCluster()
	urls := make([]*motan.URL, 0, 2)
	urls = append(urls, &motan.URL{Host: "127.0.0.1", Port: 8001, Protocol: "test"})
	urls = append(urls, &motan.URL{Host: "127.0.0.1", Port: 8002, Protocol: "test"})
	cluster.Notify(RegistryURL, urls)
	cluster.Destroy()
	if cluster.closed != true {
		t.Fatalf("cluster destroy fail, closed not false")
	}
}

func TestParseRegistryFromEnv(t *testing.T) {
	motan.ClearDirectEnvRegistry()
	os.Setenv(motan.DirectRPCEnvironmentName, "helloService>change_group@127.0.0.1:8005,127.0.0.1:8006;unknownService>127.0.0.1:8888,127.0.0.1:9999")
	cluster := initClusterWithPathGroup("com.weibo.helloService", "group1")
	assert.Equal(t, 2, len(cluster.Refers))
	for _, r := range cluster.GetRefers() {
		assert.Equal(t, "change_group", r.GetURL().Group)
		assert.Equal(t, "127.0.0.1", r.GetURL().Host)
		assert.True(t, r.GetURL().Port == 8005 || r.GetURL().Port == 8006)
	}
}

// -------------test struct--------------------
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

	registry.RegistDefaultRegistry(ext)
	return ext
}
