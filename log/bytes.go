package vlog

import (
	"strconv"
	"sync"
)

var (
	initSize             = 256
	innerBytesBufferPool = sync.Pool{New: func() interface{} {
		return &innerBytesBuffer{buf: make([]byte, 0, initSize)}
	}}
)

// innerBytesBuffer is a variable-sized buffer of bytes with Write methods.
type innerBytesBuffer struct {
	buf []byte // reuse
}

// newInnerBytesBuffer create an empty innerBytesBuffer with initial size
func newInnerBytesBuffer() *innerBytesBuffer {
	return acquireBytesBuffer()
}

func createInnerBytesBuffer(data []byte) *innerBytesBuffer {
	return &innerBytesBuffer{
		buf: data,
	}
}

// WriteString write a str string append the innerBytesBuffer
func (b *innerBytesBuffer) WriteString(str string) {
	b.buf = append(b.buf, str...)
}

// WriteBoolString append the string value of v(true/false) to innerBytesBuffer
func (b *innerBytesBuffer) WriteBoolString(v bool) {
	if v {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
}

// WriteUint64String append the string value of u to innerBytesBuffer
func (b *innerBytesBuffer) WriteUint64String(u uint64) {
	b.buf = strconv.AppendUint(b.buf, u, 10)
}

// WriteInt64String append the string value of i to innerBytesBuffer
func (b *innerBytesBuffer) WriteInt64String(i int64) {
	b.buf = strconv.AppendInt(b.buf, i, 10)
}

func (b *innerBytesBuffer) Bytes() []byte { return b.buf }

func (b *innerBytesBuffer) String() string {
	return string(b.buf)
}

func (b *innerBytesBuffer) Reset() {
	b.buf = b.buf[:0]
}

func (b *innerBytesBuffer) Len() int { return len(b.buf) }

func (b *innerBytesBuffer) Cap() int { return cap(b.buf) }

func acquireBytesBuffer() *innerBytesBuffer {
	b := innerBytesBufferPool.Get()
	if b == nil {
		return &innerBytesBuffer{buf: make([]byte, 0, 256)}
	}
	return b.(*innerBytesBuffer)
}

func releaseBytesBuffer(b *innerBytesBuffer) {
	if b != nil {
		b.Reset()
		innerBytesBufferPool.Put(b)
	}
}
