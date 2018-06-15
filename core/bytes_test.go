package core

import (
	"encoding/binary"
	"fmt"
	"testing"
)

func TestWrite(t *testing.T) {
	// new BytesBuffer
	initsize := 1
	buf := NewBytesBufferWithOrder(initsize, binary.LittleEndian)
	if buf.order != binary.LittleEndian {
		t.Errorf("order not correct. real:%s, expect:%s\n", buf.order, binary.LittleEndian)
	}

	buf = NewBytesBuffer(initsize)
	if len(buf.buf) != initsize {
		t.Errorf("wrong initsize. real size:%d, expect size:%d", len(buf.buf), initsize)
	}
	if buf.order != binary.BigEndian {
		t.Errorf("default order not bigendian.")
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

	// write byte
	buf.WriteByte('a')
	if buf.wpos != 1 || buf.Len() != 1 {
		t.Errorf("buf wpos not correct.buf:%+v\n", buf)
	}

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
	buf.WriteUint32(uint32(len(bytes)))
	buf.SetWPos(buf.GetWPos() + len(bytes))
	tempbytes := buf.Bytes()
	if int(binary.BigEndian.Uint32(tempbytes[:4])) != len(bytes) {
		t.Errorf("write uint32 not correct.buf:%+v\n", buf)
	}
	if len(tempbytes) != 4+len(bytes) {
		t.Errorf("set wpos test not correct.buf:%+v\n", buf)
	}

	// write uint
	buf.Reset()
	buf.WriteUint32(uint32(123))
	buf.WriteUint64(uint64(789))
	tempbytes = buf.Bytes()
	if 123 != int(binary.BigEndian.Uint32(tempbytes[:4])) {
		t.Errorf("write uint32 not correct.buf:%+v\n", buf)
	}
	if 789 != int(binary.BigEndian.Uint64(tempbytes[4:12])) {
		t.Errorf("write uint32 not correct.buf:%+v\n", buf)
	}

}

func TestRead(t *testing.T) {
	buf := NewBytesBuffer(128)
	size1 := 20
	bytes1 := make([]byte, 0, size1)
	for i := 0; i < size1; i++ {
		bytes1 = append(bytes1, 'a')
	}
	buf.SetWPos(4)
	buf.Write(bytes1)
	buf.SetWPos(0)
	buf.WriteUint32(uint32(size1))
	buf.SetWPos(size1 + 4)

	buf.WriteByte('b')

	size2 := 25
	bytes2 := make([]byte, 0, size2)
	for i := 0; i < size2; i++ {
		bytes2 = append(bytes2, 'c')
	}
	oldwpos := buf.GetWPos()
	buf.SetWPos(oldwpos + 8)
	buf.Write(bytes2)
	buf.SetWPos(oldwpos)
	buf.WriteUint64(uint64(size2))
	buf.SetWPos(oldwpos + 8 + size2)

	data := buf.Bytes()
	buf2 := CreateBytesBuffer(data)
	if buf2.order != binary.BigEndian {
		t.Errorf("order not correct. real:%s, expect:%s\n", buf.order, binary.BigEndian)
	}
	if buf2.rpos != 0 {
		t.Errorf("rpos init value not correct. buf:%v\n", buf2)
	}
	rsize := size1 + size2 + 4 + 8 + 1
	if buf2.Len() != rsize {
		t.Errorf("read buf len not correct. buf:%v\n", buf2)
	}
	if buf2.Remain() != rsize {
		t.Errorf("read buf remain not correct. buf:%v\n", buf2)
	}

	// read uint
	l1, _ := buf2.ReadUint32()
	if l1 != 20 || buf2.rpos != 4 {
		t.Errorf("read uint32 not correct. buf:%v\n", buf2)
	}
	if buf2.Len() != rsize {
		t.Errorf("read buf len not correct. buf:%v\n", buf2)
	}
	if buf2.Remain() != rsize-4 {
		t.Errorf("read buf remain not correct. buf:%v\n", buf2)
	}

	// read bytes
	rbytes1 := make([]byte, size1)
	buf2.ReadFull(rbytes1)
	if len(rbytes1) != size1 {
		t.Errorf("read bytes not correct. bytes: %v, buf:%v\n", rbytes1, buf2)
	}

	// read next
	buf2.SetRPos(buf2.GetRPos() - size1)
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

	if buf2.rpos != size1+4 {
		t.Errorf("rpos value not correct. buf:%v\n", buf2)
	}

	// read byte
	b, _ := buf2.ReadByte()
	if b != 'b' {
		t.Errorf("read byte not correct. byte: %v, buf:%v\n", b, buf2)
	}

	// read uint64
	l2, _ := buf2.ReadUint64()
	if int(l2) != size2 {
		t.Errorf("read uint32 not correct. buf:%v\n", buf2)
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

}

func TestZigzag(t *testing.T) {
	times := 128
	f1 := 1678
	buf := NewBytesBuffer(times * 8)
	// zigzag32
	for i := 0; i < times; i++ {
		buf.WriteZigzag32(uint32(i * f1))
	}
	bytes := buf.Bytes()
	fmt.Printf("bytes:%v\n", bytes)
	buf2 := CreateBytesBuffer(bytes)
	for i := 0; i < times; i++ {
		ni, err := buf2.ReadZigzag32()
		if err != nil || int(ni) != i*f1 {
			t.Errorf("zigzag32 not correct. ni: %d, i:%d, err :%v, buf:%v\n", ni, i, err, buf2)
		}
	}

	//zigzag64
	buf.Reset()
	f2 := 7289374928
	for i := 0; i < times; i++ {
		buf.WriteZigzag64(uint64(i * f2))
	}
	bytes = buf.Bytes()
	fmt.Printf("bytes:%v\n", bytes)
	buf2 = CreateBytesBuffer(bytes)
	for i := 0; i < times; i++ {
		ni, err := buf2.ReadZigzag64()
		if err != nil || int(ni) != i*f2 {
			t.Errorf("zigzag64 not correct. ni: %d, i:%d, err :%v, buf:%v\n", ni, i, err, buf2)
		}
	}
}
