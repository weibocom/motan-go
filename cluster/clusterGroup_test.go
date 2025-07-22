package cluster

import (
	"github.com/stretchr/testify/assert"
	motan "github.com/weibocom/motan-go/core"
	"strings"
	"sync/atomic"
	"testing"
)

func Test_NewClusterGroup(t *testing.T) {
	ext := getCustomExt()
	ctx := &motan.Context{}

	caseList := []struct {
		desc       string
		url        *motan.URL
		assertFunc func(t *testing.T, clusterGroup *ClusterGroup)
	}{
		{
			"no slave cluster",
			&motan.URL{
				Protocol: "",
				Host:     "",
				Port:     0,
				Path:     "",
				Group:    "",
				Parameters: map[string]string{
					motan.ClusterSelectorKey: mockClusterKey,
				},
			},
			func(t *testing.T, clusterGroup *ClusterGroup) {
				assert.NotNil(t, clusterGroup.masterCluster)
				assert.Equal(t, 0, len(clusterGroup.GetSandboxClusters()))
				assert.Equal(t, 0, len(clusterGroup.GetBackupClusters()))
				cs, ok := clusterGroup.clusterSelector.(*mockClusterSelector)
				assert.True(t, ok)
				assert.Equal(t, clusterGroup, cs.clusterGroup)

				// check runtime info
				runtimeInfo := clusterGroup.GetRuntimeInfo()
				v, ok := runtimeInfo["master-"+clusterGroup.GetMasterCluster().GetIdentity()]
				assert.True(t, ok)
				assert.NotNil(t, v)
			},
		},
		{
			"backup、 sandbox and grey cluster",
			&motan.URL{
				Protocol: "",
				Host:     "",
				Port:     0,
				Path:     "",
				Group:    "",
				Parameters: map[string]string{
					motan.BackupGroupsKey:    "backup1,backup2",
					motan.SandboxGroupsKey:   "sandbox1",
					motan.GreyGroupsKey:      "grey1",
					motan.ClusterSelectorKey: DefaultClusterSelectorName,
				},
			},
			func(t *testing.T, clusterGroup *ClusterGroup) {
				assert.NotNil(t, clusterGroup.masterCluster)
				assert.Equal(t, 1, len(clusterGroup.GetSandboxClusters()))
				assert.Equal(t, 2, len(clusterGroup.GetBackupClusters()))
				assert.Equal(t, 1, len(clusterGroup.GetGreyClusters()))
				cs, ok := clusterGroup.clusterSelector.(*DefaultClusterSelector)
				assert.True(t, ok)
				assert.Equal(t, clusterGroup, cs.clusterGroup)

				// check runtime info
				runtimeInfo := clusterGroup.GetRuntimeInfo()
				v, ok := runtimeInfo["master-"+clusterGroup.GetMasterCluster().GetIdentity()]
				assert.True(t, ok)
				assert.NotNil(t, v)
				for _, cluster := range clusterGroup.GetSandboxClusters() {
					v, ok = runtimeInfo["sandbox-"+cluster.GetIdentity()]
					assert.True(t, ok)
					assert.NotNil(t, v)
				}
				for _, cluster := range clusterGroup.GetBackupClusters() {
					v, ok = runtimeInfo["backup-"+cluster.GetIdentity()]
					assert.True(t, ok)
					assert.NotNil(t, v)
				}
				for _, cluster := range clusterGroup.GetGreyClusters() {
					v, ok = runtimeInfo["grey-"+cluster.GetIdentity()]
					assert.True(t, ok)
					assert.NotNil(t, v)
				}
			},
		},
	}
	for _, c := range caseList {
		clusterGroup := NewClusterGroup(ctx, ext, c.url, true)
		c.assertFunc(t, clusterGroup.(*ClusterGroup))
	}
}

func TestClusterGroup_Call(t *testing.T) {
	ext := getCustomExt()
	ctx := &motan.Context{}
	url := &motan.URL{
		Protocol: "",
		Host:     "",
		Port:     0,
		Path:     "",
		Group:    "mock_group",
		Parameters: map[string]string{
			motan.ClusterEmptyNodeNotifyKey: "true",
		},
	}
	backupGroups := []string{"backup1", "backup2"}
	backupClusters := createMultiClusters(ctx, ext, url, true, strings.Join(backupGroups, ","), true, false)

	clusterSelector := &DefaultClusterSelector{}
	clusterGroup := &ClusterGroup{
		backupClusters: backupClusters,
		backupIndex:    0,
		context:        nil,
		url:            url,
		masterCluster:  NewCluster(ctx, ext, url, true),
		backupSwitcher: nil,
	}
	clusterSelector.Init(clusterGroup)
	clusterGroup.clusterSelector = clusterSelector
	clusterGroup.backupSwitcher = motan.GetSwitcherManager().GetOrRegister(BackupClusterSwitcherKey, true)

	// verify backup cluster call
	callTimes := 10
	request := &motan.MotanRequest{}
	for i := 0; i < callTimes; i++ {
		clusterGroup.Call(request)
	}
	assert.Equal(t, uint32(callTimes), atomic.LoadUint32(&clusterGroup.backupIndex))
}
