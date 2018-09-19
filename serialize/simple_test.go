package serialize

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	motan "github.com/weibocom/motan-go/core"
)

// serialize && deserialize string
func TestSerializeString(t *testing.T) {
	simple := &SimpleSerialization{}
	verifyString("teststring", simple, t)
	verifyString("t", simple, t)
	verifyString("", simple, t)
}

func TestSerializeStringMap(t *testing.T) {
	simple := &SimpleSerialization{}
	var m map[string]string
	verifyMap(m, simple, t)
	m = make(map[string]string, 16)
	m["k1"] = "v1"
	m["k2"] = "v2"
	verifyMap(m, simple, t)
}

func TestSerializeMap(t *testing.T) {
	simple := &SimpleSerialization{}
	value := make([]interface{}, 0, 16)
	var m map[interface{}]interface{}
	m = make(map[interface{}]interface{}, 16)
	var ik, iv int64 = 123, 456 // must use int64 for value check

	m["k1"] = "v1"
	m["k2"] = "v2"
	m[ik] = iv
	m[true] = false

	a := make([]interface{}, 0, 16)
	a = append(a, "test")
	a = append(a, "asdf")
	m["sarray"] = a

	value = append(value, m)
	value = append(value, 3.1415)
	bytes, err := simple.SerializeMulti(value)
	if err != nil {
		t.Errorf("serialize multi map fail. err:%v\n", err)
	}
	nvalue, err := simple.DeSerializeMulti(bytes, nil)
	if err != nil {
		t.Errorf("deserialize multi map fail. nvalue:%v, err:%v\n", nvalue, err)
	}
	if len(value) != len(nvalue) {
		t.Errorf("deserialize multi map fail. nvalue:%v, err:%v\n", nvalue, err)
	}
	nmap := nvalue[0].(map[interface{}]interface{})
	for k, v := range nmap {
		if sa, ok := v.([]interface{}); ok {
			ra := m[k].([]interface{})
			for i, st := range sa {
				if ra[i] != st {
					t.Errorf("deserialize multi map fail. k: %+v, v:%+v, nv:%+v\n", k, m[k], v)
				}
			}
		} else {
			if m[k] != v {
				t.Errorf("deserialize multi map fail. k: %+v, v:%+v, nv:%+v\n", k, m[k], v)
			}
		}

	}
}

func TestSerializeArray(t *testing.T) {
	simple := &SimpleSerialization{}
	// string array
	value := make([]interface{}, 0, 16)
	sa := make([]string, 0, 16)
	for i := 0; i < 20; i++ {
		sa = append(sa, "slkje"+strconv.Itoa(i))
	}

	value = append(value, sa)
	bytes, err := simple.SerializeMulti(value)

	if err != nil {
		t.Errorf("serialize array fail. err:%v\n", err)
	}
	nvalue, err := simple.DeSerializeMulti(bytes, nil)
	if err != nil {
		t.Errorf("deserialize array fail. nvalue:%v, err:%v\n", nvalue, err)
	}
	rsa := value[0].([]string)
	if len(rsa) != len(sa) {
		t.Errorf("deserialize array fail. nvalue:%v, err:%v\n", nvalue, err)
	}
	for i, ts := range sa {
		if rsa[i] != ts {
			t.Errorf("deserialize array fail. sa:%v, rsa:%v\n", sa, rsa)
		}
	}

	//interface{} array
	a := make([]interface{}, 0, 16)
	var m map[interface{}]interface{}
	m = make(map[interface{}]interface{}, 16)
	var ik, iv int64 = 123, 456 // must use int64 for value check

	m["k1"] = "v1"
	m["k2"] = "v2"
	m[ik] = iv
	m[true] = false

	a = append(a, "test")
	a = append(a, "asdf")
	a = append(a, m)
	a = append(a, 3.1415)
	value = make([]interface{}, 0, 16)
	value = append(value, a)

	bytes, err = simple.SerializeMulti(value)
	if err != nil {
		t.Errorf("serialize array fail. err:%v\n", err)
	}
	nvalue, err = simple.DeSerializeMulti(bytes, nil)
	if err != nil {
		t.Errorf("deserialize array fail. nvalue:%v, err:%v\n", nvalue, err)
	}
	ria := value[0].([]interface{})
	if len(ria) != len(a) {
		t.Errorf("deserialize array fail. nvalue:%v, err:%v\n", nvalue, err)
	}
	for i, ts := range a {
		if im, ok := ts.(map[interface{}]interface{}); ok {
			for mk, mv := range m {
				if im[mk] != mv {
					t.Errorf("deserialize array fail. m: %+v, im:%+v\n", m, im)
				}
			}
		} else {
			if ria[i] != ts {
				t.Errorf("deserialize array fail. sa:%v, rsa:%v\n", sa, rsa)
			}
		}

	}
}

func TestSerializeBytes(t *testing.T) {
	simple := &SimpleSerialization{}
	var ba []byte
	verifyBytes(ba, simple, t)
	ba = make([]byte, 2, 2)
	verifyBytes(ba, simple, t)
	ba = make([]byte, 0, 2)
	verifyBytes(ba, simple, t)
	ba = append(ba, 3)
	ba = append(ba, '3')
	ba = append(ba, 'x')
	ba = append(ba, 0x12)
	verifyBytes(ba, simple, t)
}

func TestSerializeNil(t *testing.T) {
	simple := &SimpleSerialization{}
	var test error
	b, err := simple.Serialize(test)
	if err != nil {
		t.Errorf("serialize nil fail. err:%v\n", err)
	}
	if len(b) != 1 && b[0] != 0 {
		t.Errorf("serialize nil fail. b:%v\n", b)
	}
	var ntest *error
	_, err = simple.DeSerialize(b, ntest)
	if err != nil || ntest != nil {
		t.Errorf("serialize nil fail. err:%v, test:%v\n", err, test)
	}
}

func TestSerializeMulti(t *testing.T) {
	//single value
	simple := &SimpleSerialization{}
	var rs string
	verifySingleValue("string", &rs, simple, t)
	m := make(map[string]string, 16)
	m["k"] = "v"
	var rm map[string]string
	verifySingleValue(m, &rm, simple, t)
	verifySingleValue(nil, nil, simple, t)
	b := []byte{1, 2, 3}
	var rb []byte
	verifySingleValue(b, &rb, simple, t)

	//multi value
	a := []interface{}{"stringxx", m, b, nil}
	r := []interface{}{&rs, &rm, &rb, nil}
	verifyMulti(a, r, simple, t)

}

func TestSerializeBaseType(t *testing.T) {
	simple := &SimpleSerialization{}
	// bool
	verifyBaseType(true, simple, t)
	verifyBaseType(false, simple, t)
	//byte
	verifyBaseType(byte(16), simple, t)
	verifyBaseType(byte(0), simple, t)
	verifyBaseType(byte(255), simple, t)
	// int16
	verifyBaseType(int16(-16), simple, t)
	verifyBaseType(int16(0), simple, t)
	//int32
	verifyBaseType(int32(-16), simple, t)
	verifyBaseType(int32(0), simple, t)
	//int
	verifyBaseType(int(-16), simple, t)
	verifyBaseType(int(0), simple, t)
	//int64
	verifyBaseType(int64(-16), simple, t)
	verifyBaseType(int64(0), simple, t)
	//float32
	verifyBaseType(float32(3.141592653), simple, t)
	verifyBaseType(float32(-3.141592653), simple, t)
	verifyBaseType(float32(0), simple, t)
	//float64
	verifyBaseType(float64(3.141592653), simple, t)
	verifyBaseType(float64(-3.141592653), simple, t)
	verifyBaseType(float64(0), simple, t)
}

func verifyBaseType(v interface{}, s motan.Serialization, t *testing.T) {
	sv, err := s.Serialize(v)
	if err != nil || len(sv) == 0 {
		t.Errorf("serialize fail. byte size:%d, err:%v\n", len(sv), err)
	}
	dv, err := s.DeSerialize(sv, reflect.TypeOf(v))
	// int should cast to int64; uint should cast to uint64
	if iv, ok := v.(int); ok {
		v = int64(iv)
	} else if uv, ok := v.(uint); ok {
		v = uint64(uv)
	}
	if err != nil {
		t.Errorf("serialize fail. err:%v\n", err)
	} else if v != dv {
		t.Errorf("deserialize value not correct. result:%v(%v), %v(%v)\n", reflect.TypeOf(v), v, reflect.TypeOf(dv), dv)
	}
}

func verifySingleValue(i interface{}, reply interface{}, simple *SimpleSerialization, t *testing.T) {
	a := []interface{}{i}
	b, err := simple.SerializeMulti(a)
	if err != nil {
		t.Errorf("serialize multi fail. err:%v\n", err)
	}
	if len(b) < 1 {
		t.Errorf("serialize multi fail. b:%v\n", b)
	}
	na := []interface{}{reply}
	v, err := simple.DeSerializeMulti(b, na)
	fmt.Printf("format:%+v\n", v[0])
	if err != nil {
		t.Errorf("serialize multi fail. err:%v\n", err)
	}

	if len(na) != 1 {
		t.Errorf("serialize multi fail. a:%v, na:%v\n", a, na)
	}
}

func verifyMulti(v []interface{}, reply []interface{}, simple *SimpleSerialization, t *testing.T) {
	b, err := simple.SerializeMulti(v)
	if err != nil {
		t.Errorf("serialize multi fail. err:%v\n", err)
	}
	if len(b) < 1 {
		t.Errorf("serialize multi fail. b:%v\n", b)
	}

	result, err := simple.DeSerializeMulti(b, reply)
	fmt.Printf("format:%+v\n", result)
	if err != nil {
		t.Errorf("serialize multi fail. err:%v\n", err)
	}
	if len(reply) != len(v) {
		t.Errorf("serialize multi fail. len:%d\n", len(reply))
	}

}

func verifyString(s string, simple *SimpleSerialization, t *testing.T) {
	b, err := simple.Serialize(s)
	if err != nil {
		t.Errorf("serialize string fail. err:%v\n", err)
	}
	if b[0] != 1 {
		t.Errorf("serialize string fail. b:%v\n", b)
	}
	var ns string
	_, err = simple.DeSerialize(b, &ns)
	if err != nil {
		t.Errorf("serialize string fail. err:%v\n", err)
	}
	if ns != s {
		t.Errorf("serialize string fail. s:%s, ns:%s\n", s, ns)
	}
}

func verifyMap(m map[string]string, simple *SimpleSerialization, t *testing.T) {
	b, err := simple.Serialize(m)
	if err != nil {
		t.Errorf("serialize Map fail. err:%v\n", err)
	}
	if b[0] != 2 {
		t.Errorf("serialize Map fail. b:%v\n", b)
	}
	nm := make(map[string]string, 16)
	_, err = simple.DeSerialize(b, &nm)
	if err != nil {
		t.Errorf("serialize map fail. err:%v\n", err)
	}
	if len(nm) != len(m) {
		t.Errorf("serialize map fail. m:%s, nm:%s\n", m, nm)
	}
	for k, v := range nm {
		if v != m[k] {
			t.Errorf("serialize map value fail. m:%s, nm:%s\n", m, nm)
		}
	}
}

func verifyBytes(ba []byte, simple *SimpleSerialization, t *testing.T) {
	b, err := simple.Serialize(ba)
	if err != nil {
		t.Errorf("serialize []byte fail. err:%v\n", err)
	}
	if b[0] != 3 {
		t.Errorf("serialize []byte fail. b:%v\n", b)
	}
	nba := make([]byte, 0, 1024)
	_, err = simple.DeSerialize(b, &nba)
	if err != nil {
		t.Errorf("serialize []byte fail. err:%v\n", err)
	}
	if len(nba) != len(ba) {
		t.Errorf("serialize []byte fail. ba:%v, nba:%v\n", ba, nba)
	}
	for i, u := range nba {
		if u != ba[i] {
			t.Errorf("serialize []byte value fail. ba:%v, nba:%v\n", ba, nba)
		}
	}
}
