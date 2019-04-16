package serialize

import (
	motan "github.com/weibocom/motan-go/core"
)

// serialization name
const (
	Simple = "simple"
	Pb     = "protobuf"
	GrpcPb = "grpc-pb"
	Breeze = "breeze"
)

// serialization number in motan2 header
const (
	HessionNumber = iota
	GrpcPbNumber
	JSONNumber
	MsgpackNumber
	HproseNumber
	PbNumber
	SimpleNumber
	GrpcPbJSONNumber
	BreezeNumber
)

func RegistDefaultSerializations(extFactory motan.ExtensionFactory) {
	extFactory.RegistryExtSerialization(Simple, SimpleNumber, func() motan.Serialization {
		return &SimpleSerialization{}
	})
	extFactory.RegistryExtSerialization(Pb, PbNumber, func() motan.Serialization {
		return &PbSerialization{}
	})
	extFactory.RegistryExtSerialization(GrpcPb, GrpcPbNumber, func() motan.Serialization {
		return &GrpcPbSerialization{}
	})
	extFactory.RegistryExtSerialization(Breeze, BreezeNumber, func() motan.Serialization {
		return &BreezeSerialization{}
	})
}
