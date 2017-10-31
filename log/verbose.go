package vlog

import (
	"runtime"
	"sync/atomic"
)

type Verbose bool

func V(level Level) Verbose {
	if logging.verbosity.get() >= level {
		return Verbose(true)
	}

	if atomic.LoadInt32(&logging.filterLength) > 0 {
		logging.mu.Lock()
		defer logging.mu.Unlock()
		if runtime.Callers(2, logging.pcs[:]) == 0 {
			return Verbose(false)
		}
		v, ok := logging.vmap[logging.pcs[0]]
		if !ok {
			v = logging.setV(logging.pcs[0])
		}
		return Verbose(v >= level)
	}
	return Verbose(false)
}

func (v Verbose) Info(args ...interface{}) {
	if v {
		logging.print(infoLog, args...)
	}
}

func (v Verbose) Infoln(args ...interface{}) {
	if v {
		logging.println(infoLog, args...)
	}
}

func (v Verbose) Infof(format string, args ...interface{}) {
	if v {
		logging.printf(infoLog, format, args...)
	}
}
