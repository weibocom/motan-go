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
	return buf.Bytes(), p.serializeBuf(buf, v)
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

	rv := getReflectValue(v)
	switch rv.Kind() {
	case reflect.Bool:
		if rv.Bool() {
			buf.EncodeVarint(1)
		} else {
			buf.EncodeVarint(0)
		}
	case reflect.Int32, reflect.Int16:
		buf.EncodeZigzag32(uint64(rv.Int()))
	case reflect.Uint32, reflect.Uint16:
		buf.EncodeZigzag32(rv.Uint())
	case reflect.Int, reflect.Int64:
		buf.EncodeZigzag64(uint64(rv.Int()))
	case reflect.Uint, reflect.Uint64:
		buf.EncodeZigzag64(rv.Uint())
	case reflect.Float32:
		buf.EncodeFixed32(uint64(rv.Float()))
	case reflect.Float64:
		buf.EncodeFixed64(uint64(rv.Float()))
	case reflect.String:
		buf.EncodeStringBytes(rv.String())
	default:
		if rv.Type().String() == "[]uint8" {
			buf.EncodeRawBytes(rv.Bytes())
		} else {
			vlog.Errorf("%v is not support in PbSerialization\n", rv.Type().String())
			return errors.New("not support serialize type : " + rv.Type().String())
		}
	}
	return nil
}

func (p *PbSerialization) DeSerialize(b []byte, v interface{}) (interface{}, error) {
	buf := proto.NewBuffer(b)
	return p.deSerializeBuf(buf, v)
}

func (p *PbSerialization) deSerializeBuf(buf *proto.Buffer, v interface{}) (interface{}, error) {
	i, err := buf.DecodeVarint()
	if err != nil {
		return nil, err
	}
	if i == 1 {
		return nil, nil
	}
	if message, ok := v.(proto.Message); ok {
		err := buf.Unmarshal(message)
		return message, err
	}
	rv := getReflectValue(v)
	switch rv.Kind() {
	case reflect.Bool:
		u, err := buf.DecodeVarint()
		if rv.Bool() {
			buf.de
			buf.EncodeVarint(1)
		} else {
			buf.EncodeVarint(0)
		}
	case reflect.Int32, reflect.Int16:
		buf.EncodeZigzag32(uint64(rv.Int()))
	case reflect.Uint32, reflect.Uint16:
		buf.EncodeZigzag32(rv.Uint())
	case reflect.Int, reflect.Int64:
		buf.EncodeZigzag64(uint64(rv.Int()))
	case reflect.Uint, reflect.Uint64:
		buf.EncodeZigzag64(rv.Uint())
	case reflect.Float32:
		buf.EncodeFixed32(uint64(rv.Float()))
	case reflect.Float64:
		buf.EncodeFixed64(uint64(rv.Float()))
	case reflect.String:
		buf.EncodeStringBytes(rv.String())
	default:
		if rv.Type().String() == "[]uint8" {
			buf.EncodeRawBytes(rv.Bytes())
		} else {
			vlog.Errorf("%v is not support in PbSerialization\n", rv.Type().String())
			return nil, errors.New("not support serialize type : " + rv.Type().String())
		}
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
