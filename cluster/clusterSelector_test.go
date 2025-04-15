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
	ext := getCustomExt()
	ctx := &motan.Context{}
	url := &motan.URL{
		Protocol:   "test",
		Host:       "",
		Port:       0,
		Path:       "",
		Group:      "masterGroup",
		Parameters: nil,
	}
	clusterSelector := &DefaultClusterSelector{}

	sandboxGroups := []string{"sandbox1", "sandbox2"}
	sandboxClusters := createMultiClusters(ctx, ext, url, true, strings.Join(sandboxGroups, ","), true, true)

	clusterGroup := &ClusterGroup{
		sandboxClusters: sandboxClusters,
		backupIndex:     0,
		context:         nil,
		url:             nil,
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
	assert.Equal(t, "masterGroup", cluster.GetURL().Group)

	// master group when the sandbox group refers is empty
	attachment.Store(MRouteGroup, "sandbox")
	cluster = clusterSelector.Select(request)
	assert.NotNil(t, cluster)
	assert.Equal(t, "masterGroup", cluster.GetURL().Group)

	for _, g := range sandboxGroups {
		attachment.Store(MRouteGroup, g)
		cluster = clusterSelector.Select(request)
		assert.NotNil(t, cluster)
		assert.Equal(t, "masterGroup", cluster.GetURL().Group)
	}

	// sandbox group when the sandbox group refers is not empty
	urlList := []*motan.URL{
		{Host: "127.0.0.1", Port: 1234, Group: "test1", Protocol: "test"},
		{Host: "127.0.0.1", Port: 1235, Group: "test2", Protocol: "test"},
	}
	for _, c := range sandboxClusters {
		c.Notify(RegistryURL, urlList)
	}
	for _, g := range sandboxGroups {
		attachment.Store(MRouteGroup, g)
		cluster = clusterSelector.Select(request)
		assert.NotNil(t, cluster)
		assert.Equal(t, g, cluster.GetURL().Group)
	}
}
