package core

import (
	"fmt"
	"sync"
)

// StringMap goroutine safe string map, this will just be used in few goroutines
// so just use a simple implementation
type StringMap struct {
	mu       sync.RWMutex
	innerMap map[string]string
}

func NewStringMap(cap int) *StringMap {
	if cap < 0 {
		panic(fmt.Sprintf("illegal initial capacity %d", cap))
	}
	return &StringMap{innerMap: make(map[string]string, cap)}
}

func (m *StringMap) Store(key, value string) {
	m.mu.Lock()
	m.innerMap[key] = value
	m.mu.Unlock()
}

func (m *StringMap) Delete(key string) {
	m.mu.Lock()
	delete(m.innerMap, key)
	m.mu.Unlock()
}

func (m *StringMap) Load(key string) (value string, ok bool) {
	m.mu.RLock()
	value, ok = m.innerMap[key]
	m.mu.RUnlock()
	return value, ok
}

func (m *StringMap) LoadOrEmpty(key string) string {
	v, _ := m.Load(key)
	return v
}

// Range calls f sequentially for each key and value present in the map
// If f returns false, range stops the iteration
func (m *StringMap) Range(f func(k, v string) bool) {
	m.mu.RLock()
	keys := make([]string, 0, len(m.innerMap))
	for k := range m.innerMap {
		keys = append(keys, k)
	}
	m.mu.RUnlock()

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

func (m *StringMap) RawMap() map[string]string {
	m.mu.RLock()
	rawMap := make(map[string]string, len(m.innerMap))
	for k, v := range m.innerMap {
		rawMap[k] = v
	}
	m.mu.RUnlock()
	return rawMap
}

func (m *StringMap) Copy() *StringMap {
	return &StringMap{innerMap: m.RawMap()}
}

func (m *StringMap) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.innerMap)
}
