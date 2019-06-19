package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAtomicBool_Get(t *testing.T) {
	var b AtomicBool

	assert.False(t, b.Get())
	b.Set(true)
	assert.True(t, b.Get())
	b.Set(false)
	assert.False(t, b.Get())

	bv := NewAtomicBool(false)
	assert.False(t, bv.Get())
	bv.Set(true)
	assert.True(t, bv.Get())
	bv.Set(false)
	assert.False(t, bv.Get())
}
