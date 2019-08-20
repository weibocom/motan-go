package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAtomicString(t *testing.T) {
	var as AtomicString
	assert.Equal(t, "", as.Load())
	as.Store("test")
	assert.Equal(t, "test", as.Load())
}
