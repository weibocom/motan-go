package core

import (
	"fmt"
	"sync"
)

type StringMap interface {
	// Store sets the value for a key
	Store(key, value string)
	// Delete deletes the value for a key
	Delete(key string)
	// Load returns the value stored in the map for a key
	Load(key string) (value string, ok bool)
	// Load returns the value stored in the map for a key, empty string if no value is present
	LoadOrEmpty(key string) string
	// Range calls f sequentially for each key and value present in the map
	// If f returns false, range stops the iteration
	Range(f func(k, v string) bool)
	// RawMap returns a copy of inner map
	RawMap() map[string]string
	// Len returns the size of map
	Len() int
	// Copy returns a copy of this StringMap
	Copy() StringMap
}

var EmptyStringMap = &eSMap{}

// eSMap: empty string map
type eSMap struct {
}

func (m *eSMap) Store(key, value string) {
	panic("can not do store operation with empty map")
}

func (m *eSMap) Delete(key string) {
	panic("can not do delete operation with empty map")
}

func (m *eSMap) Load(key string) (value string, ok bool) {
	return "", false
}

func (m *eSMap) LoadOrEmpty(key string) string {
	return ""
}

func (m *eSMap) Range(f func(k, v string) bool) {
}

func (m *eSMap) RawMap() map[string]string {
	return make(map[string]string, 0)
}

func (m *eSMap) Copy() StringMap {
	return &SSMap{innerMap: m.RawMap()}
}

func (m *eSMap) Len() int {
	return 0
}

// SSMap: goroutine safe string map, this will just be used in few goroutines
// so just use a simple implementation
type SSMap struct {
	lock     sync.RWMutex
	innerMap map[string]string
}

func NewStringMap(cap int) StringMap {
	if cap < 0 {
		panic(fmt.Sprintf("illegal initial capacity %d", cap))
	}
	return &SSMap{innerMap: make(map[string]string, cap)}
}

func (m *SSMap) Store(key, value string) {
	m.lock.Lock()
	m.innerMap[key] = value
	m.lock.Unlock()
}

func (m *SSMap) Delete(key string) {
	m.lock.Lock()
	delete(m.innerMap, key)
	m.lock.Unlock()
}

func (m *SSMap) Load(key string) (value string, ok bool) {
	m.lock.RLock()
	value, ok = m.innerMap[key]
	m.lock.RUnlock()
	return value, ok
}

func (m *SSMap) LoadOrEmpty(key string) string {
	v, _ := m.Load(key)
	return v
}

func (m *SSMap) Range(f func(k, v string) bool) {
	m.lock.RLock()
	keys := make([]string, 0, len(m.innerMap))
	for k := range m.innerMap {
		keys = append(keys, k)
	}
	m.lock.RUnlock()

	for _, k := range keys {
		v, ok := m.Load(k)
		if !ok {
			continue
		}
		if !f(k, v) {
			break
		}
	}
}

func (m *SSMap) RawMap() map[string]string {
	m.lock.RLock()
	rawMap := make(map[string]string, len(m.innerMap))
	for k, v := range m.innerMap {
		rawMap[k] = v
	}
	m.lock.RUnlock()
	return rawMap
}

func (m *SSMap) Copy() StringMap {
	return &SSMap{innerMap: m.RawMap()}
}

func (m *SSMap) Len() int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return len(m.innerMap)
}
