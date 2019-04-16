package serialize

import (
	"testing"

	"github.com/weibreeze/breeze-go"
)

var breezeSerialization = &BreezeSerialization{}

func TestBreezeSerialization_GetSerialNum(t *testing.T) {
	CheckSerialeNumber(t, breezeSerialization, BreezeNumber)
}

func TestBreezeSerialization_Serialize(t *testing.T) {
	testInt(t, breezeSerialization)
	testBasic(t, breezeSerialization)
	testMap(t, breezeSerialization)
	testSlice(t, breezeSerialization)
}

func TestBreezeSerialization_SerializeMulti(t *testing.T) {
	testMultiBasic(t, breezeSerialization)
	testMulti(t, breezeSerialization)
}

func TestMultiBreezeMessage(t *testing.T) {
	verifyMultiValue(t, breezeSerialization, []interface{}{getTestMsg(12), getTestMsg(234545), 234, true, float64(234.543645)})
}

func TestBreezeMessage(t *testing.T) {
	verify(t, breezeSerialization, getTestMsg(12))
}

func getTestMsg(num int) *breeze.TestMsg {
	tsm := getTestSubMsg(num)
	t := &breeze.TestMsg{I: num, S: "jiernoce"}
	t.M = make(map[string]*breeze.TestSubMsg)
	t.M["m1"] = tsm
	t.A = make([]*breeze.TestSubMsg, 0, 12)
	t.A = append(t.A, tsm)
	return t
}

func getTestSubMsg(num int) *breeze.TestSubMsg {
	tsm := &breeze.TestSubMsg{S: "uoiwer", I: num * 2, I64: 234, F32: 23.434, F64: 8923.234234, Byte: 5, Bytes: []byte("ipower"), B: true}
	im1 := make(map[string][]byte, 16)
	im1["jdie"] = []byte("ierjkkkd")
	im1["jddfwwie"] = []byte("ieere9943rjkkkd")
	tsm.Map1 = im1
	il := make([]interface{}, 0, 12)
	il = append(il, 34)
	il = append(il, 56)
	im2 := make(map[int][]interface{}, 16)
	im2[12] = il
	im2[3] = []interface{}{34, 45, 657}
	im2[6] = []interface{}{23, 66}
	tsm.Map2 = im2
	tsm.List = []int{234, 6456, 234, 6859}
	return tsm
}
