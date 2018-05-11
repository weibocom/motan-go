package serialize

import (
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/weibocom/motan-go/log"
	"reflect"
)

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

type PbSerialization struct{}

func (p *PbSerialization) GetSerialNum() int {
	return 5
}

func (p *PbSerialization) Serialize(v interface{}) ([]byte, error) {
	buf := proto.NewBuffer(nil)
	err := p.serializeBuf(buf, v)
	return buf.Bytes(), err
}

func (p *PbSerialization) serializeBuf(buf *proto.Buffer, v interface{}) error {
	if v == nil {
		buf.EncodeVarint(1)
		return nil
	}
	buf.EncodeVarint(0)
	if message, ok := v.(proto.Message); ok {
		buf.Marshal(message)
		return nil
	}
	var err error = nil
	rv := getReflectValue(v)
	fmt.Println("Se--TypeOf:", reflect.TypeOf(v).Kind(), "Type:", rv.Type(), "Kind:", rv.Kind())
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
		err = buf.EncodeFixed32(uint64(rv.Float()))
	case reflect.Float64:
		err = buf.EncodeFixed64(uint64(rv.Float()))
	case reflect.String:
		err = buf.EncodeStringBytes(rv.String())
	default:
		if rv.Type().String() == "[]uint8" {
			err = buf.EncodeRawBytes(rv.Bytes())
		} else {
			err = errors.New("not support serialize type : " + rv.Type().String())
		}
	}
	if err != nil {
		vlog.Errorln(err)
		fmt.Println(err)
		return err
	}
	return nil
}

func (p *PbSerialization) DeSerialize(b []byte, v interface{}) (interface{}, error) {
	buf := proto.NewBuffer(b)
	return p.deSerializeBuf(buf, v)
}

func (p *PbSerialization) deSerializeBuf(buf *proto.Buffer, v interface{}) (interface{}, error) {
	var temp interface{}
	i, err := buf.DecodeVarint()
	if err == nil {
		if i == 1 {
			return nil, nil
		}
		if message, ok := v.(proto.Message); ok {
			err = buf.Unmarshal(message)
			return message, err
		}
		rv := getReflectValue(v)
		k := rv.Kind()
		if k == reflect.Interface {
			rv = reflect.ValueOf(rv.Interface())
			k = rv.Kind()
		}
		fmt.Println("De--", v, "TypeOf:", reflect.TypeOf(v).Elem().Kind(), "Type:", rv.Type(), "Kind:", rv.Kind(), "k:", k)

		k = reflect.String //test

		switch k {
		case reflect.Bool:
			temp, err = buf.DecodeVarint()
			if err == nil {
				if temp == 1 {
					v = true
				} else {
					v = false
				}
			}
		case reflect.Int32, reflect.Int16:
			temp, err = buf.DecodeZigzag32()
			v = temp.(int32)
		case reflect.Uint32, reflect.Uint16:
			temp, err = buf.DecodeZigzag32()
			v = temp.(uint32)
		case reflect.Int, reflect.Int64:
			temp, err = buf.DecodeZigzag64()
			v = temp.(int)
		case reflect.Uint, reflect.Uint64:
			temp, err = buf.DecodeZigzag64()
			v = temp.(uint)
		case reflect.Float32:
			temp, err = buf.DecodeFixed32()
			v = temp.(float32)
		case reflect.Float64:
			temp, err = buf.DecodeFixed64()
			v = temp.(float64)
		case reflect.String:
			temp, err = buf.DecodeStringBytes()
			v = temp.(string)
		default:
			if rv.Type().String() == "[]uint8" {
				v, err = buf.DecodeRawBytes(true)
			} else {
				err = errors.New("not support deserialize type : " + rv.Type().String())
			}
		}
	}
	if err != nil {
		vlog.Errorln(err)
		fmt.Println(err)
		return nil, err
	}
	return v, nil
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
