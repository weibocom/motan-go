package serialize

import (
	"errors"
	motan "github.com/weibocom/motan-go/core"
	"io"
	"math"
	"reflect"
)

// serialize type
const (
	sNull = iota
	sString
	sStringMap
	sByteArray
	sStringArray
	sBool
	sInt8
	sUint8 // byte
	sInt16
	sUint16
	sInt32
	sUint32
	sInt64
	sUint64
	sFloat32
	sFloat64

	// [string]interface{}
	sMap   = 20
	sArray = 21
)

var DefaultBufferSize = 2048

var (
	ErrNotSupport = errors.New("not support type by SimpleSerialization")
	ErrWrongSize  = errors.New("read byte size not correct")
)

type SimpleSerialization struct {
}

func (s *SimpleSerialization) GetSerialNum() int {
	return 6
}

func (s *SimpleSerialization) Serialize(v interface{}) ([]byte, error) {
	buf := motan.NewBytesBuffer(DefaultBufferSize)
	err := serializeBuf(v, buf)
	return buf.Bytes(), err
}

func (s *SimpleSerialization) SerializeMulti(v []interface{}) ([]byte, error) {
	if len(v) == 0 {
		return nil, nil
	}
	buf := motan.NewBytesBuffer(DefaultBufferSize)
	for _, o := range v {
		err := serializeBuf(o, buf)
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func serializeBuf(v interface{}, buf *motan.BytesBuffer) error {
	if v == nil {
		buf.WriteByte(sNull)
		return nil
	}
	var rv reflect.Value
	if nrv, ok := v.(reflect.Value); ok {
		rv = nrv
	} else {
		rv = reflect.ValueOf(v)
	}
	k := rv.Kind()
	if k == reflect.Interface {
		rv = reflect.ValueOf(rv.Interface())
		k = rv.Kind()
	}

	switch k {
	case reflect.String:
		encodeString(rv.String(), buf)
	case reflect.Bool:
		encodeBool(rv.Bool(), buf)
	case reflect.Int8:
		encodeInt8(rv.Int(), buf)
	case reflect.Uint8:
		encodeUint8(rv.Uint(), buf)
	case reflect.Int16:
		encodeInt16(rv.Int(), buf)
	case reflect.Uint16:
		encodeUint16(rv.Uint(), buf)
	case reflect.Int32:
		encodeInt32(rv.Int(), buf)
	case reflect.Uint32:
		encodeUint32(rv.Uint(), buf)
	case reflect.Int, reflect.Int64:
		encodeInt64(rv.Int(), buf)
	case reflect.Uint, reflect.Uint64:
		encodeUint64(rv.Uint(), buf)
	case reflect.Float32:
		encodeFloat32(rv.Float(), buf)
	case reflect.Float64:
		encodeFloat64(rv.Float(), buf)
	case reflect.Slice:
		t := rv.Type().String()
		if t == "[]string" {
			encodeStringArray(rv, buf)
		} else if t == "[]uint8" {
			encodeBytes(rv.Bytes(), buf)
		} else {
			return encodeArray(rv, buf)
		}
	case reflect.Map:
		t := rv.Type().String()
		if "map[string]string" == t {
			encodeStringMap(rv, buf)
		} else {
			return encodeMap(rv, buf)
		}
	default:
		return ErrNotSupport
	}
	return nil
}

func (s *SimpleSerialization) DeSerialize(b []byte, v interface{}) (interface{}, error) {
	if len(b) == 0 {
		return nil, nil
	}
	buf := motan.CreateBytesBuffer(b)
	return deSerializeBuf(buf, v)
}

func (s *SimpleSerialization) DeSerializeMulti(b []byte, v []interface{}) (ret []interface{}, err error) {
	ret = make([]interface{}, 0, len(v))
	buf := motan.CreateBytesBuffer(b)
	if v != nil {
		for _, o := range v {
			rv, err := deSerializeBuf(buf, o)
			if err != nil {
				return nil, err
			}
			ret = append(ret, rv)
		}
	} else {
		for buf.Remain() > 0 {
			rv, err := deSerializeBuf(buf, nil)
			if err != nil {
				if err == io.EOF {
					break
				} else {
					return nil, err
				}
			}
			ret = append(ret, rv)
		}
	}

	return ret, nil
}

func deSerializeBuf(buf *motan.BytesBuffer, v interface{}) (interface{}, error) {
	tp, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	switch int(tp) {
	case sNull:
		return nil, nil
	case sString:
		return decodeString(buf, v)
	case sStringMap:
		return decodeStringMap(buf, v)
	case sByteArray:
		return decodeBytes(buf, v)
	case sStringArray:
		return decodeStringArray(buf, v)
	case sBool:
		return decodeBool(buf, v)
	case sInt8:
		return decodeInt8(buf, v)
	case sUint8:
		return decodeUint8(buf, v)
	case sInt16:
		return decodeInt16(buf, v)
	case sUint16:
		return decodeUint16(buf, v)
	case sInt32:
		return decodeInt32(buf, v)
	case sUint32:
		return decodeUint32(buf, v)
	case sInt64:
		return decodeInt64(buf, v)
	case sUint64:
		return decodeUint64(buf, v)
	case sFloat32:
		return decodeFloat32(buf, v)
	case sFloat64:
		return decodeFloat64(buf, v)
	case sMap:
		return decodeMap(buf, v)
	case sArray:
		return decodeArray(buf, v)
	}
	return nil, ErrNotSupport
}

func encodeString(s string, buf *motan.BytesBuffer) {
	buf.WriteByte(sString)
	encodeStringNoTag(s, buf)
}

func encodeStringNoTag(s string, buf *motan.BytesBuffer) {
	b := []byte(s)
	l := len(b)
	buf.WriteUint32(uint32(l))
	buf.Write(b)
}

func encodeStringMap(v reflect.Value, buf *motan.BytesBuffer) {
	buf.WriteByte(sStringMap)
	pos := buf.GetWPos()
	buf.SetWPos(pos + 4)
	for _, mk := range v.MapKeys() {
		encodeStringNoTag(mk.String(), buf)
		encodeStringNoTag(v.MapIndex(mk).String(), buf)
	}
	npos := buf.GetWPos()
	buf.SetWPos(pos)
	buf.WriteUint32(uint32(npos - pos - 4))
	buf.SetWPos(npos)
}

func encodeBytes(b []byte, buf *motan.BytesBuffer) {
	buf.WriteByte(sByteArray)
	buf.WriteUint32(uint32(len(b)))
	buf.Write(b)
}

func encodeStringArray(v reflect.Value, buf *motan.BytesBuffer) {
	buf.WriteByte(sStringArray)
	pos := buf.GetWPos()
	buf.SetWPos(pos + 4)
	for i := 0; i < v.Len(); i++ {
		encodeStringNoTag(v.Index(i).String(), buf)
	}
	npos := buf.GetWPos()
	buf.SetWPos(pos)
	buf.WriteUint32(uint32(npos - pos - 4))
	buf.SetWPos(npos)
}

func encodeBool(b bool, buf *motan.BytesBuffer) {
	buf.WriteByte(sBool)
	if b {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
}

func encodeMap(v reflect.Value, buf *motan.BytesBuffer) error {
	buf.WriteByte(sMap)
	pos := buf.GetWPos()
	buf.SetWPos(pos + 4)
	var err error
	for _, mk := range v.MapKeys() {
		err = serializeBuf(mk, buf)
		if err != nil {
			return err
		}
		err = serializeBuf(v.MapIndex(mk), buf)
		if err != nil {
			return err
		}
	}
	npos := buf.GetWPos()
	buf.SetWPos(pos)
	buf.WriteUint32(uint32(npos - pos - 4))
	buf.SetWPos(npos)
	return err
}

func encodeArray(v reflect.Value, buf *motan.BytesBuffer) error {
	buf.WriteByte(sArray)
	pos := buf.GetWPos()
	buf.SetWPos(pos + 4)
	var err error
	for i := 0; i < v.Len(); i++ {
		err = serializeBuf(v.Index(i), buf)
		if err != nil {
			return err
		}
	}
	npos := buf.GetWPos()
	buf.SetWPos(pos)
	buf.WriteUint32(uint32(npos - pos - 4))
	buf.SetWPos(npos)
	return nil
}

func encodeInt8(i int64, buf *motan.BytesBuffer) {
	buf.WriteByte(sInt8)
	buf.WriteByte(uint8(i))
}

func encodeUint8(i uint64, buf *motan.BytesBuffer) {
	buf.WriteByte(sUint8)
	buf.WriteByte(uint8(i))
}

func encodeInt16(i int64, buf *motan.BytesBuffer) {
	buf.WriteByte(sInt16)
	buf.WriteUint16(uint16(i))
}

func encodeUint16(u uint64, buf *motan.BytesBuffer) {
	buf.WriteByte(sUint16)
	buf.WriteUint16(uint16(u))
}

func encodeInt32(i int64, buf *motan.BytesBuffer) {
	buf.WriteByte(sInt32)
	buf.WriteZigzag32(uint32(i))
}

func encodeUint32(u uint64, buf *motan.BytesBuffer) {
	buf.WriteByte(sUint32)
	buf.WriteZigzag32(uint32(u))
}

func encodeInt64(i int64, buf *motan.BytesBuffer) {
	buf.WriteByte(sInt64)
	buf.WriteZigzag64(uint64(i))
}

func encodeUint64(u uint64, buf *motan.BytesBuffer) {
	buf.WriteByte(sUint64)
	buf.WriteZigzag64(u)
}

func encodeFloat32(f float64, buf *motan.BytesBuffer) {
	buf.WriteByte(sFloat32)
	buf.WriteUint32(math.Float32bits(float32(f)))
}

func encodeFloat64(f float64, buf *motan.BytesBuffer) {
	buf.WriteByte(sFloat64)
	buf.WriteUint64(math.Float64bits(f))
}

func decodeBool(buf *motan.BytesBuffer, v interface{}) (bool, error) {
	b, err := buf.ReadByte()
	if err != nil {
		return false, err
	}
	var ret bool
	if b == 1 {
		ret = true
	}
	if v != nil {
		if sv, ok := v.(*bool); ok {
			*sv = ret
		}
	}
	return ret, nil
}

func decodeString(buf *motan.BytesBuffer, v interface{}) (string, error) {
	size, err := buf.ReadInt()
	if err != nil {
		return "", err
	}
	b, err := buf.Next(size)
	if err != nil {
		return "", motan.ErrNotEnough
	}
	if v != nil {
		if sv, ok := v.(*string); ok {
			*sv = string(b)
			return *sv, nil
		}
	}
	return string(b), nil
}

func decodeStringMap(buf *motan.BytesBuffer, v interface{}) (map[string]string, error) {
	total, err := buf.ReadInt() // total size
	if err != nil {
		return nil, err
	}
	if total <= 0 {
		return nil, nil
	}
	m := make(map[string]string, 32)
	pos := buf.GetRPos()
	var k, tv string
	for (buf.GetRPos() - pos) < total {
		k, err = decodeString(buf, nil)
		if err != nil {
			return nil, err
		}
		if (buf.GetRPos() - pos) > total {
			return nil, ErrWrongSize
		}
		tv, err = decodeString(buf, nil)
		if err != nil {
			return nil, err
		}
		if (buf.GetRPos() - pos) > total {
			return nil, ErrWrongSize
		}
		m[k] = tv
	}
	if v != nil {
		if mv, ok := v.(*map[string]string); ok {
			*mv = m
		}
	}
	return m, nil
}

func decodeBytes(buf *motan.BytesBuffer, v interface{}) ([]byte, error) {
	size, err := buf.ReadInt()
	if err != nil {
		return nil, err
	}
	b, err := buf.Next(size)
	if err != nil {
		return nil, motan.ErrNotEnough
	}
	if v != nil {
		if bv, ok := v.(*[]byte); ok {
			*bv = b
		}
	}
	return b, nil
}

func decodeMap(buf *motan.BytesBuffer, v interface{}) (map[interface{}]interface{}, error) {
	total, err := buf.ReadInt()
	if err != nil {
		return nil, err
	}
	if total <= 0 {
		return nil, nil
	}
	m := make(map[interface{}]interface{}, 32)
	var k interface{}
	var tv interface{}
	pos := buf.GetRPos()
	for (buf.GetRPos() - pos) < total {
		k, err = deSerializeBuf(buf, nil)
		if err != nil {
			return nil, err
		}
		if (buf.GetRPos() - pos) > total {
			return nil, ErrWrongSize
		}
		tv, err = deSerializeBuf(buf, nil)
		if err != nil {
			return nil, err
		}
		if (buf.GetRPos() - pos) > total {
			return nil, ErrWrongSize
		}
		m[k] = tv
	}
	// TODO set v if v is a pointer
	return m, nil
}

func decodeArray(buf *motan.BytesBuffer, v interface{}) ([]interface{}, error) {
	total, err := buf.ReadInt() // total size
	if err != nil {
		return nil, err
	}
	if total <= 0 {
		return nil, nil
	}
	a := make([]interface{}, 0, 32)
	pos := buf.GetRPos()
	var tv interface{}
	for (buf.GetRPos() - pos) < total {
		tv, err = deSerializeBuf(buf, nil)
		if err != nil {
			return nil, err
		}
		a = append(a, tv)
	}
	if (buf.GetRPos() - pos) != total {
		return nil, ErrWrongSize
	}
	// TODO set v if v is a pointer
	return a, nil
}

func decodeStringArray(buf *motan.BytesBuffer, v interface{}) ([]string, error) {
	total, err := buf.ReadInt() // total size
	if err != nil {
		return nil, err
	}
	if total <= 0 {
		return nil, nil
	}
	a := make([]string, 0, 32)
	pos := buf.GetRPos()
	var tv string
	for (buf.GetRPos() - pos) < total {
		tv, err = decodeString(buf, nil)
		if err != nil {
			return nil, err
		}
		a = append(a, tv)
	}
	if (buf.GetRPos() - pos) != total {
		return nil, ErrWrongSize
	}
	if v != nil {
		if bv, ok := v.(*[]string); ok {
			*bv = a
		}
	}
	return a, nil
}

func decodeInt8(buf *motan.BytesBuffer, v interface{}) (int8, error) {
	b, err := buf.ReadByte()
	if err != nil {
		return 0, err
	}
	if v != nil {
		if bv, ok := v.(*int8); ok {
			*bv = int8(b)
		}
	}
	return int8(b), nil
}

func decodeUint8(buf *motan.BytesBuffer, v interface{}) (uint8, error) {
	b, err := buf.ReadByte()
	if err != nil {
		return 0, err
	}
	if v != nil {
		if bv, ok := v.(*uint8); ok {
			*bv = b
		}
	}
	return b, nil
}

func decodeInt16(buf *motan.BytesBuffer, v interface{}) (int16, error) {
	i, err := buf.ReadUint16()
	if err != nil {
		return 0, err
	}
	if v != nil {
		if bv, ok := v.(*int16); ok {
			*bv = int16(i)
		}
	}
	return int16(i), nil
}

func decodeUint16(buf *motan.BytesBuffer, v interface{}) (uint16, error) {
	i, err := buf.ReadUint16()
	if err != nil {
		return 0, err
	}
	if v != nil {
		if bv, ok := v.(*uint16); ok {
			*bv = i
		}
	}
	return i, nil
}

func decodeInt32(buf *motan.BytesBuffer, v interface{}) (int32, error) {
	i, err := buf.ReadZigzag32()
	if err != nil {
		return 0, err
	}
	if v != nil {
		if bv, ok := v.(*int32); ok {
			*bv = int32(i)
		}
	}
	return int32(i), nil
}

func decodeUint32(buf *motan.BytesBuffer, v interface{}) (uint32, error) {
	i, err := buf.ReadZigzag32()
	if err != nil {
		return 0, err
	}
	if v != nil {
		if bv, ok := v.(*uint32); ok {
			*bv = uint32(i)
		}
	}
	return uint32(i), nil
}

func decodeInt64(buf *motan.BytesBuffer, v interface{}) (int64, error) {
	i, err := buf.ReadZigzag64()
	if err != nil {
		return 0, err
	}
	if v != nil {
		if bv, ok := v.(*int64); ok {
			*bv = int64(i)
		}
		if bv, ok := v.(*int); ok {
			*bv = int(i)
		}
	}
	return int64(i), nil
}

func decodeUint64(buf *motan.BytesBuffer, v interface{}) (uint64, error) {
	i, err := buf.ReadZigzag64()
	if err != nil {
		return 0, err
	}
	if v != nil {
		if bv, ok := v.(*uint64); ok {
			*bv = i
		}
		if bv, ok := v.(*uint); ok {
			*bv = uint(i)
		}
	}
	return i, nil
}

func decodeFloat32(buf *motan.BytesBuffer, v interface{}) (float32, error) {
	i, err := buf.ReadUint32()
	if err != nil {
		return 0, err
	}
	f := math.Float32frombits(i)
	if v != nil {
		if bv, ok := v.(*float32); ok {
			*bv = f
		}
	}
	return f, nil
}

func decodeFloat64(buf *motan.BytesBuffer, v interface{}) (float64, error) {
	i, err := buf.ReadUint64()
	if err != nil {
		return 0, err
	}
	f := math.Float64frombits(i)
	if v != nil {
		if bv, ok := v.(*float64); ok {
			*bv = f
		}
	}
	return f, nil
}
