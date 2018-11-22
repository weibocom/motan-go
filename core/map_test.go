package core

import (
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringMap(t *testing.T) {
	stringMap := NewStringMap(0)
	assert.Equal(t, 0, stringMap.Len())
	stringMap.Store("key", "value")
	assert.Equal(t, 1, stringMap.Len())
	stringMap.Delete("key")
	assert.Equal(t, 0, stringMap.Len())

	stringMap.Store("key1", "value1")
	stringMap.Store("key2", "value2")
	stringMap.Store("key3", "value3")
	stringMap.Store("key4", "value4")
	stringMap.Store("key5", "value5")
	assert.Equal(t, 5, stringMap.Len())
	value, ok := stringMap.Load("key3")
	assert.Equal(t, true, ok)
	assert.Equal(t, "value3", value)
	assert.Equal(t, "value3", stringMap.LoadOrEmpty("key3"))
	assert.Equal(t, "", stringMap.LoadOrEmpty("key6"))
	assert.Equal(t, 5, len(stringMap.RawMap()))

	stringMap.Range(func(k, v string) bool {
		assert.Equal(t, "value"+k[3:], v)
		return true
	})
}

func BenchmarkStringMap(b *testing.B) {
	stringMap := NewStringMap(0)
	for i := 0; i < b.N; i++ {
		s := strconv.Itoa(i)
		stringMap.Store(s, s)
	}
}

func BenchmarkStringMapParallel(b *testing.B) {
	stringMap := NewStringMap(0)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s := strconv.Itoa(rand.Intn(10000) + 10000)
			stringMap.Store(s, s)
		}
	})
}

func BenchmarkStringMapRead(b *testing.B) {
	size := 100
	stringMap := NewStringMap(0)
	for i := 0; i < size; i++ {
		s := strconv.Itoa(i)
		stringMap.Store(s, s)
	}
	b.SetParallelism(3)
	b.N = 10000
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			stringMap.Range(func(_, _ string) bool { return true })
		}
	})
}

func TestCopyOnWriteMap_Load(t *testing.T) {
	cowMap := NewCopyOnWriteMap()
	value, b := cowMap.Load("testKey")
	assert.Equal(t, false, b)
	assert.Equal(t, nil, value)
	cowMap.Store("testKey", "testValue")
	value, b = cowMap.Load("testKey")
	assert.Equal(t, true, b)
	assert.Equal(t, "testValue", value)
}

func TestCopyOnWriteMap_LoadOrNil(t *testing.T) {
	cowMap := NewCopyOnWriteMap()
	value := cowMap.LoadOrNil("testKey")
	assert.Equal(t, nil, value)
	cowMap.Store("testKey", "testValue")
	value = cowMap.LoadOrNil("testKey")
	assert.Equal(t, "testValue", value)
}

func TestCopyOnWriteMap_Store(t *testing.T) {
	cowMap := NewCopyOnWriteMap()
	for i := 0; i < 100; i++ {
		cowMap.Store("testKey"+strconv.Itoa(i), "testValue"+strconv.Itoa(i))
	}

	for i := 0; i < 100; i++ {
		assert.Equal(t, "testValue"+strconv.Itoa(i), cowMap.LoadOrNil("testKey"+strconv.Itoa(i)))
	}
}

func TestCopyOnWriteMap_Delete(t *testing.T) {
	cowMap := NewCopyOnWriteMap()
	for i := 0; i < 100; i++ {
		cowMap.Store("testKey"+strconv.Itoa(i), "testValue"+strconv.Itoa(i))
	}

	delIndex := rand.Intn(100)
	cowMap.Delete("testKey" + strconv.Itoa(delIndex))

	for i := 0; i < 100; i++ {
		if i == delIndex {
			assert.Equal(t, nil, cowMap.LoadOrNil("testKey"+strconv.Itoa(delIndex)))
		} else {
			assert.Equal(t, "testValue"+strconv.Itoa(i), cowMap.LoadOrNil("testKey"+strconv.Itoa(i)))
		}
	}
}
