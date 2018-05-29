package core

import "sync"

// ConcurrentStringMap for attachment and metadata, this will just be used in few goroutines(generally one or two)
// so just use a simple implementation
type ConcurrentStringMap struct {
	lock     sync.RWMutex
	innerMap map[string]string
}

func NewConcurrentStringMap() *ConcurrentStringMap {
	return &ConcurrentStringMap{innerMap: make(map[string]string)}
}

func (m *ConcurrentStringMap) Load(key string) (value string, ok bool) {
	m.lock.RLock()
	value, ok = m.innerMap[key]
	m.lock.RUnlock()
	return value, ok
}

func (m *ConcurrentStringMap) LoadOrEmpty(key string) string {
	v, _ := m.Load(key)
	return v
}

func (m *ConcurrentStringMap) Range(f func(k, v string) bool) {
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

func (m *ConcurrentStringMap) RawMap() map[string]string {
	m.lock.RLock()
	rawMap := make(map[string]string, len(m.innerMap))
	for k, v := range m.innerMap {
		rawMap[k] = v
	}
	m.lock.RUnlock()
	return rawMap
}

func (m *ConcurrentStringMap) Store(key, value string) {
	m.lock.Lock()
	m.innerMap[key] = value
	m.lock.Unlock()
}

func (m *ConcurrentStringMap) Delete(key string) {
	m.lock.Lock()
	delete(m.innerMap, key)
	m.lock.Unlock()
}

func (m *ConcurrentStringMap) Len() int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return len(m.innerMap)
}
