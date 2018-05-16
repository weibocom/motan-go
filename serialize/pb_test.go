package serialize

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	motan "github.com/weibocom/motan-go/core"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"math"
	"reflect"
	"testing"
)

// ------- grpc-pb --------
func TestGrpcPbSerialization_Serialize(t *testing.T) {
	s := &GrpcPbSerialization{}
	testPbSerialize(s, t)
}

func TestGrpcPbSerialization_SerializeMulti(t *testing.T) {
	s := &GrpcPbSerialization{}
	testPbSerializeMulti(s, false, t)
}

func testPbSerialize(s motan.Serialization, t *testing.T) {
	r := &HelloRequest{Name: "ray"}
	b, err := s.Serialize(r)
	if err != nil || len(b) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(b), err)
	}

	var r2 HelloRequest
	r3, err := s.DeSerialize(b, &r2)
	if err != nil || r2.Name != "ray" {
		t.Errorf("deserialize value not correct. result:%v, err:%v,\n", r2, err)
	}
	if req, ok := r3.(*HelloRequest); !ok {
		t.Errorf("deserialize not correct. result:%v, err:%v,\n", r3, err)
	} else if req.Name != "ray" {
		t.Errorf("deserialize value not correct. result:%v\n", r3)
	}

}

func testPbSerializeMulti(s motan.Serialization, testMulti bool, t *testing.T) {
	v := []interface{}{&HelloRequest{Name: "ray"}}
	b, err := s.SerializeMulti(v)
	if err != nil || len(b) == 1 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(b), err)
	}

	var r2 HelloRequest

	r, err := s.DeSerializeMulti(b, []interface{}{&r2})
	if err != nil || len(r) != 1 || r2.Name != "ray" {
		t.Errorf("deserialize multi value not correct. result:%v, r2:%v, err:%v,\n", r, r2, err)
	}

	if req, ok := r[0].(*HelloRequest); !ok {
		t.Errorf("deserialize not correct. result:%v, err:%v,\n", r[0], err)
	} else if req.Name != "ray" {
		t.Errorf("deserialize value not correct. result:%v\n", r[0])
	}

	if testMulti { // multi values

	}
}

// ------- pb --------
func TestPbSerialization_Serialize(t *testing.T) {
	s := &PbSerialization{}

	//type: message
	r := &HelloRequest{Name: "ray"}
	b, err := s.Serialize(r)
	if err != nil || len(b) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(b), err)
	}
	var r2 HelloRequest
	r3, err := s.DeSerialize(b, &r2)
	if err != nil || r2.Name != "ray" {
		t.Errorf("deserialize value not correct. result:%v, err:%v,\n", r2, err)
	}
	if req, ok := r3.(*HelloRequest); !ok {
		t.Errorf("deserialize not correct. result:%v, err:%v,\n", r3, err)
	} else if req.Name != "ray" {
		t.Errorf("deserialize value not correct. result:%v\n", r3)
	}

	//type: bool
	var rBool = true
	seBool, err := s.Serialize(rBool)
	if err != nil || len(seBool) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(seBool), err)
	}
	var resBool = false
	deBool, err := s.DeSerialize(seBool, reflect.TypeOf(resBool))
	if err != nil {
		t.Errorf("serialize fail. err:%v\n", err)
	} else if deBool != rBool {
		t.Errorf("deserialize value not correct. result:%v\n", rBool)
	}

	//type: int32
	var rInt32 int32 = 323232
	seInt32, err := s.Serialize(rInt32)
	if err != nil || len(seInt32) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(seInt32), err)
	}
	var resInt32 int32
	deInt32, err := s.DeSerialize(seInt32, reflect.TypeOf(resInt32))
	if err != nil {
		t.Errorf("serialize fail. err:%v\n", err)
	} else if deInt32 != rInt32 {
		t.Errorf("deserialize value not correct. result:%v\n", rInt32)
	}

	//type: uint32
	var rUint32 uint32 = 232323
	seUint32, err := s.Serialize(rUint32)
	if err != nil || len(seUint32) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(seUint32), err)
	}
	var resUint32 uint32
	deUint32, err := s.DeSerialize(seUint32, reflect.TypeOf(resUint32))
	if err != nil {
		t.Errorf("serialize fail. err:%v\n", err)
	} else if deUint32 != rUint32 {
		t.Errorf("deserialize value not correct. result:%v\n", rUint32)
	}

	//type: int, int64
	var rInt int64 = 646464
	seInt, err := s.Serialize(rInt)
	if err != nil || len(seInt) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(seInt), err)
	}
	var resInt int
	deInt, err := s.DeSerialize(seInt, reflect.TypeOf(resInt))
	if err != nil {
		t.Errorf("serialize fail. err:%v\n", err)
	} else if deInt != rInt {
		t.Errorf("deserialize value not correct. result:%v, %v\n", rInt, deInt)
	}

	//type: uint, uint64
	var rUint uint64 = 464646
	seUint, err := s.Serialize(rUint)
	if err != nil || len(seUint) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(seUint), err)
	}
	var resUint uint
	deUint, err := s.DeSerialize(seUint, reflect.TypeOf(resUint))
	if err != nil {
		t.Errorf("serialize fail. err:%v\n", err)
	} else if deUint != rUint {
		t.Errorf("deserialize value not correct. result:%v, %v\n", rUint, deUint)
	}

	//type: float32
	var rFloat32 float32 = 32.3232
	seFloat32, err := s.Serialize(rFloat32)
	if err != nil || len(seFloat32) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(seFloat32), err)
	}
	var resFloat32 float32
	deFloat32, err := s.DeSerialize(seFloat32, reflect.TypeOf(resFloat32))
	if err != nil {
		t.Errorf("serialize fail. err:%v\n", err)
	} else if deFloat32 != rFloat32 {
		t.Errorf("deserialize value not correct. result:%v\n", rFloat32)
	}

	//type: float64
	rFloat64 := 64.6464
	seFloat64, err := s.Serialize(rFloat64)
	if err != nil || len(seFloat64) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(seFloat64), err)
	}
	var resFloat64 float64
	deFloat64, err := s.DeSerialize(seFloat64, reflect.TypeOf(resFloat64))
	if err != nil {
		t.Errorf("serialize fail. err:%v\n", err)
	} else if deFloat64 != rFloat64 {
		t.Errorf("deserialize value not correct. result:%v\n", rFloat64)
	}

	//type: string
	rString := "string type"
	seString, err := s.Serialize(rString)
	if err != nil || len(seString) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(seString), err)
	}
	var resString string
	deString, err := s.DeSerialize(seString, reflect.TypeOf(resString))
	if err != nil {
		t.Errorf("serialize fail. err:%v\n", err)
	} else if rString != deString {
		t.Errorf("deserialize value not correct. result:%v\n", rString)
	}

	//type: []uint8
	rUint8 := []uint8("a")
	seUint8, err := s.Serialize(rUint8)
	if err != nil || len(seString) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(seUint8), err)
	}
	var resUint8 []uint8
	deUint8, err := s.DeSerialize(seUint8, reflect.TypeOf(resUint8))
	if err != nil {
		t.Errorf("serialize fail. err:%v\n", err)
	} else if rUint8[0] != deUint8.([]uint8)[0] {
		t.Errorf("deserialize value not correct. result:%v, %v\n", rUint8, deUint8)
	}

}

func TestPbSerialization_SerializeMulti(t *testing.T) {
	s := &PbSerialization{}
	v := []interface{}{&HelloRequest{Name: "ray"}}
	b, err := s.SerializeMulti(v)
	if err != nil || len(b) == 1 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(b), err)
	}

	var r2 HelloRequest

	r, err := s.DeSerializeMulti(b, []interface{}{&r2})
	if err != nil || len(r) != 1 || r2.Name != "ray" {
		t.Errorf("deserialize multi value not correct. result:%v, r2:%v, err:%v,\n", r, r2, err)
	}

	if req, ok := r[0].(*HelloRequest); !ok {
		t.Errorf("deserialize not correct. result:%v, err:%v,\n", r[0], err)
	} else if req.Name != "ray" {
		t.Errorf("deserialize value not correct. result:%v\n", r[0])
	}
}

// Code generated by protoc-gen-go.
// source: helloworld.proto
// DO NOT EDIT!

/*
Package helloworld is a generated protocol buffer package.

It is generated from these files:
	helloworld.proto

It has these top-level messages:
	HelloRequest
	HelloReply
*/
// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// The request message containing the user's name.
type HelloRequest struct {
	Name string `protobuf:"bytes,1,opt,name=name" json:"name,omitempty"`
}

func (m *HelloRequest) Reset()                    { *m = HelloRequest{} }
func (m *HelloRequest) String() string            { return proto.CompactTextString(m) }
func (*HelloRequest) ProtoMessage()               {}
func (*HelloRequest) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{0} }

// The response message containing the greetings
type HelloReply struct {
	Message string `protobuf:"bytes,1,opt,name=message" json:"message,omitempty"`
}

func (m *HelloReply) Reset()                    { *m = HelloReply{} }
func (m *HelloReply) String() string            { return proto.CompactTextString(m) }
func (*HelloReply) ProtoMessage()               {}
func (*HelloReply) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{1} }

func init() {
	proto.RegisterType((*HelloRequest)(nil), "helloworld.HelloRequest")
	proto.RegisterType((*HelloReply)(nil), "helloworld.HelloReply")
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// Client API for Greeter service

type GreeterClient interface {
	// Sends a greeting
	SayHello(ctx context.Context, in *HelloRequest, opts ...grpc.CallOption) (*HelloReply, error)
}

type greeterClient struct {
	cc *grpc.ClientConn
}

func NewGreeterClient(cc *grpc.ClientConn) GreeterClient {
	return &greeterClient{cc}
}

func (c *greeterClient) SayHello(ctx context.Context, in *HelloRequest, opts ...grpc.CallOption) (*HelloReply, error) {
	out := new(HelloReply)
	err := grpc.Invoke(ctx, "/helloworld.Greeter/SayHello", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Server API for Greeter service

type GreeterServer interface {
	// Sends a greeting
	SayHello(context.Context, *HelloRequest) (*HelloReply, error)
}

func RegisterGreeterServer(s *grpc.Server, srv GreeterServer) {
	s.RegisterService(&_Greeter_serviceDesc, srv)
}

func _Greeter_SayHello_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(HelloRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GreeterServer).SayHello(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/helloworld.Greeter/SayHello",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GreeterServer).SayHello(ctx, req.(*HelloRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Greeter_serviceDesc = grpc.ServiceDesc{
	ServiceName: "helloworld.Greeter",
	HandlerType: (*GreeterServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "SayHello",
			Handler:    _Greeter_SayHello_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "helloworld.proto",
}

func init() { proto.RegisterFile("helloworld.proto", fileDescriptor0) }

var fileDescriptor0 = []byte{
	// 174 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x09, 0x6e, 0x88, 0x02, 0xff, 0xe2, 0x12, 0xc8, 0x48, 0xcd, 0xc9,
	0xc9, 0x2f, 0xcf, 0x2f, 0xca, 0x49, 0xd1, 0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0xe2, 0x42, 0x88,
	0x28, 0x29, 0x71, 0xf1, 0x78, 0x80, 0x78, 0x41, 0xa9, 0x85, 0xa5, 0xa9, 0xc5, 0x25, 0x42, 0x42,
	0x5c, 0x2c, 0x79, 0x89, 0xb9, 0xa9, 0x12, 0x8c, 0x0a, 0x8c, 0x1a, 0x9c, 0x41, 0x60, 0xb6, 0x92,
	0x1a, 0x17, 0x17, 0x54, 0x4d, 0x41, 0x4e, 0xa5, 0x90, 0x04, 0x17, 0x7b, 0x6e, 0x6a, 0x71, 0x71,
	0x62, 0x3a, 0x4c, 0x11, 0x8c, 0x6b, 0xe4, 0xc9, 0xc5, 0xee, 0x5e, 0x94, 0x9a, 0x5a, 0x92, 0x5a,
	0x24, 0x64, 0xc7, 0xc5, 0x11, 0x9c, 0x58, 0x09, 0xd6, 0x25, 0x24, 0xa1, 0x87, 0xe4, 0x02, 0x64,
	0xcb, 0xa4, 0xc4, 0xb0, 0xc8, 0x00, 0xad, 0x50, 0x62, 0x70, 0x32, 0xe0, 0x92, 0xce, 0xcc, 0xd7,
	0x4b, 0x2f, 0x2a, 0x48, 0xd6, 0x4b, 0xad, 0x48, 0xcc, 0x2d, 0xc8, 0x49, 0x2d, 0x46, 0x52, 0xeb,
	0xc4, 0x0f, 0x56, 0x1c, 0x0e, 0x62, 0x07, 0x80, 0xbc, 0x14, 0xc0, 0x98, 0xc4, 0x06, 0xf6, 0x9b,
	0x31, 0x20, 0x00, 0x00, 0xff, 0xff, 0x0f, 0xb7, 0xcd, 0xf2, 0xef, 0x00, 0x00, 0x00,
}
