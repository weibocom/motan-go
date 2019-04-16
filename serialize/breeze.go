package serialize

import (
	"github.com/weibreeze/breeze-go"
)

type BreezeSerialization struct {
}

func (b *BreezeSerialization) GetSerialNum() int {
	return BreezeNumber
}

func (b *BreezeSerialization) Serialize(v interface{}) ([]byte, error) {
	buf := breeze.NewBuffer(256)
	err := breeze.WriteValue(buf, v)
	return buf.Bytes(), err
}

func (b *BreezeSerialization) DeSerialize(bytes []byte, v interface{}) (interface{}, error) {
	buf := breeze.CreateBuffer(bytes)
	return breeze.ReadValue(buf, v)
}

func (b *BreezeSerialization) SerializeMulti(v []interface{}) ([]byte, error) {
	buf := breeze.NewBuffer(256)
	for _, sv := range v {
		err := breeze.WriteValue(buf, sv)
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (b *BreezeSerialization) DeSerializeMulti(bytes []byte, v []interface{}) ([]interface{}, error) {
	buf := breeze.CreateBuffer(bytes)
	ret := make([]interface{}, len(v), len(v))
	for i, sv := range v {
		rv, err := breeze.ReadValue(buf, sv)
		if err != nil {
			return nil, err
		}
		ret[i] = rv
	}
	return ret, nil
}
