package vlog

import (
	"strconv"
	"testing"
)

func TestWrite(t *testing.T) {
	// new BytesBuffer
	buf := newInnerBytesBuffer()
	if buf.Len() != 0 {
		t.Errorf("new buf length not zero.")
	}
	if buf.Cap() != initSize {
		t.Errorf("buf cap not correct.real:%d, expect:%d\n", buf.Cap(), initSize)
	}

	// write string
	buf.Reset()
	buf.WriteString("string1")
	buf.WriteString("string2")
	tempbytes := buf.Bytes()
	if "string1" != string(tempbytes[:7]) {
		t.Errorf("write string not correct.buf:%+v\n", buf)
	}
	if "string2" != string(tempbytes[7:14]) {
		t.Errorf("write string not correct.buf:%+v\n", buf)
	}

	// write bool string
	buf.Reset()
	buf.WriteBoolString(true)
	buf.WriteBoolString(false)
	tempbytes = buf.Bytes()
	if "true" != string(tempbytes[:4]) {
		t.Errorf("write bool string not correct.buf:%+v\n", buf)
	}
	if "false" != string(tempbytes[4:9]) {
		t.Errorf("write bool string not correct.buf:%+v\n", buf)
	}

	// write uint64 string
	buf.Reset()
	var u1 uint64 = 11111111
	var u2 uint64 = 22222222
	buf.WriteUint64String(u1)
	buf.WriteUint64String(u2)
	tempbytes = buf.Bytes()
	if "11111111" != string(tempbytes[:8]) {
		t.Errorf("write unit64 string not correct.buf:%+v\n", buf)
	}
	if "22222222" != string(tempbytes[8:]) {
		t.Errorf("write uint64 string not correct.buf:%+v\n", buf)
	}

	// write int64 string
	buf.Reset()
	var i1 int64 = 11111111
	var i2 int64 = -22222222
	buf.WriteInt64String(i1)
	buf.WriteInt64String(i2)
	tempbytes = buf.Bytes()
	if "11111111" != string(tempbytes[:8]) {
		t.Errorf("write unit64 string not correct.buf:%+v\n", buf)
	}
	if "-22222222" != string(tempbytes[8:]) {
		t.Errorf("write uint64 string not correct.buf:%+v\n", buf)
	}
}

func TestRead(t *testing.T) {
	buf := newInnerBytesBuffer()
	string1 := "aaaaaaaaaaaa"
	buf.WriteString(string1)
	buf.WriteUint64String(uint64(len(string1)))
	buf.WriteBoolString(false)
	buf.WriteInt64String(int64(-len(string1)))

	string2 := "bbbbbbbbbbbb"
	buf.WriteString(string2)
	buf.WriteUint64String(uint64(len(string2)))
	buf.WriteBoolString(true)
	buf.WriteInt64String(int64(-len(string2)))

	data := buf.Bytes()
	buf2 := createInnerBytesBuffer(data)
	rsize := len(string1) + 2 + 5 + 3 + len(string2) + 2 + 4 + 3
	if buf2.Len() != rsize {
		t.Errorf("read buf len not correct. buf:%v\n", buf2)
	}

	// read value
	expectValue := string1 +
		strconv.Itoa(len(string1)) +
		"false" +
		"-" + strconv.Itoa(len(string1)) +
		string2 +
		strconv.Itoa(len(string2)) +
		"true" +
		"-" + strconv.Itoa(len(string2))
	if expectValue != buf2.String() {
		t.Errorf("read value not correct. buf:%v\n", buf2)
	}
}
