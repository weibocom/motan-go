package cluster

import (
	motan "github.com/weibocom/motan-go/core"
)

type DefaultClusterSelector struct {
	clusterGroup  motan.ClusterGroup
	routeGroupMap map[string]motan.Cluster
}

func (d *DefaultClusterSelector) Init(clusterGroup motan.ClusterGroup) {
	d.clusterGroup = clusterGroup
	if len(clusterGroup.GetSandboxClusters()) > 0 {
		d.routeGroupMap = make(map[string]motan.Cluster)
		d.routeGroupMap[DefaultSandboxRouteGroup] = clusterGroup.GetSandboxClusters()[0]
		for _, cluster := range clusterGroup.GetSandboxClusters() {
			d.routeGroupMap[cluster.GetURL().Group] = cluster
		}
	}
}

func (d *DefaultClusterSelector) Select(request motan.Request) motan.Cluster {
	if len(d.routeGroupMap) != 0 {
		routeGroup := request.GetAttachment(MRouteGroup)
		cluster, ok := d.routeGroupMap[routeGroup]
		if ok && cluster != nil && len(cluster.GetRefers()) > 0 {
			return cluster.(motan.Cluster)
		}
	}
	return d.clusterGroup.GetMasterCluster()
}

func (d *DefaultClusterSelector) Destroy() {
}
