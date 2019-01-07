package core

import (
	"sync"
	"testing"
	"time"
)

type listener struct {
	notified bool
	lock     sync.Mutex
}

func (l *listener) Notify(value bool) {
	l.lock.Lock()
	l.notified = true
	l.lock.Unlock()
}

func (l *listener) IsNotified() bool {
	l.lock.Lock()
	defer l.lock.Unlock()
	return l.notified
}

func TestSwitcher(t *testing.T) {
	s := GetSwitcherManager()
	if s == nil {
		t.Error("GetSwitcherManager fail")
	}
	la, lb, lc := &listener{}, &listener{}, &listener{}
	s.Register("aaa", true)
	s.Register("bbb", true, lb)
	if switcher := s.GetSwitcher("aaa"); switcher != nil {
		if switcher.GetName() != "aaa" {
			t.Error("GetName error, origin: aaa, return:", switcher.GetName())
		}
	} else {
		t.Error("GetSwitcher failed")
	}

	if check := s.GetSwitcher("aaa").IsOpen(); check != true {
		t.Error("check IsOpen failed")
	}

	s.GetSwitcher("aaa").Watch(la)
	s.GetSwitcher("bbb").Watch(lc)
	s.GetSwitcher("aaa").SetValue(false)
	s.GetSwitcher("bbb").SetValue(false)
	time.Sleep(200 * time.Millisecond) //wait notify
	if !la.IsNotified() || !lb.IsNotified() || !lc.IsNotified() {
		t.Error("watch failed")
	}

	if switchers := s.GetAllSwitchers(); len(switchers) != 2 {
		t.Error("GetAllSwitchers error. expect: 3, return:", len(switchers))
	}
}
