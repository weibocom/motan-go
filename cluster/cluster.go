package cluster

import (
	motan "github.com/weibocom/motan-go/core"
)

const (
	MRouteGroup              = "motan-route-group"
	DefaultSandboxRouteGroup = "sandbox"

	DefaultClusterSelectorName = "default"
)

func RegistClusterSelector(extFactory motan.ExtensionFactory) {
	extFactory.RegistryExtClusterSelector(DefaultClusterSelectorName, func() motan.ClusterSelector {
		return &DefaultClusterSelector{}
	})
}
