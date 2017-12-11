package serialize

import (
	motan "github.com/weibocom/motan-go/core"
)

const (
	Simple = "simple"
	Json = "json"
)

func RegistDefaultSerializations(extFactory motan.ExtentionFactory) {
	extFactory.RegistryExtSerialization(Simple, 6, func() motan.Serialization {
		return &SimpleSerialization{}
	})
	extFactory.RegistryExtSerialization(Json, 18, func() motan.Serialization {
		return &JsonSerialization{}
	})
}
