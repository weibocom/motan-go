package serialize

import (
	motan "github.com/weibocom/motan-go/core"
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
	rInt := 646464
	seInt, err := s.Serialize(rInt)
	if err != nil || len(seInt) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(seInt), err)
	}
	var resInt int
	deInt, err := s.DeSerialize(seInt, reflect.TypeOf(resInt))
	if err != nil {
		t.Errorf("serialize fail. err:%v\n", err)
	} else if deInt != rInt {
		t.Errorf("deserialize value not correct. result:%v\n", rInt)
	}

	//type: uint, uint64
	var rUint uint = 464646
	seUint, err := s.Serialize(rUint)
	if err != nil || len(seUint) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(seUint), err)
	}
	var resUint uint
	deUint, err := s.DeSerialize(seUint, reflect.TypeOf(resUint))
	if err != nil {
		t.Errorf("serialize fail. err:%v\n", err)
	} else if deUint != rUint {
		t.Errorf("deserialize value not correct. result:%v\n", rUint)
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

	////type: []uint8
	//var rUint8 []uint8 = []uint8(11)
	//seString, err := s.Serialize(rString)
	//if err != nil || len(seString) == 0 {
	//	t.Errorf("serialize fail. byte size:%d, err:%v\n", len(seString), err)
	//}
	//var resString string
	//deString, err := s.DeSerialize(seString, reflect.TypeOf(resString))
	//if err != nil {
	//	t.Errorf("serialize fail. err:%v\n", err)
	//} else if rString != deString {
	//	t.Errorf("deserialize value not correct. result:%v\n", rString)
	//}
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
