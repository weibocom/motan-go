package vlog

import (
	"strconv"
	"testing"
)

func TestWrite(t *testing.T) {
	// new BytesBuffer
	initsize := 1
	buf := newInnerBytesBuffer(initsize)
	if len(buf.buf) != initsize {
		t.Errorf("wrong initsize. real size:%d, expect size:%d", len(buf.buf), initsize)
	}
	if buf.Len() != 0 {
		t.Errorf("new buf length not zero.")
	}
	if buf.Cap() != initsize {
		t.Errorf("buf cap not correct.real:%d, expect:%d\n", buf.Cap(), initsize)
	}
	if buf.wpos != 0 || buf.rpos != 0 {
		t.Errorf("buf wpos or rpos init value not correct.wpos:%d, rpos:%d\n", buf.wpos, buf.rpos)
	}

	buf.SetWPos(3)
	if buf.Cap() < 3 || buf.wpos != 3 {
		t.Errorf("buf SetWPos expand buffer failed: %d", buf.Cap())
	}

	buf.SetWPos(0)

	// write []byte
	oldpos := buf.GetWPos()
	size := 20
	bytes := make([]byte, 0, size)
	for i := 0; i < size; i++ {
		bytes = append(bytes, 'b')
	}
	buf.Write(bytes)
	if buf.wpos != size+oldpos || buf.Len() != size+oldpos {
		t.Errorf("buf wpos not correct.buf:%+v\n", buf)
	}

	// write with grow
	oldpos = buf.GetWPos()
	size = 107
	bytes = make([]byte, 0, size)
	for i := 0; i < size; i++ {
		bytes = append(bytes, 'c')
	}
	buf.Write(bytes)
	if buf.wpos != size+oldpos || buf.Len() != size+oldpos {
		t.Errorf("buf wpos not correct.buf:%+v\n", buf)
	}
	// set wpos
	buf.Reset()
	oldpos = buf.GetWPos()
	if oldpos != 0 {
		t.Errorf("buf reset wpos not correct.buf:%+v\n", buf)
	}
	buf.SetWPos(4)
	buf.Write(bytes)
	buf.SetWPos(oldpos)
	buf.WriteBoolString(true)
	buf.SetWPos(buf.GetWPos() + len(bytes))
	tempbytes := buf.Bytes()
	if len(tempbytes) != 4+len(bytes) {
		t.Errorf("set wpos test not correct.buf:%+v\n", buf)
	}

	// write string
	buf.Reset()
	buf.WriteString("string1")
	buf.WriteString("string2")
	tempbytes = buf.Bytes()
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
}

func TestRead(t *testing.T) {
	buf := newInnerBytesBuffer(128)
	size1 := 20
	bytes1 := make([]byte, 0, size1)
	for i := 0; i < size1; i++ {
		bytes1 = append(bytes1, 'a')
	}
	buf.SetWPos(2)
	buf.Write(bytes1)
	buf.SetWPos(0)
	buf.WriteString(strconv.Itoa(size1))
	buf.SetWPos(size1 + 2)
	buf.WriteBoolString(false)

	size2 := 25
	bytes2 := make([]byte, 0, size2)
	for i := 0; i < size2; i++ {
		bytes2 = append(bytes2, 'c')
	}
	oldwpos := buf.GetWPos()
	buf.SetWPos(oldwpos + 2)
	buf.Write(bytes2)
	buf.SetWPos(oldwpos)
	buf.WriteString(strconv.Itoa(size2))
	buf.SetWPos(oldwpos + size2 + 2)
	buf.WriteBoolString(true)
	buf.SetWPos(oldwpos + size2 + 2 + 4)

	data := buf.Bytes()
	buf2 := createInnerBytesBuffer(data)
	if buf2.rpos != 0 {
		t.Errorf("rpos init value not correct. buf:%v\n", buf2)
	}
	rsize := size1 + size2 + 2 + 5 + 2 + 4
	if buf2.Len() != rsize {
		t.Errorf("read buf len not correct. buf:%v\n", buf2)
	}
	if buf2.Remain() != rsize {
		t.Errorf("read buf remain not correct. buf:%v\n", buf2)
	}
	// read bytes
	rbytes1 := make([]byte, size1)
	buf2.SetRPos(2)
	buf2.ReadFull(rbytes1)
	if len(rbytes1) != size1 {
		t.Errorf("read bytes not correct. bytes: %v, buf:%v\n", rbytes1, buf2)
	}

	// read next
	buf2.SetRPos(0)
	s1, err := buf2.Next(2)
	if err != nil || string(s1) != strconv.Itoa(size1) || buf2.rpos != 2 {
		t.Errorf("read buf next not correct. buf:%v\n", buf2)
	}
	if buf2.Len() != rsize {
		t.Errorf("read buf len not correct. buf:%v\n", buf2)
	}
	if buf2.Remain() != rsize-2 {
		t.Errorf("read buf remain not correct. buf:%v\n", buf2)
	}

	rbytes2, _ := buf2.Next(size1)
	if len(rbytes2) != size1 {
		t.Errorf("read next not correct. bytes: %v, buf:%v\n", rbytes2, buf2)
	}
	for i := 0; i < size1; i++ {
		if rbytes1[i] != 'a' {
			t.Errorf("read bytes not correct. bytes: %v, buf:%v\n", rbytes1, buf2)
		}
		if rbytes2[i] != 'a' {
			t.Errorf("read next not correct. bytes: %v, buf:%v\n", rbytes2, buf2)
		}
	}

	if buf2.rpos != size1+2 {
		t.Errorf("rpos value not correct. buf:%v\n", buf2)
	}

	// read bool value
	boolBytes, _ := buf2.Next(5)
	if "false" != string(boolBytes) {
		t.Errorf("read bool value not correct. buf:%v\n", buf2)
	}
	s2, _ := buf2.Next(2)
	if string(s2) != strconv.Itoa(size2) {
		t.Errorf("read buf next not correct. buf:%v\n", buf2)
	}
	rbytes3, _ := buf2.Next(size2)
	if len(rbytes3) != size2 {
		t.Errorf("read next not correct. bytes: %v, buf:%v\n", rbytes3, buf2)
	}
	for i := 0; i < size2; i++ {
		if rbytes3[i] != 'c' {
			t.Errorf("read next not correct. bytes: %v, buf:%v\n", rbytes3, buf2)
		}
	}
	boolBytes, _ = buf2.Next(4)
	if "true" != string(boolBytes) {
		t.Errorf("read bool value not correct. buf:%v\n", buf2)
	}
}
