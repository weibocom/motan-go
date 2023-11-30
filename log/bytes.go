package vlog

import (
	"errors"
	"io"
	"sync"
)

var (
	innerBytesBufferPool = sync.Pool{New: func() interface{} {
		return new(innerBytesBuffer)
	}}
)

// innerBytesBuffer is a variable-sized buffer of bytes with Read and Write methods.
// innerBytesBuffer is not thread safe for multi goroutine operation.
type innerBytesBuffer struct {
	buf  []byte // contents are the bytes buf[0 : woff] in write, are the bytes buf[roff: len(buf)] in read
	rpos int    // read position
	wpos int    // write position
}

var ErrNotEnough = errors.New("innerBytesBuffer: not enough bytes")

// newInnerBytesBuffer create an empty innerBytesBuffer with initial size
func newInnerBytesBuffer(initSize int) *innerBytesBuffer {
	bb := acquireBytesBuffer()
	if bb.buf == nil {
		bb.buf = make([]byte, initSize)
	}
	return bb
}

func createInnerBytesBuffer(data []byte) *innerBytesBuffer {
	return &innerBytesBuffer{
		buf:  data,
		rpos: 0,
		wpos: len(data),
	}
}

// SetWPos set the write position of innerBytesBuffer
func (b *innerBytesBuffer) SetWPos(pos int) {
	if cap(b.buf) < pos {
		b.grow(pos - cap(b.buf))
	}
	b.wpos = pos
}

// GetWPos get the write position of innerBytesBuffer
func (b *innerBytesBuffer) GetWPos() int {
	return b.wpos
}

// SetRPos get the read position of innerBytesBuffer
func (b *innerBytesBuffer) SetRPos(pos int) {
	b.rpos = pos
}

// GetRPos get the read position of innerBytesBuffer
func (b *innerBytesBuffer) GetRPos() int {
	return b.rpos
}

// WriteString write a str string append the innerBytesBuffer
func (b *innerBytesBuffer) WriteString(str string) {
	l := len(str)
	if cap(b.buf) < b.wpos+l {
		b.grow(l)
	}
	copy(b.buf[b.wpos:], str)
	b.wpos += l
}

// Write write a byte array append the innerBytesBuffer
func (b *innerBytesBuffer) Write(bytes []byte) {
	l := len(bytes)
	if cap(b.buf) < b.wpos+l {
		b.grow(l)
	}
	copy(b.buf[b.wpos:], bytes)
	b.wpos += l
}

// WriteBoolString append the string value of v(true/false) to innerBytesBuffer
func (b *innerBytesBuffer) WriteBoolString(v bool) {
	if v {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
}

func (b *innerBytesBuffer) grow(n int) {
	buf := make([]byte, 2*len(b.buf)+n)
	copy(buf, b.buf[:b.wpos])
	b.buf = buf
}

func (b *innerBytesBuffer) Bytes() []byte { return b.buf[:b.wpos] }

func (b *innerBytesBuffer) String() string {
	return string(b.buf[:b.wpos])
}

func (b *innerBytesBuffer) Read(p []byte) (n int, err error) {
	if b.rpos >= len(b.buf) {
		return 0, io.EOF
	}

	n = copy(p, b.buf[b.rpos:])
	b.rpos += n
	return n, nil
}

func (b *innerBytesBuffer) ReadFull(p []byte) error {
	if b.Remain() < len(p) {
		return ErrNotEnough
	}
	n := copy(p, b.buf[b.rpos:])
	if n < len(p) {
		return ErrNotEnough
	}
	b.rpos += n
	return nil
}

func (b *innerBytesBuffer) Next(n int) ([]byte, error) {
	m := b.Remain()
	if n > m {
		return nil, ErrNotEnough
	}
	data := b.buf[b.rpos : b.rpos+n]
	b.rpos += n
	return data, nil
}

func (b *innerBytesBuffer) ReadByte() (byte, error) {
	if b.rpos >= len(b.buf) {
		return 0, io.EOF
	}
	c := b.buf[b.rpos]
	b.rpos++
	return c, nil
}

func (b *innerBytesBuffer) Reset() {
	b.rpos = 0
	b.wpos = 0
}

func (b *innerBytesBuffer) Remain() int { return b.wpos - b.rpos }

func (b *innerBytesBuffer) Len() int { return b.wpos - 0 }

func (b *innerBytesBuffer) Cap() int { return cap(b.buf) }

func acquireBytesBuffer() *innerBytesBuffer {
	b := innerBytesBufferPool.Get()
	if b == nil {
		return &innerBytesBuffer{}
	}
	return b.(*innerBytesBuffer)
}

func releaseBytesBuffer(b *innerBytesBuffer) {
	if b != nil {
		b.Reset()
		innerBytesBufferPool.Put(b)
	}
}
