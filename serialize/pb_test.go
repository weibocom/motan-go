package serialize

import (
	"fmt"
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

	//type: int
	rInt := 111
	bInt, err := s.Serialize(rInt)
	if err != nil || len(bInt) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(bInt), err)
	}
	var resInt int
	deInt, err := s.DeSerialize(bInt, reflect.TypeOf(resInt))
	fmt.Println("******* Result:", deInt)

	rFloat := 1.23
	bFloat, err := s.Serialize(rFloat)
	if err != nil || len(bFloat) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(bFloat), err)
	}
	var resFLoat float32
	deFloat, err := s.DeSerialize(bFloat, reflect.TypeOf(resFLoat))
	fmt.Println("******* Result:", deFloat)
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
