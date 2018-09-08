package core

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewTraceContext(t *testing.T) {
	beforeTest()
	for i := 0; i < int(MaxTraceSize+10); i++ {
		tc := NewTraceContext(uint64(i))
		if i <= int(MaxTraceSize) && tc == nil ||
			i > int(MaxTraceSize) && tc != nil {
			t.Errorf("NewTraceContext test fail. TraceContext is nil:%t, i:%d, max:%d\n", tc == nil, i, MaxTraceSize)
		}
	}
}

func TestNoTrace(t *testing.T) {
	beforeTest()
	for i := 0; i < int(MaxTraceSize); i++ {
		tc := NoTrace(uint64(i), nil)
		if tc != nil {
			t.Errorf("NoTrace test fail. TraceContext is nil:%t, i:%d, max:%d\n", tc == nil, i, MaxTraceSize)
		}
	}
}

func TestAlwaysTrace(t *testing.T) {
	beforeTest()
	for i := 0; i < int(MaxTraceSize); i++ {
		tc := AlwaysTrace(uint64(i), nil)
		if tc == nil {
			t.Errorf("AlwaysTrace test fail. TraceContext is nil:%t, i:%d, max:%d\n", tc == nil, i, MaxTraceSize)
		}
	}
}

func TestRandomTrace(t *testing.T) {
	beforeTest()
	RandomTraceBase = 5 // 1/5 probability
	size := 20
	tcs := make([]*TraceContext, 0, size)
	for i := 0; i < size; i++ {
		tc := RandomTrace(uint64(i), nil)
		if tc != nil {
			tcs = append(tcs, tc)
		}
	}
	if len(tcs) == 0 || len(tcs) == size {
		t.Errorf("RandomTrace test fail. TraceContexts  size:%d\n", len(tcs))
	}
}

func TestGetTraceContexts(t *testing.T) {
	beforeTest()
	size := 20
	tcs := make([]*TraceContext, 0, size)
	for i := 0; i < size; i++ {
		tc := NewTraceContext(uint64(i))
		if tc != nil {
			tcs = append(tcs, tc)
		}
	}
	tcs2 := GetTraceContexts()
	assert.Equal(t, len(tcs), len(tcs2))

	for i, tc := range tcs2 {
		assert.Equal(t, tcs[i], tc)
	}
}

func beforeTest() {
	MaxTraceSize = 50
	GetTraceContexts()
}
