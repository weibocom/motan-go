package core

import (
	"fmt"
	"sync"
	"sync/atomic"
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

func (m *StringMap) Reset() {
	//TODO: 这个地方是否应该加锁呢？
	m.mu.Lock()
	for k := range m.innerMap {
		delete(m.innerMap, k)
	}
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

type CopyOnWriteMap struct {
	mu       sync.Mutex
	innerMap atomic.Value
}

func NewCopyOnWriteMap() *CopyOnWriteMap {
	return &CopyOnWriteMap{}
}

func (m *CopyOnWriteMap) data() map[interface{}]interface{} {
	v := m.innerMap.Load()
	if v == nil {
		return nil
	}
	return v.(map[interface{}]interface{})
}

func (m *CopyOnWriteMap) Load(key interface{}) (interface{}, bool) {
	value, ok := m.data()[key]
	return value, ok
}

func (m *CopyOnWriteMap) LoadOrNil(key interface{}) interface{} {
	return m.data()[key]
}

func (m *CopyOnWriteMap) Range(f func(k, v interface{}) bool) {
	for k, v := range m.data() {
		if !f(k, v) {
			return
		}
	}
}

func (m *CopyOnWriteMap) Store(key, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UnsafeStore(key, value)
}

func (m *CopyOnWriteMap) UnsafeStore(key, value interface{}) {
	lastMap := m.data()
	copiedMap := make(map[interface{}]interface{}, len(lastMap)+1)
	for k, v := range lastMap {
		copiedMap[k] = v
	}
	copiedMap[key] = value
	m.innerMap.Store(copiedMap)
}

func (m *CopyOnWriteMap) Delete(key interface{}) (pv interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lastMap := m.data()
	if _, ok := lastMap[key]; !ok {
		return pv
	}
	copiedMap := make(map[interface{}]interface{}, len(lastMap))
	for k, v := range lastMap {
		if k == key {
			pv = v
			continue
		}
		copiedMap[k] = v
	}
	m.innerMap.Store(copiedMap)
	return pv
}

func (m *CopyOnWriteMap) Len() int {
	return len(m.data())
}

func (m *CopyOnWriteMap) Swap(newMap map[interface{}]interface{}) map[interface{}]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	lastMap := m.data()
	m.innerMap.Store(newMap)
	return lastMap
}

func (m *CopyOnWriteMap) SafeDoFunc(f func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f()
}
