package core

import "sync/atomic"

type AtomicString struct {
	v atomic.Value
}

func NewAtomicString(str string) *AtomicString {
	s := &AtomicString{}
	if str != "" {
		s.Store(str)
	}
	return s
}

func (s *AtomicString) Load() string {
	v := s.v.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}

func (s *AtomicString) Store(str string) {
	s.v.Store(str)
}
