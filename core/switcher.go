package core

import (
	"sync"

	"github.com/weibocom/motan-go/log"
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

func GetSwitcherManager() *SwitcherManager {
	return manager
}

func (s *SwitcherManager) Register(name string, value bool, listeners ...SwitcherListener) {
	if name == "" {
		vlog.Warningln("[switcher] register failed: switcher name is empty")
		return
	}
	s.switcherLock.Lock()
	defer s.switcherLock.Unlock()
	if _, ok := s.switcherMap[name]; ok {
		vlog.Warningf("[switcher] register failed: %s has been registered", name)
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

func (s *SwitcherManager) GetSwitcher(name string) *Switcher {
	s.switcherLock.RLock()
	defer s.switcherLock.RUnlock()
	if sw, ok := s.switcherMap[name]; ok {
		return sw
	}
	vlog.Warningf("[switcher] get switcher failed: %s is not registered", name)
	return nil
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
