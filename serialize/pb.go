package serialize

import (
	"errors"
	"github.com/golang/protobuf/proto"
	"github.com/weibocom/motan-go/log"
	"math"
	"reflect"
	"strings"
)

// ------- grpc-pb --------
type GrpcPbSerialization struct{}

func (g *GrpcPbSerialization) GetSerialNum() int {
	return 1
}

func (g *GrpcPbSerialization) Serialize(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, errors.New("param must not nil in GrpcPbSerialization Serialize")
	}
	if message, ok := v.(proto.Message); ok {
		return proto.Marshal(message)
	}
	vlog.Errorf("param must be proto.Message in GrpcPbSerialization Serialize. param:%v\n", v)
	return nil, errors.New("param must be proto.Message in GrpcPbSerialization Serialize")
}

func (g *GrpcPbSerialization) DeSerialize(b []byte, v interface{}) (interface{}, error) {
	if message, ok := v.(proto.Message); ok {
		err := proto.Unmarshal(b, message)
		return message, err
	}
	vlog.Errorf("param must be proto.Message in GrpcPbSerialization DeSerialize. param:%v\n", v)
	return nil, errors.New("param must be proto.Message in GrpcPbSerialization DeSerialize")
}

func (g *GrpcPbSerialization) SerializeMulti(v []interface{}) ([]byte, error) {
	if v == nil {
		return nil, errors.New("param must not nil in GrpcPbSerialization SerializeMulti")
	}
	if len(v) == 1 {
		return g.Serialize(v[0])
	}
	vlog.Errorf("GrpcPbSerialization do not support multi param. param:%v\n", v)
	return nil, errors.New("GrpcPbSerialization do not support multi param")
}

func (g *GrpcPbSerialization) DeSerializeMulti(b []byte, v []interface{}) ([]interface{}, error) {
	if v == nil {
		return nil, errors.New("param must not nil in GrpcPbSerialization SerializeMulti")
	}
	if len(v) == 1 {
		r, err := g.DeSerialize(b, v[0])
		if err != nil {
			return nil, err
		}
		return []interface{}{r}, nil
	}
	vlog.Errorf("GrpcPbSerialization DeSerialize do not support multi param. param:%v\n", v)
	return nil, errors.New("GrpcPbSerialization DeSerialize do not support multi param")
}

// ------- pb --------
type PbSerialization struct{}

func (p *PbSerialization) GetSerialNum() int {
	return 5
}

func (p *PbSerialization) Serialize(v interface{}) ([]byte, error) {
	buf := proto.NewBuffer(nil)
	err := p.serializeBuf(buf, v)
	return buf.Bytes(), err
}

func (p *PbSerialization) serializeBuf(buf *proto.Buffer, v interface{}) (err error) {
	if v == nil {
		buf.EncodeVarint(1)
		return nil
	}
	buf.EncodeVarint(0)
	if message, ok := v.(proto.Message); ok {
		buf.Marshal(message)
		return nil
	}
	var rv reflect.Value
	if rTemp, ok := v.(reflect.Value); ok {
		rv = rTemp
	} else {
		rv = reflect.ValueOf(v)
	}
	switch rv.Kind() {
	case reflect.Bool:
		if rv.Bool() {
			err = buf.EncodeVarint(1)
		} else {
			err = buf.EncodeVarint(0)
		}
	case reflect.Int32, reflect.Int16:
		err = buf.EncodeZigzag32(uint64(rv.Int()))
	case reflect.Uint32, reflect.Uint16:
		err = buf.EncodeZigzag32(rv.Uint())
	case reflect.Int, reflect.Int64:
		err = buf.EncodeZigzag64(uint64(rv.Int()))
	case reflect.Uint, reflect.Uint64:
		err = buf.EncodeZigzag64(rv.Uint())
	case reflect.Float32:
		err = buf.EncodeFixed32(uint64(math.Float32bits(float32(rv.Float()))))
	case reflect.Float64:
		err = buf.EncodeFixed64(math.Float64bits(rv.Float()))
	case reflect.String:
		err = buf.EncodeStringBytes(rv.String())
	case reflect.Uint8:
		err = buf.EncodeVarint(rv.Uint())
	default:
		if pb, ok := rv.Interface().(proto.Message); ok {
			buf.Marshal(pb)
		} else {
			err = errors.New("not support serialize type: " + rv.Type().String())
		}
	}
	if err != nil {
		vlog.Errorln(err)
		return err
	}
	return nil
}

func (p *PbSerialization) DeSerialize(b []byte, v interface{}) (interface{}, error) {
	buf := proto.NewBuffer(b)
	return p.deSerializeBuf(buf, v)
}

type Stringer interface {
	String() string
}

func (p *PbSerialization) deSerializeBuf(buf *proto.Buffer, v interface{}) (interface{}, error) {
	i, err := buf.DecodeVarint()
	if err == nil {
		if i == 1 {
			return nil, nil
		}
		if message, ok := v.(proto.Message); ok {
			err = buf.Unmarshal(message)
			return message, err
		}
		vStr := reflect.TypeOf(v).String()
		if vStr == "*reflect.rtype" {
			vStr = v.(Stringer).String()
		}
		vStr = strings.Replace(vStr, "*", "", -1)
		switch vStr {
		case "bool":
			dv, err := buf.DecodeVarint()
			if err == nil {
				s := false
				if dv == 1 {
					s = true
				}
				if sv, ok := v.(*bool); ok {
					*sv = s
				}
				return s, err
			}
		case "int16":
			dv, err := buf.DecodeZigzag32()
			if sv, ok := v.(*int16); ok {
				*sv = int16(dv)
			}
			return int16(dv), err
		case "uint16":
			dv, err := buf.DecodeZigzag32()
			if sv, ok := v.(*uint16); ok {
				*sv = uint16(dv)
			}
			return uint16(dv), err
		case "int32":
			dv, err := buf.DecodeZigzag32()
			if sv, ok := v.(*int32); ok {
				*sv = int32(dv)
			}
			return int32(dv), err
		case "uint32":
			dv, err := buf.DecodeZigzag32()
			if sv, ok := v.(*uint32); ok {
				*sv = uint32(dv)
			}
			return uint32(dv), err
		case "int", "int64":
			dv, err := buf.DecodeZigzag64()
			if sv, ok := v.(*int64); ok {
				*sv = int64(dv)
			}
			return int64(dv), err
		case "uint", "uint64":
			dv, err := buf.DecodeZigzag64()
			if sv, ok := v.(*uint64); ok {
				*sv = uint64(dv)
			}
			return uint64(dv), err
		case "float32":
			d, err := buf.DecodeFixed32()
			dv := math.Float32frombits(uint32(d))
			if sv, ok := v.(*float32); ok {
				*sv = float32(dv)
			}
			return float32(dv), err
		case "float64":
			d, err := buf.DecodeFixed64()
			dv := math.Float64frombits(d)
			if sv, ok := v.(*float64); ok {
				*sv = float64(dv)
			}
			return float64(dv), err
		case "string":
			dv, err := buf.DecodeStringBytes()
			if sv, ok := v.(*string); ok {
				*sv = string(dv)
			}
			return string(dv), err
		case "uint8":
			dv, err := buf.DecodeVarint()
			if sv, ok := v.(*uint8); ok {
				*sv = uint8(dv)
			}
			return uint8(dv), err
		default:
			typ := proto.MessageType(vStr)
			if typ != nil {
				interf := reflect.New(typ.Elem()).Interface()
				if pb, ok := interf.(proto.Message); ok {
					err = buf.Unmarshal(pb)
					return pb, err
				}
			}
			return nil, errors.New("not support deserialize type: " + vStr)
		}
	}
	return nil, err
}

func (p *PbSerialization) SerializeMulti(v []interface{}) ([]byte, error) {
	buf := proto.NewBuffer(nil)
	var err error
	for _, sv := range v {
		err = p.serializeBuf(buf, sv)
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
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
