package serialize

import (
	"fmt"
	"testing"
)

// serialize && deserialize string
func TestSerializeString(t *testing.T) {
	simple := &SimpleSerialization{}
	verifyString("teststring", simple, t)
	verifyString("t", simple, t)
	verifyString("", simple, t)
}

func TestSerializeMap(t *testing.T) {
	simple := &SimpleSerialization{}
	var m map[string]string
	verifyMap(m, simple, t)
	m = make(map[string]string, 16)
	m["k1"] = "v1"
	m["k2"] = "v2"
	verifyMap(m, simple, t)
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
