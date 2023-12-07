package core

import (
	"math/rand"
	"strconv"
	"sync"
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

func TestStringMap_Load(t *testing.T) {
	size := 10
	stringMap := NewStringMap(size)
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	for i := 0; i < size; i++ {
		s := "test" + strconv.Itoa(i)
		stringMap.Store(s, s)
	}
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				s := "test" + strconv.Itoa(rand.Intn(size))
				stringMap.Store(s, s)
			}
		}
	}()
	for i := 0; i < 100; i++ {
		s := "test" + strconv.Itoa(rand.Intn(size))
		assert.Equal(t, s, stringMap.LoadOrEmpty(s))
	}
	stop <- struct{}{}
	wg.Wait()
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

func BenchmarkStringMap_Range(b *testing.B) {
	size := 100
	stringMap := NewStringMap(0)
	for i := 0; i < size; i++ {
		s := strconv.Itoa(i)
		stringMap.Store(s, s)
	}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			stringMap.Range(func(_, _ string) bool { return true })
		}
	})
}

func BenchmarkStringMap_Load(b *testing.B) {
	size := 20
	stringMap := NewStringMap(0)
	for i := 0; i < size; i++ {
		s := strconv.Itoa(i)
		stringMap.Store(s, s)
	}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			stringMap.Load(strconv.Itoa(rand.Intn(size)))
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

func TestCopyOnWriteMap_Range(t *testing.T) {
	cowMap := NewCopyOnWriteMap()
	for i := 0; i < 100; i++ {
		cowMap.Store("testKey"+strconv.Itoa(i), "testValue"+strconv.Itoa(i))
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

func TestNewCopyOnWriteMap(t *testing.T) {
	size := 10
	cowMap := NewCopyOnWriteMap()
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	for i := 0; i < size; i++ {
		s := "test" + strconv.Itoa(i)
		cowMap.Store(s, s)
	}
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				s := "test" + strconv.Itoa(rand.Intn(size))
				cowMap.Store(s, s)
			}
		}
	}()
	for i := 0; i < 100; i++ {
		s := "test" + strconv.Itoa(rand.Intn(size))
		assert.Equal(t, s, cowMap.LoadOrNil(s))
	}
	stop <- struct{}{}
	wg.Wait()
}

func BenchmarkCopyOnWriteMap_Store(b *testing.B) {
	cowMap := NewCopyOnWriteMap()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cowMap.Store(rand.Intn(100), rand.Int())
		}
	})
}

func BenchmarkCopyOnWriteMap_Load(b *testing.B) {
	size := 20
	cowMap := NewCopyOnWriteMap()
	for i := 0; i < size; i++ {
		s := strconv.Itoa(i)
		cowMap.Store(s, s)
	}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cowMap.Load(strconv.Itoa(rand.Intn(size)))
		}
	})
}

func TestStringMap_Range(t *testing.T) {
	a := NewStringMap(10)
	a.Store("a", "a")
	a.Store("b", "a")
	a.Store("c", "a")
	s := ""
	a.Range(func(k, v string) bool {
		s += v
		return true
	})
	assert.Equal(t, "aaa", s)
}
