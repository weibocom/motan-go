package core

import (
	"testing"
	"time"
)

var (
	notifyA  = false
	notifyB  = false
	notifyBB = false

	lisA  = &switcherA{}
	lisB  = &switcherB{}
	lisBB = &switcherBB{}
)

type switcherA struct{}

func (*switcherA) Notify(value bool) {
	notifyA = true
}

type switcherB struct{}

func (*switcherB) Notify(value bool) {
	notifyB = true
}

type switcherBB struct{}

func (*switcherBB) Notify(value bool) {
	notifyBB = true
}

func TestSwitcher(t *testing.T) {
	s := GetSwitcherManager()
	if s == nil {
		t.Error("GetSwitcherManager fail")
	}

	s.Register("aaa", true)
	s.Register("bbb", true, lisB)
	if switcherA := s.GetSwitcher("aaa"); switcherA != nil {
		if switcherA.GetName() != "aaa" {
			t.Error("GetName error, origin: aaa, return:", switcherA.GetName())
		}
	} else {
		t.Error("GetSwitcher failed")
	}

	if check := s.GetSwitcher("aaa").IsOpen(); check != true {
		t.Error("check IsOpen failed")
	}

	s.GetSwitcher("aaa").Watch(lisA)
	s.GetSwitcher("bbb").Watch(lisBB)
	s.GetSwitcher("aaa").SetValue(false)
	s.GetSwitcher("bbb").SetValue(false)
	time.Sleep(200 * time.Millisecond) //wait notify
	if notifyA != true || notifyB != true || notifyBB != true {
		t.Error("watch failed")
	}

	if switchers := s.GetAllSwitchers(); len(switchers) != 2 {
		t.Error("GetAllSwitchers error. expect: 3, return:", len(switchers))
	}
}
