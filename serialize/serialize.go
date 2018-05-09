package serialize

import (
	motan "github.com/weibocom/motan-go/core"
	"reflect"
)

const (
	Simple = "simple"
)

func RegistDefaultSerializations(extFactory motan.ExtentionFactory) {
	extFactory.RegistryExtSerialization(Simple, 6, func() motan.Serialization {
		return &SimpleSerialization{}
	})
}

func getReflectValue(v interface{}) reflect.Value {
	if rv, ok := v.(reflect.Value); ok {
		return rv
	} else {
		return reflect.ValueOf(v)
	}
}
