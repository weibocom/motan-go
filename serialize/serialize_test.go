package serialize

import (
	"math"
	"reflect"
	"testing"

	motan "github.com/weibocom/motan-go/core"
)

// base tests for serializations
func CheckSerialeNumber(t *testing.T, serialization motan.Serialization, number int) {
	if serialization.GetSerialNum() != number {
		t.Errorf("wrong breezeSerialization number. expect:%d, real:%d\n", number, serialization.GetSerialNum())
	}
}

func testBasic(t *testing.T, serialization motan.Serialization) {
	tests := []struct {
		name string
		v    interface{}
	}{
		{"bool", true},
		{"bool-2", false},
		{"string", ""},
		{"string-2", "Iyu#%$%&^^*&&()?><?>//n//breezeSerialization//r"},
		{"byte", byte(46)},
		{"byte-2", byte(math.MaxUint8)},
		{"int16", int16(math.MinInt16)},
		{"int16-2", int16(math.MaxInt16)},
		{"int16-3", int16(234)},
		{"int32", int32(234)},
		{"int32-2", int32(math.MinInt32)},
		{"int32-3", int32(math.MaxInt32)},
		{"int64", int64(math.MinInt64)},
		{"int64-2", int64(math.MaxInt64)},
		{"int64-2", int64(23847909845334580)},
		{"float32", float32(3.1415)},
		{"float32-2", float32(math.MaxFloat32)},
		{"float64", float64(234234.534759348534)},
		{"float64-2", float64(math.MaxFloat64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verify(t, serialization, tt.v)
		})
	}
}

func testInt(t *testing.T, serialization motan.Serialization) {
	tests := []struct {
		name string
		v    interface{}
	}{
		{"int", 234745},
		{"int-2", -234897},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verify(t, serialization, tt.v)
		})
	}
}

func testMap(t *testing.T, serialization motan.Serialization) {
	var m = make(map[string]int32, 16)
	m["wjeriew"] = 234
	m[">@#D3"] = 234234
	m["@#>$:P:"] = 98023
	tests := []struct {
		name string
		v    interface{}
	}{
		{"map", m},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verify(t, serialization, tt.v)
		})
	}
}

func testSlice(t *testing.T, serialization motan.Serialization) {
	tests := []struct {
		name string
		v    interface{}
	}{
		{"bytes", []byte("wioejfn//n?><#@)$%(")},
		{"strings", []string{"", "sjie", "erowir23<&*^", "", "23j8"}},
		{"int32s", []int32{23, 3, 4, -435, -234, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verify(t, serialization, tt.v)
		})
	}
}

func testMultiBasic(t *testing.T, serialization motan.Serialization) {
	tests := []struct {
		name string
		v    []interface{}
	}{
		{"multi-basic", []interface{}{int32(123), "ji23l>*)", true, float32(324.343)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifyMultiValue(t, serialization, tt.v)
		})
	}
}

func testMulti(t *testing.T, serialization motan.Serialization) {
	strings := []string{"", "sjie", "erowir23<&*^", "", "23j8"}
	var m = make(map[string]int32, 16)
	m["wjeriew"] = 234
	m[">@#D3"] = 234234
	m["@#>$:P:"] = 98023
	tests := []struct {
		name string
		v    []interface{}
	}{
		{"multi-complex", []interface{}{strings, m, int32(-23), "ji23l>*)", false, float32(-324.987)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifyMultiValue(t, serialization, tt.v)
		})
	}
}

func verify(t *testing.T, serialization motan.Serialization, v interface{}) {
	bytes, err := serialization.Serialize(v)
	if err != nil {
		t.Errorf("error = %v", err)
		return
	}
	tp := reflect.TypeOf(v)
	result, err := serialization.DeSerialize(bytes, tp)
	if err != nil {
		t.Errorf("error = %v", err)
		return
	}
	if !reflect.DeepEqual(result, v) {
		t.Errorf("wrong deserialize value. expect %v, real %v", v, result)
	}
}

func verifyMultiValue(t *testing.T, serialization motan.Serialization, v []interface{}) {
	bytes, err := serialization.SerializeMulti(v)
	if err != nil {
		t.Errorf("error = %v", err)
		return
	}
	tps := make([]interface{}, len(v))
	for i, v := range v {
		tps[i] = reflect.TypeOf(v)
	}
	result, err := serialization.DeSerializeMulti(bytes, tps)
	if err != nil {
		t.Errorf("error = %v", err)
		return
	}
	if !reflect.DeepEqual(result, v) {
		t.Errorf("wrong deserialize value. expect %v, real %v", v, result)
	}
}
