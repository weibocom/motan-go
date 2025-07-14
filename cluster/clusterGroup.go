package cluster

import (
	motan "github.com/weibocom/motan-go/core"
	vlog "github.com/weibocom/motan-go/log"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	BackupClusterSwitcherKey = "feature.motan.backup.cluster.enable"
)

type ClusterGroup struct {
	sandboxClusters []motan.Cluster
	backupClusters  []motan.Cluster
	greyClusters    []motan.Cluster
	backupIndex     uint32

	clusterSelector motan.ClusterSelector
	context         *motan.Context
	url             *motan.URL
	masterCluster   motan.Cluster
	backupSwitcher  *motan.Switcher
}

func NewClusterGroup(context *motan.Context, extFactory motan.ExtensionFactory, url *motan.URL, proxy bool) motan.ClusterGroup {
	clusterGroup := &ClusterGroup{
		context: context,
		url:     url,
	}
	clusterGroup.masterCluster = NewCluster(context, extFactory, url, proxy)

	// sandbox clusters
	sandboxGroup := url.GetParam(motan.SandboxGroupsKey, motan.GetDefaultSandboxGroups())
	if sandboxGroup != "" {
		clusterGroup.sandboxClusters = createMultiClusters(context, extFactory, url, proxy, sandboxGroup, true, true)
		vlog.Infof("init sandbox clusters success. master cluster url: %s, sandbox groups: %s", url.ToExtInfo(), sandboxGroup)
	}

	// backup clusters
	backupGroup := url.GetParam(motan.BackupGroupsKey, "")
	if backupGroup != "" {
		clusterGroup.backupClusters = createMultiClusters(context, extFactory, url, proxy, backupGroup, true, false)
		vlog.Infof("init backup clusters success. master cluster url: %s, backup groups: %s", url.ToExtInfo(), backupGroup)
	}

	// gray clusters
	greyGroups := url.GetParam(motan.GreyGroupsKey, motan.GetDefaultGreyGroups())
	if greyGroups != "" {
		clusterGroup.greyClusters = createMultiClusters(context, extFactory, url, proxy, greyGroups, false, true)
		vlog.Infof("init grey clusters success. master cluster url: %s, grey groups: %s", url.ToExtInfo(), greyGroups)
	}

	// cluster selector
	clusterSelectorKey := url.GetParam(motan.ClusterSelectorKey, DefaultClusterSelectorName)
	clusterSelector := extFactory.GetClusterSelector(clusterSelectorKey)
	if clusterSelector == nil {
		vlog.Warningf("cluster selector %s not found, use default clusterSelector replace", clusterSelectorKey)
		clusterSelector = extFactory.GetClusterSelector(DefaultClusterSelectorName)
	}
	clusterSelector.Init(clusterGroup)
	clusterGroup.clusterSelector = clusterSelector

	// backupSwitcher
	clusterGroup.backupSwitcher = motan.GetSwitcherManager().GetOrRegister(BackupClusterSwitcherKey, true)
	return clusterGroup
}

func (c *ClusterGroup) GetContext() *motan.Context {
	return c.context
}

func (c *ClusterGroup) GetRuntimeInfo() map[string]interface{} {
	info := map[string]interface{}{}
	info["master-"+c.masterCluster.GetIdentity()] = c.masterCluster.GetRuntimeInfo()
	for _, cluster := range c.sandboxClusters {
		info["sandbox-"+cluster.GetIdentity()] = cluster.GetRuntimeInfo()
	}
	for _, cluster := range c.backupClusters {
		info["backup-"+cluster.GetIdentity()] = cluster.GetRuntimeInfo()
	}
	for _, cluster := range c.greyClusters {
		info["grey-"+cluster.GetIdentity()] = cluster.GetRuntimeInfo()
	}
	return info
}

func (c *ClusterGroup) GetURL() *motan.URL {
	return c.url
}

func (c *ClusterGroup) SetURL(url *motan.URL) {
	c.url = url
}

func (c *ClusterGroup) IsAvailable() bool {
	return c.masterCluster.IsAvailable()
}

func (c *ClusterGroup) Call(request motan.Request) (res motan.Response) {
	defer motan.HandlePanic(func() {
		res = motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "cluster group call panic", ErrType: motan.ServiceException})
		vlog.Errorf("cluster group call panic. req:%s", motan.GetReqInfo(request))
	})
	cluster := c.clusterSelector.Select(request)
	res = cluster.Call(request)
	if res.GetException() != nil && c.backupSwitcher.IsOpen() && len(c.backupClusters) > 0 &&
		strings.Contains(res.GetException().ErrMsg, motan.NoRefersForRequestPrefix) {
		nextIndex := atomic.AddUint32(&c.backupIndex, 1) % uint32(len(c.backupClusters))
		cluster = c.backupClusters[nextIndex]
		res = cluster.Call(request)
	}
	return
}

func (c *ClusterGroup) Destroy() {
	c.masterCluster.Destroy()
	for _, cluster := range c.sandboxClusters {
		cluster.Destroy()
	}
	for _, cluster := range c.sandboxClusters {
		cluster.Destroy()
	}
	for _, cluster := range c.backupClusters {
		cluster.Destroy()
	}
	for _, cluster := range c.greyClusters {
		cluster.Destroy()
	}
	c.clusterSelector.Destroy()
}

func (c *ClusterGroup) GetMasterCluster() motan.Cluster {
	return c.masterCluster
}

func (c *ClusterGroup) GetSandboxClusters() []motan.Cluster {
	return c.sandboxClusters
}

func (c *ClusterGroup) GetBackupClusters() []motan.Cluster {
	return c.backupClusters
}

func (c *ClusterGroup) GetGreyClusters() []motan.Cluster {
	return c.greyClusters
}

func (c *ClusterGroup) SetRefersFilter(rf motan.RefersFilter) {
	c.masterCluster.SetRefersFilter(rf)
	for _, cluster := range c.sandboxClusters {
		cluster.SetRefersFilter(rf)
	}
	for _, cluster := range c.backupClusters {
		cluster.SetRefersFilter(rf)
	}
}

func createMultiClusters(context *motan.Context, extFactory motan.ExtensionFactory, url *motan.URL, proxy bool, groupsStr string, lazyInit bool, emptyNodeNotify bool) []motan.Cluster {
	var clusters []motan.Cluster
	groups := motan.TrimSplitSet(groupsStr, motan.GroupNameSeparator)
	for group := range groups {
		if strings.HasPrefix(group, motan.GroupSuffixString) {
			group = group[len(motan.GroupSuffixString):]
			// if the url.Group end with the group, skip it
			if strings.HasSuffix(url.Group, group) {
				continue
			}
			group = url.Group + group
		}
		// if the group same as url.Group, skip it
		if group == url.Group {
			continue
		}
		newUrl := url.Copy()
		newUrl.PutParam(motan.LazyInit, strconv.FormatBool(proxy))
		newUrl.PutParam(motan.ClusterEmptyNodeNotifyKey, strconv.FormatBool(emptyNodeNotify))
		newUrl.Group = group
		cluster := NewCluster(context, extFactory, newUrl, proxy)
		clusters = append(clusters, cluster)
	}
	return clusters
}
