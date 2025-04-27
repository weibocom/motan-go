package cluster

import (
	motan "github.com/weibocom/motan-go/core"
	"strings"
)

type DefaultClusterSelector struct {
	clusterGroup          motan.ClusterGroup
	defaultSandboxCluster motan.Cluster
}

func (d *DefaultClusterSelector) Init(clusterGroup motan.ClusterGroup) {
	d.clusterGroup = clusterGroup
	if len(clusterGroup.GetSandboxClusters()) > 0 {
		d.defaultSandboxCluster = clusterGroup.GetSandboxClusters()[0]
	}
}

func (d *DefaultClusterSelector) Select(request motan.Request) motan.Cluster {
	if d.defaultSandboxCluster != nil && len(d.defaultSandboxCluster.GetRefers()) > 0 {
		routeGroup := strings.TrimSpace(request.GetAttachment(MRouteGroup))
		if routeGroup == DefaultSandboxRouteGroup {
			return d.defaultSandboxCluster
		}
		routeGroupList := motan.TrimSplit(routeGroup, ",")
		for _, group := range routeGroupList {
			if group == d.clusterGroup.GetURL().Group {
				return d.defaultSandboxCluster
			}
		}
	}
	return d.clusterGroup.GetMasterCluster()
}

func (d *DefaultClusterSelector) Destroy() {
}
