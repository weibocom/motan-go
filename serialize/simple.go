package serialize

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
)

type SimpleSerialization struct {
}

func (s *SimpleSerialization) GetSerialNum() int {
	return 6
}

func (s *SimpleSerialization) Serialize(v interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := s.serializeBuf(v, buf)
	return buf.Bytes(), err
}

func (s *SimpleSerialization) serializeBuf(v interface{}, buf *bytes.Buffer) error {
	if v == nil {
		buf.WriteByte(0)
		return nil
	}
	var rv reflect.Value
	if nrv, ok := v.(reflect.Value); ok {
		rv = nrv
	} else {
		rv = reflect.ValueOf(v)
	}

	t := fmt.Sprintf("%s", rv.Type())

	var err error
	switch t {
	case "string":
		buf.WriteByte(1)
		_, err = encodeString(rv, buf)
	case "map[string]string":
		buf.WriteByte(2)
		err = encodeMap(rv, buf)
	case "[]uint8":
		buf.WriteByte(3)
		err = encodeBytes(rv, buf)
	}
	return err
}

func (s *SimpleSerialization) DeSerialize(b []byte, v interface{}) (interface{}, error) {
	if len(b) == 0 {
		return nil, nil
	}
	buf := bytes.NewBuffer(b)
	return s.deSerializeBuf(buf, v)
}

func (s *SimpleSerialization) deSerializeBuf(buf *bytes.Buffer, v interface{}) (interface{}, error) {
	tp, _ := buf.ReadByte()
	switch tp {
	case 0:
		v = nil
		return nil, nil
	case 1:
		st, _, err := decodeString(buf)
		if err != nil {
			return nil, err
		}
		if v != nil {
			if sv, ok := v.(*string); ok {
				*sv = st
			}
		}
		return st, err
	case 2:
		ma, err := decodeMap(buf)
		if err != nil {
			return nil, err
		}
		if v != nil {
			if mv, ok := v.(*map[string]string); ok {
				*mv = ma
			}
		}
		return ma, err
	case 3:
		by, err := decodeBytes(buf)
		if err != nil {
			return nil, err
		}
		if v != nil {
			if bv, ok := v.(*[]byte); ok {
				*bv = by
			}
		}
		return by, err
	}
	return nil, fmt.Errorf("can not deserialize. unknown type:%v", tp)
}

func (s *SimpleSerialization) SerializeMulti(v []interface{}) ([]byte, error) {
	if len(v) == 0 {
		return nil, nil
	}
	buf := new(bytes.Buffer)
	for _, o := range v {
		err := s.serializeBuf(o, buf)
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (s *SimpleSerialization) DeSerializeMulti(b []byte, v []interface{}) (ret []interface{}, err error) {
	ret = make([]interface{}, 0, len(v))
	buf := bytes.NewBuffer(b)
	for _, o := range v {
		rv, err := s.deSerializeBuf(buf, o)
		if err != nil {
			return nil, err
		}
		ret = append(ret, rv)
	}
	return ret, nil
}

func readInt32(buf *bytes.Buffer) (int, error) {
	var i int32
	err := binary.Read(buf, binary.BigEndian, &i)
	if err != nil {
		return 0, err
	}
	return int(i), nil
}

func decodeString(buf *bytes.Buffer) (string, int, error) {
	size, err := readInt32(buf)
	if err != nil {
		return "", 0, err
	}
	b := buf.Next(size)
	if len(b) != size {
		return "", 0, errors.New("read byte not enough")
	}

	return string(b), size + 4, nil
}

func decodeMap(buf *bytes.Buffer) (map[string]string, error) {
	total, err := readInt32(buf) // total size
	if err != nil {
		return nil, err
	}
	if total <= 0 {
		return nil, nil
	}
	m := make(map[string]string, 32)
	size := 0
	var k, v string
	var l int
	for size < total {
		k, l, err = decodeString(buf)
		if err != nil {
			return nil, err
		}
		size += l
		if size > total {
			return nil, errors.New("read byte size not correct")
		}
		v, l, err = decodeString(buf)
		if err != nil {
			return nil, err
		}
		size += l
		if size > total {
			return nil, errors.New("read byte size not correct")
		}
		m[k] = v
	}
	return m, nil
}

func decodeBytes(buf *bytes.Buffer) ([]byte, error) {
	size, err := readInt32(buf)
	if err != nil {
		return nil, err
	}
	b := buf.Next(size)
	if len(b) != size {
		return nil, errors.New("read byte not enough")
	}

	return b, nil
}

func encodeString(v reflect.Value, buf *bytes.Buffer) (int32, error) {
	b := []byte(v.String())
	l := int32(len(b))
	err := binary.Write(buf, binary.BigEndian, l)
	err = binary.Write(buf, binary.BigEndian, b)
	if err != nil {
		return 0, err
	}
	return l + 4, nil
}

func encodeMap(v reflect.Value, buf *bytes.Buffer) error {
	b := new(bytes.Buffer)
	var size, l int32
	var err error
	for _, mk := range v.MapKeys() {
		mv := v.MapIndex(mk)
		l, err = encodeString(mk, b)
		size += l
		if err != nil {
			return err
		}
		l, err = encodeString(mv, b)
		size += l
		if err != nil {
			return err
		}
	}
	err = binary.Write(buf, binary.BigEndian, int32(size))
	err = binary.Write(buf, binary.BigEndian, b.Bytes()[:size])
	return err
}

func encodeBytes(v reflect.Value, buf *bytes.Buffer) error {
	l := len(v.Bytes())
	err := binary.Write(buf, binary.BigEndian, int32(l))
	err = binary.Write(buf, binary.BigEndian, v.Bytes())
	return err
}
