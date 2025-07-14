package cluster

import (
	motan "github.com/weibocom/motan-go/core"
	"strings"
)

type DefaultClusterSelector struct {
	clusterGroup          motan.ClusterGroup
	defaultSandboxCluster motan.Cluster
	defaultGreyCluster    motan.Cluster
}

func (d *DefaultClusterSelector) Init(clusterGroup motan.ClusterGroup) {
	d.clusterGroup = clusterGroup
	if len(clusterGroup.GetSandboxClusters()) > 0 {
		d.defaultSandboxCluster = clusterGroup.GetSandboxClusters()[0]
	}
	if len(clusterGroup.GetGreyClusters()) > 0 {
		d.defaultGreyCluster = clusterGroup.GetGreyClusters()[0]
	}
}

func (d *DefaultClusterSelector) Select(request motan.Request) motan.Cluster {
	routeGroup := request.GetAttachment(MRouteGroup)
	if routeGroup != "" {
		var sandboxGroup string
		var greyGroup string
		if d.defaultSandboxCluster != nil && len(d.defaultSandboxCluster.GetRefers()) > 0 {
			sandboxGroup = d.defaultSandboxCluster.GetURL().Group
		}
		if d.defaultGreyCluster != nil && len(d.defaultGreyCluster.GetRefers()) > 0 {
			greyGroup = d.defaultGreyCluster.GetURL().Group
		}
		if sandboxGroup != "" && strings.TrimSpace(routeGroup) == DefaultSandboxRouteGroup {
			return d.defaultSandboxCluster
		}
		routeGroupList := motan.TrimSplit(routeGroup, ",")
		for _, group := range routeGroupList {
			if sandboxGroup == group {
				return d.defaultSandboxCluster
			}
			if greyGroup == group {
				return d.defaultGreyCluster
			}
		}
	}
	return d.clusterGroup.GetMasterCluster()
}

func (d *DefaultClusterSelector) Destroy() {
}
