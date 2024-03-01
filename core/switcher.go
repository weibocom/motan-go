package core

import (
	"github.com/weibocom/motan-go/log"
	"sync"
)

var (
	manager = &SwitcherManager{switcherMap: make(map[string]*Switcher)}
)

type SwitcherManager struct {
	switcherLock sync.RWMutex
	switcherMap  map[string]*Switcher
}

type SwitcherListener interface {
	Notify(value bool)
}

type switcherConfig map[string]bool

func GetSwitcherManager() *SwitcherManager {
	return manager
}

func initSwitcher(c *Context) {
	switcherOnce.Do(func() {
		var sc switcherConfig
		err := c.Config.GetStruct("switchers", &sc)
		if err != nil {
			vlog.Warningf("init switcher config fail: %v", err)
			return
		}
		for k, v := range sc {
			GetSwitcherManager().Register(k, v)
		}
	})
}

func (s *SwitcherManager) Register(name string, value bool, listeners ...SwitcherListener) {
	if name == "" {
		vlog.Warningln("[switcher] register failed: switcher name is empty")
		return
	}
	s.switcherLock.Lock()
	defer s.switcherLock.Unlock()
	if _, ok := s.switcherMap[name]; ok { // just ignore if already registered
		return
	}
	newSwitcher := &Switcher{name: name, value: value, listeners: []SwitcherListener{}}
	if len(listeners) > 0 {
		newSwitcher.Watch(listeners...)
	}
	s.switcherMap[name] = newSwitcher
	vlog.Infof("[switcher] register %s success, value:%v, len(listeners):%d", name, value, len(listeners))
}

func (s *SwitcherManager) GetAllSwitchers() map[string]bool {
	s.switcherLock.RLock()
	defer s.switcherLock.RUnlock()
	result := make(map[string]bool)
	for key, sw := range s.switcherMap {
		result[key] = sw.value
	}
	return result
}

// GetSwitcher returns the switcher with the given name, or nil if not found.
func (s *SwitcherManager) GetSwitcher(name string) *Switcher {
	s.switcherLock.RLock()
	defer s.switcherLock.RUnlock()
	if sw, ok := s.switcherMap[name]; ok {
		return sw
	}
	vlog.Warningf("[switcher] get switcher failed: %s is not registered", name)
	return nil
}

// GetOrRegister returns the switcher with the given name if it's already registered, otherwise registers and returns the new switcher.
func (s *SwitcherManager) GetOrRegister(name string, value bool, listeners ...SwitcherListener) *Switcher {
	sw := s.GetSwitcher(name)
	if sw == nil {
		s.Register(name, value, listeners...)
	}
	return s.GetSwitcher(name)
}

// IsOpen returns true if the switcher is present and on, otherwise false.
func (s *SwitcherManager) IsOpen(sn string) bool {
	s.switcherLock.RLock()
	defer s.switcherLock.RUnlock()
	if sw, ok := s.switcherMap[sn]; ok {
		return sw.IsOpen()
	}
	return false
}

// SetValue sets the value of the switcher with the given name.
func (s *SwitcherManager) SetValue(name string, value bool) {
	s.GetOrRegister(name, value).SetValue(value)
}

// clear clears all registered switchers. only for testing.
func (s *SwitcherManager) clear() {
	s.switcherLock.Lock()
	defer s.switcherLock.Unlock()
	s.switcherMap = make(map[string]*Switcher)
}

type Switcher struct {
	name         string
	value        bool
	listenerLock sync.RWMutex
	listeners    []SwitcherListener
}

func (s *Switcher) GetName() string {
	return s.name
}

func (s *Switcher) IsOpen() bool {
	return s.value
}

func (s *Switcher) Watch(listeners ...SwitcherListener) {
	s.listenerLock.Lock()
	defer s.listenerLock.Unlock()
	for _, listener := range listeners {
		s.listeners = append(s.listeners, listener)
	}
	vlog.Infof("[switcher] watch %s success. len(listeners):%d", s.GetName(), len(listeners))
}

func (s *Switcher) SetValue(value bool) {
	name := s.GetName()
	if value == s.value {
		return
	}
	s.value = value
	vlog.Infof("[switcher] value changed, name:%s, value:%v", name, value)
	listeners := s.listeners
	if listeners != nil {
		go func() {
			s.listenerLock.RLock()
			defer s.listenerLock.RUnlock()
			for _, listener := range listeners {
				listener.Notify(value)
			}
			vlog.Infof("[switcher] notify %s all listeners. len(listeners):%d", name, len(listeners))
		}()
	}
}
