package serialize

import (
	motan "github.com/weibocom/motan-go/core"
)

const (
	Simple = "simple"
)

func RegistDefaultSerializations(extFactory motan.ExtentionFactory) {
	extFactory.RegistryExtSerialization(Simple, 6, func() motan.Serialization {
		return &SimpleSerialization{}
	})
}
