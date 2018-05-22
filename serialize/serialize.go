package serialize

import (
	motan "github.com/weibocom/motan-go/core"
)

const (
	Simple = "simple"
	Pb     = "protobuf"
	GrpcPb = "grpc-pb"
)

func RegistDefaultSerializations(extFactory motan.ExtentionFactory) {
	extFactory.RegistryExtSerialization(Simple, 6, func() motan.Serialization {
		return &SimpleSerialization{}
	})
	extFactory.RegistryExtSerialization(Pb, 5, func() motan.Serialization {
		return &PbSerialization{}
	})
	extFactory.RegistryExtSerialization(GrpcPb, 1, func() motan.Serialization {
		return &GrpcPbSerialization{}
	})
}
