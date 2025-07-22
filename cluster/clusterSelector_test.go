package cluster

import (
	"github.com/stretchr/testify/assert"
	motan "github.com/weibocom/motan-go/core"
	"strings"
	"testing"
)

var (
	mockClusterKey = "mockClusterSelector"
)

type mockClusterSelector struct {
	clusterGroup motan.ClusterGroup
}

func (m *mockClusterSelector) Destroy() {
}

func (m *mockClusterSelector) Select(request motan.Request) motan.Cluster {
	return m.clusterGroup.GetMasterCluster()
}

func (m *mockClusterSelector) Init(clusterGroup motan.ClusterGroup) {
	m.clusterGroup = clusterGroup
}

func TestDefaultClusterSelector_Select(t *testing.T) {
	masterGroup := "masterGroup"
	ext := getCustomExt()
	ctx := &motan.Context{}
	url := &motan.URL{
		Protocol:   "test",
		Host:       "",
		Port:       0,
		Path:       "",
		Group:      masterGroup,
		Parameters: nil,
	}
	clusterSelector := &DefaultClusterSelector{}

	sandboxGroups := []string{"sandbox1", "sandbox2"}
	sandboxClusters := createMultiClusters(ctx, ext, url, true, strings.Join(sandboxGroups, ","), true, true)
	greyGroups := []string{"grey1", "grey2", "grey3"}
	greyClusters := createMultiClusters(ctx, ext, url, true, strings.Join(greyGroups, ","), false, true)

	clusterGroup := &ClusterGroup{
		sandboxClusters: sandboxClusters,
		greyClusters:    greyClusters,
		backupIndex:     0,
		context:         nil,
		url:             url,
		masterCluster:   NewCluster(ctx, ext, url, true),
		backupSwitcher:  nil,
	}
	clusterSelector.Init(clusterGroup)
	clusterGroup.clusterSelector = clusterSelector

	// master group
	attachment := motan.NewStringMap(12)
	request := &motan.MotanRequest{
		RequestID:   0,
		ServiceName: "",
		Method:      "",
		MethodDesc:  "",
		Arguments:   nil,
		Attachment:  attachment,
		RPCContext:  nil,
	}
	cluster := clusterSelector.Select(request)
	assert.NotNil(t, cluster)
	assert.Equal(t, masterGroup, cluster.GetURL().Group)

	// master group when the sandbox group refers is empty
	attachment.Store(MRouteGroup, DefaultSandboxRouteGroup)
	cluster = clusterSelector.Select(request)
	assert.NotNil(t, cluster)
	assert.Equal(t, masterGroup, cluster.GetURL().Group)

	for _, g := range sandboxGroups {
		attachment.Store(MRouteGroup, g)
		cluster = clusterSelector.Select(request)
		assert.NotNil(t, cluster)
		assert.Equal(t, masterGroup, cluster.GetURL().Group)
	}

	// sandbox group when the sandbox group refers is not empty
	urlList := []*motan.URL{
		{Host: "127.0.0.1", Port: 1234, Group: "test1", Protocol: "test"},
		{Host: "127.0.0.1", Port: 1235, Group: "test2", Protocol: "test"},
	}
	for _, c := range sandboxClusters {
		c.Notify(RegistryURL, urlList)
	}
	for _, c := range greyClusters {
		c.Notify(RegistryURL, urlList)
	}
	attachment.Store(MRouteGroup, DefaultSandboxRouteGroup)
	for _ = range sandboxGroups {
		cluster = clusterSelector.Select(request)
		assert.NotNil(t, cluster)
		assert.Equal(t, clusterSelector.defaultSandboxCluster.GetURL().Group, cluster.GetURL().Group)
	}

	// master group when the motan-route-group not equal to the master cluster group
	for _ = range sandboxGroups {
		attachment.Store(MRouteGroup, "not-exist-group")
		cluster = clusterSelector.Select(request)
		assert.NotNil(t, cluster)
		assert.Equal(t, masterGroup, cluster.GetURL().Group)
	}

	// sandbox group when the motan-route-group equal to the sandbox cluster group
	attachment.Store(MRouteGroup, clusterSelector.defaultSandboxCluster.GetURL().Group)
	cluster = clusterSelector.Select(request)
	assert.NotNil(t, cluster)
	assert.Equal(t, clusterSelector.defaultSandboxCluster.GetURL().Group, cluster.GetURL().Group)

	// grey group when the motan-route-group equal to the grey cluster group
	attachment.Store(MRouteGroup, clusterSelector.defaultGreyCluster.GetURL().Group)
	cluster = clusterSelector.Select(request)
	assert.NotNil(t, cluster)
	assert.Equal(t, clusterSelector.defaultGreyCluster.GetURL().Group, cluster.GetURL().Group)
}
