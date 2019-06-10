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
	t := &breeze.TestMsg{MyInt: int32(num), MyString: "jiernoce"}
	t.MyMap = make(map[string]*breeze.TestSubMsg)
	t.MyMap["m1"] = tsm
	t.MyArray = make([]*breeze.TestSubMsg, 0, 12)
	t.MyArray = append(t.MyArray, tsm)
	return t
}

func getTestSubMsg(num int) *breeze.TestSubMsg {
	tsm := &breeze.TestSubMsg{MyString: "uoiwer", MyInt: int32(num * 2), MyInt64: 234, MyFloat32: 23.434, MyFloat64: 8923.234234, MyByte: 5, MyBytes: []byte("ipower"), MyBool: true}
	im1 := make(map[string][]byte, 16)
	im1["jdie"] = []byte("ierjkkkd")
	im1["jddfwwie"] = []byte("ieere9943rjkkkd")
	tsm.MyMap1 = im1
	il := make([]int32, 0, 12)
	il = append(il, 34)
	il = append(il, 56)
	im2 := make(map[int32][]int32, 16)
	im2[12] = il
	im2[3] = []int32{34, 45, 657}
	im2[6] = []int32{23, 66}
	tsm.MyMap2 = im2
	tsm.MyArray = []int32{234, 6456, 234, 6859}
	return tsm
}
