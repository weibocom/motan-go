package serialize

import (
	"errors"
	"io"
	"math"
	"reflect"

	"github.com/golang/protobuf/proto"
)

var (
	ErrMultiParam      = errors.New("do not support multi param in Serialization ")
	ErrNilParam        = errors.New("param must not nil in Serialization")
	ErrNotMessageParam = errors.New("param must be proto.Message in Serialization")
)

// GrpcPbSerialization can serialize & deserialize only single pb message.
type GrpcPbSerialization struct{}

func (g *GrpcPbSerialization) GetSerialNum() int {
	return GrpcPbNumber
}

func (g *GrpcPbSerialization) SerializeMulti(v []interface{}) ([]byte, error) {
	if v == nil {
		return nil, ErrNilParam
	}
	if len(v) == 1 {
		return g.Serialize(v[0])
	}
	return nil, ErrMultiParam
}

func (g *GrpcPbSerialization) Serialize(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, ErrNilParam
	}
	if message, ok := v.(proto.Message); ok {
		return proto.Marshal(message)
	}
	if rv, ok := v.(reflect.Value); ok {
		if message, ok := rv.Interface().(proto.Message); ok {
			return proto.Marshal(message)
		}
	}
	return nil, ErrNotMessageParam
}

func (g *GrpcPbSerialization) DeSerializeMulti(b []byte, v []interface{}) ([]interface{}, error) {
	if v == nil {
		return nil, ErrNilParam
	}
	if len(v) == 1 {
		r, err := g.DeSerialize(b, v[0])
		if err != nil {
			return nil, err
		}
		return []interface{}{r}, nil
	}
	return nil, ErrMultiParam
}

func (g *GrpcPbSerialization) DeSerialize(b []byte, v interface{}) (interface{}, error) {
	if v == nil {
		return nil, ErrNilParam
	}
	if message, ok := v.(proto.Message); ok {
		err := proto.Unmarshal(b, message)
		return message, err
	}
	if rt, ok := v.(reflect.Type); ok {
		if rt.Kind() == reflect.Ptr {
			if message, ok := reflect.New(rt.Elem()).Interface().(proto.Message); ok {
				err := proto.Unmarshal(b, message)
				return message, err
			}
		}
	}
	return nil, ErrNotMessageParam
}

// PbSerialization can serialize & deserialize multi message of pb, primtive type such as int32, bool etc.
type PbSerialization struct{}

func (p *PbSerialization) GetSerialNum() int {
	return PbNumber
}

func (p *PbSerialization) Serialize(v interface{}) ([]byte, error) {
	buf := proto.NewBuffer(nil)
	err := p.serializeBuf(buf, v)
	return buf.Bytes(), err
}

func (p *PbSerialization) SerializeMulti(v []interface{}) ([]byte, error) {
	buf := proto.NewBuffer(nil)
	for _, sv := range v {
		err := p.serializeBuf(buf, sv)
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (p *PbSerialization) serializeBuf(buf *proto.Buffer, v interface{}) (err error) {
	if v == nil {
		buf.EncodeVarint(1)
		return nil
	}
	rv, ok := v.(reflect.Value)
	if !ok {
		rv = reflect.ValueOf(v)
	}
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Invalid {
		buf.EncodeVarint(1)
		return nil
	}
	buf.EncodeVarint(0)
	switch rv.Kind() {
	case reflect.Bool:
		if rv.Bool() {
			return buf.EncodeVarint(1)
		}
		return buf.EncodeVarint(0)
	case reflect.Int32, reflect.Int16:
		return buf.EncodeZigzag32(uint64(rv.Int()))
	case reflect.Uint32, reflect.Uint16:
		return buf.EncodeZigzag32(rv.Uint())
	case reflect.Int, reflect.Int64:
		return buf.EncodeZigzag64(uint64(rv.Int()))
	case reflect.Uint, reflect.Uint64:
		return buf.EncodeZigzag64(rv.Uint())
	case reflect.Float32:
		return buf.EncodeFixed32(uint64(math.Float32bits(float32(rv.Float()))))
	case reflect.Float64:
		return buf.EncodeFixed64(math.Float64bits(rv.Float()))
	case reflect.String:
		return buf.EncodeStringBytes(rv.String())
	case reflect.Uint8:
		return buf.EncodeVarint(rv.Uint())
	case reflect.Slice: //  Experimental: This type only works in golang
		for i := 0; i < rv.Len(); i++ {
			iv := rv.Index(i)
			if _, ok := iv.Interface().(proto.Message); !ok {
				kind := iv.Type().Kind()
				if iv.Type().Kind() == reflect.Ptr {
					kind = iv.Elem().Kind()
				}
				return errors.New("not support other types in pb-array in PbSerialization. type: " + kind.String())
			}
			if err = p.serializeBuf(buf, iv.Elem().Interface()); err != nil {
				return err
			}
		}
		return nil
	default:
		if rv.CanAddr() {
			if pb, ok := rv.Addr().Interface().(proto.Message); ok {
				return buf.EncodeMessage(pb)
			}
		}
		return errors.New("not support serialize type in PbSerialization. type: " + rv.Type().String())
	}
}

func (p *PbSerialization) DeSerialize(b []byte, v interface{}) (interface{}, error) {
	buf := proto.NewBuffer(b)
	return p.deSerializeBuf(buf, v)
}

func (p *PbSerialization) DeSerializeMulti(b []byte, v []interface{}) ([]interface{}, error) {
	ret := make([]interface{}, len(v), len(v))
	buf := proto.NewBuffer(b)
	for i, sv := range v {
		rv, err := p.deSerializeBuf(buf, sv)
		if err != nil {
			return nil, err
		}
		ret[i] = rv
	}
	return ret, nil
}

func (p *PbSerialization) deSerializeBuf(buf *proto.Buffer, v interface{}) (interface{}, error) {
	i, err := buf.DecodeVarint()
	if err != nil {
		return nil, err
	}
	if i == 1 {
		return nil, nil
	}
	if v == nil {
		return nil, ErrNilParam
	}
	rt, ok := v.(reflect.Type)
	if !ok {
		rt = reflect.TypeOf(v)
	}
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	switch rt.Kind() {
	case reflect.Bool:
		dv, err := buf.DecodeVarint()
		s := false
		if err == nil {
			if dv == 1 {
				s = true
			}
			if sv, ok := v.(*bool); ok {
				*sv = s
			}
		}
		return s, err
	case reflect.Int16:
		dv, err := buf.DecodeZigzag32()
		if sv, ok := v.(*int16); ok {
			*sv = int16(dv)
		}
		return int16(dv), err
	case reflect.Uint16:
		dv, err := buf.DecodeZigzag32()
		if sv, ok := v.(*uint16); ok {
			*sv = uint16(dv)
		}
		return uint16(dv), err
	case reflect.Int32:
		dv, err := buf.DecodeZigzag32()
		if sv, ok := v.(*int32); ok {
			*sv = int32(dv)
		}
		return int32(dv), err
	case reflect.Uint32:
		dv, err := buf.DecodeZigzag32()
		if sv, ok := v.(*uint32); ok {
			*sv = uint32(dv)
		}
		return uint32(dv), err
	case reflect.Int, reflect.Int64:
		dv, err := buf.DecodeZigzag64()
		if sv, ok := v.(*int64); ok {
			*sv = int64(dv)
		}
		return int64(dv), err
	case reflect.Uint, reflect.Uint64:
		dv, err := buf.DecodeZigzag64()
		if sv, ok := v.(*uint64); ok {
			*sv = dv
		}
		return dv, err
	case reflect.Float32:
		d, err := buf.DecodeFixed32()
		dv := math.Float32frombits(uint32(d))
		if sv, ok := v.(*float32); ok {
			*sv = dv
		}
		return dv, err
	case reflect.Float64:
		d, err := buf.DecodeFixed64()
		dv := math.Float64frombits(d)
		if sv, ok := v.(*float64); ok {
			*sv = dv
		}
		return dv, err
	case reflect.String:
		dv, err := buf.DecodeStringBytes()
		if sv, ok := v.(*string); ok {
			*sv = dv
		}
		return dv, err
	case reflect.Uint8:
		dv, err := buf.DecodeVarint()
		if sv, ok := v.(*uint8); ok {
			*sv = uint8(dv)
		}
		return uint8(dv), err
	case reflect.Slice: // Experimental: This type only works in golang
		msgArray := reflect.ValueOf(v).Elem()
		rtElem := rt.Elem()
		if rtElem.Kind() != reflect.Ptr {
			return nil, errors.New("not support non-pointer deserialize type in pb-array in PbSerialization. type: " + rtElem.String())
		}
		message := reflect.New(rtElem.Elem()).Interface()
		for {
			if _, err = p.deSerializeBuf(buf, message); err != nil {
				if err == io.ErrUnexpectedEOF {
					break
				}
				return nil, err
			}
			msgArray.Set(reflect.Append(msgArray, reflect.ValueOf(message)))
		}
		return msgArray, nil
	default:
		message, ok := v.(proto.Message)
		if !ok {
			message, ok = reflect.New(rt).Interface().(proto.Message)
		}
		if ok {
			err = buf.DecodeMessage(message)
			return message, err
		}
		return nil, errors.New("not support deserialize type in PbSerialization. type: " + rt.String())
	}
}
