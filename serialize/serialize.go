package serialize

import (
	motan "github.com/weibocom/motan-go/core"
	"reflect"
)

const (
	Simple = "simple"
	Pb     = "pb"
	GrpcPb = "grpcPb"
)

func RegistDefaultSerializations(extFactory motan.ExtentionFactory) {
	extFactory.RegistryExtSerialization(Simple, 6, func() motan.Serialization {
		return &SimpleSerialization{}
	})
	extFactory.RegistryExtSerialization(Pb, 5, func() motan.Serialization {
		return &PbSerialization{}
	})
	extFactory.RegistryExtSerialization(GrpcPb, 4, func() motan.Serialization {
		return &GrpcPbSerialization{}
	})
}

func getReflectValue(v interface{}) reflect.Value {
	if rv, ok := v.(reflect.Value); ok {
		return rv
	} else {
		return reflect.ValueOf(v)
	}
}
