package serialize

import (
	"bytes"
	"encoding/binary"

	jsoniter "github.com/json-iterator/go"
)

type JsonSerialization struct {
}

func (s *JsonSerialization) GetSerialNum() int {
	return 18
}

func (s *JsonSerialization) Serialize(v interface{}) ([]byte, error) {
	if v == nil {
		return []byte{0}, nil
	}

	return jsoniter.Marshal(v)
}

func (s *JsonSerialization) DeSerialize(b []byte, v interface{}) (interface{}, error) {
	if len(b) == 0 {
		return nil, nil
	}

	err := jsoniter.Unmarshal(b, v)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func (s *JsonSerialization) SerializeMulti(v []interface{}) ([]byte, error) {
	if len(v) == 0 {
		return nil, nil
	} else if len(v) == 1 {
		return s.Serialize(v[0])
	}

	buf := bytes.NewBuffer(nil)
	for _, vv := range v {
		b, err := jsoniter.Marshal(vv)
		if err != nil {
			return nil, err
		}
		l := uint64(len(b))
		err = binary.Write(buf, binary.BigEndian, &l)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}

	return buf.Bytes(), nil
}

func (s *JsonSerialization) DeSerializeMulti(b []byte, v []interface{}) (ret []interface{}, err error) {
	if len(v) == 1 {
		r, err := s.DeSerialize(b, v[0])
		if err != nil {
			return nil, err
		}

		return []interface{}{r}, nil
	}

	buf := bytes.NewBuffer(b)

	for _, vv := range v {
		len := bytes.NewBuffer(buf.Next(8))
		var size uint64
		err := binary.Read(len, binary.BigEndian, &size)

		data := buf.Next(int(size))
		err = jsoniter.Unmarshal(data, vv)
		if err != nil {
			return nil, err
		}
	}

	return v, nil
}
