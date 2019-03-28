package core

import "sync/atomic"

type AtomicBool int32

// Create a new atomic bool value, you should always use *AtomicBool in a struct to avoid copy
func NewAtomicBool(value bool) *AtomicBool {
	atomicBool := new(AtomicBool)
	atomicBool.Set(value)
	return atomicBool
}

func (b *AtomicBool) Get() bool {
	return atomic.LoadInt32((*int32)(b)) == 1
}

func (b *AtomicBool) Set(value bool) {
	if value {
		atomic.StoreInt32((*int32)(b), 1)
	} else {
		atomic.StoreInt32((*int32)(b), 0)
	}
}
