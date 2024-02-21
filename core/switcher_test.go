package core

import (
	"github.com/stretchr/testify/assert"
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

func TestSwitcherManager(t *testing.T) {
	s := GetSwitcherManager()
	s.clear()
	// test IsOpen
	assert.False(t, s.IsOpen("sw1")) // not exist
	s.Register("sw1", false)
	assert.False(t, s.IsOpen("sw1"))
	s.SetValue("sw1", true)
	assert.True(t, s.IsOpen("sw1"))
	s.SetValue("sw1", false)
	assert.False(t, s.IsOpen("sw1"))

	// test GetOrRegister
	s.clear()
	sw1 := s.GetOrRegister("sw1", true) // register
	assert.NotNil(t, sw1)
	assert.True(t, sw1.IsOpen())
	sw2 := s.GetOrRegister("sw1", false) // get
	assert.True(t, sw1 == sw2)           // same instance

	// test set value
	s.clear()
	s.SetValue("sw1", true) // set value for not exist switcher
	assert.True(t, s.IsOpen("sw1"))
	s.clear()
	s.SetValue("sw1", false) // set value for not exist switcher
	assert.False(t, s.IsOpen("sw1"))
	sw1 = s.GetSwitcher("sw1")
	s.SetValue("sw1", true) // set value for exist switcher
	assert.True(t, sw1.IsOpen())
	sw2 = s.GetSwitcher("sw1")
	assert.True(t, sw1 == sw2) // same instance
}
