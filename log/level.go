package vlog

import (
	"strconv"
	"sync/atomic"
)

type Level int32

func (l *Level) get() Level {
	return Level(atomic.LoadInt32((*int32)(l)))
}

func (l *Level) set(val Level) {
	atomic.StoreInt32((*int32)(l), int32(val))
}

func (l *Level) String() string {
	return strconv.FormatInt(int64(*l), 10)
}

// Get is part of the flag.Value interface.
func (l *Level) Get() interface{} {
	return *l
}

// Set is part of the flag.Value interface.
func (l *Level) Set(value string) error {
	v, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	logging.mu.Lock()
	defer logging.mu.Unlock()
	logging.setVState(Level(v), logging.vmodule.filter, false)
	return nil
}
