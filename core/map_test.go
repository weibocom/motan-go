package core

import (
	"math/rand"
	"strconv"
	"testing"
)

func assertEqualInt(excepted, actual int, t *testing.T) {
	if excepted != actual {
		t.Errorf("int except: %v, but actual was: %v", excepted, actual)
	}
}

func assertEqualString(excepted, actual string, t *testing.T) {
	if excepted != actual {
		t.Errorf("string except: %v, but actual was: %v", excepted, actual)
	}
}

func assertEqualBool(excepted, actual bool, t *testing.T) {
	if excepted != actual {
		t.Errorf("bool except: %v, but actual was: %v", excepted, actual)
	}
}

func TestConcurrentStringMap(t *testing.T) {
	stringMap := NewConcurrentStringMap()
	assertEqualInt(0, stringMap.Len(), t)
	stringMap.Store("key", "value")
	assertEqualInt(1, stringMap.Len(), t)
	stringMap.Delete("key")
	assertEqualInt(0, stringMap.Len(), t)

	stringMap.Store("key1", "value1")
	stringMap.Store("key2", "value2")
	stringMap.Store("key3", "value3")
	stringMap.Store("key4", "value4")
	stringMap.Store("key5", "value5")
	assertEqualInt(5, stringMap.Len(), t)
	value, ok := stringMap.Load("key3")
	assertEqualBool(true, ok, t)
	assertEqualString("value3", value, t)
	assertEqualString("value3", stringMap.LoadOrEmpty("key3"), t)
	assertEqualString("", stringMap.LoadOrEmpty("key6"), t)

	assertEqualInt(5, len(stringMap.RawMap()), t)

	stringMap.Range(func(k, v string) bool {
		assertEqualString("value"+k[3:], v, t)
		return true
	})
}

func BenchmarkConcurrentStringMap(b *testing.B) {
	stringMap := NewConcurrentStringMap()
	for i := 0; i < b.N; i++ {
		s := strconv.Itoa(i)
		stringMap.Store(s, s)
	}
}

func BenchmarkConcurrentStringMapParallel(b *testing.B) {
	stringMap := NewConcurrentStringMap()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s := strconv.Itoa(rand.Intn(10000) + 10000)
			stringMap.Store(s, s)
		}
	})
}

func BenchmarkConcurrentStringMapRead(b *testing.B) {
	size := 100
	stringMap := NewConcurrentStringMap()
	for i := 0; i < size; i++ {
		s := strconv.Itoa(i)
		stringMap.Store(s, s)
	}
	b.SetParallelism(3)
	b.N = 3
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			stringMap.Range(func(_, _ string) bool { return true })
		}
	})
}
