package vlog

import (
	"flag"
	"sync"
)

var log Logger
var once sync.Once

type Logger interface {
	Infoln(...interface{})
	Infof(string, ...interface{})
	Warningln(...interface{})
	Warningf(string, ...interface{})
	Errorln(...interface{})
	Errorf(string, ...interface{})
	Fatalln(...interface{})
	Fatalf(string, ...interface{})
	Flush()
}

func LogInit(logger Logger) {
	once.Do(func() {
		//l.Instance = logging
		if nil == logger {
			flag.BoolVar(&logging.toStderr, "logtostderr", false, "log to standard error instead of files")
			flag.BoolVar(&logging.alsoToStderr, "alsologtostderr", false, "log to standard error as well as files")
			flag.Var(&logging.verbosity, "v", "log level for V logs")
			flag.Var(&logging.stderrThreshold, "stderrthreshold", "logs at or above this threshold go to stderr")
			flag.Var(&logging.vmodule, "vmodule", "comma-separated list of pattern=N settings for file-filtered logging")
			flag.Var(&logging.traceLocation, "log_backtrace_at", "when logging hits line file:N, emit a stack trace")

			// Default stderrThreshold is FATAL.
			logging.stderrThreshold = fatalLog

			logging.setVState(0, nil, false)
			go logging.flushDaemon()
			log = &Log{Instance: &logging}
		} else {
			log = logger
		}
	})
}

type Log struct {
	Instance *loggingT
}

func (l Log) Infoln(args ...interface{}) {
	logging.println(infoLog, args...)
}

func (l Log) Infof(format string, args ...interface{}) {
	logging.printf(infoLog, format, args...)
}

func (l Log) Warningln(args ...interface{}) {
	logging.println(warningLog, args...)
}

func (l Log) Warningf(format string, args ...interface{}) {
	logging.printf(warningLog, format, args...)
}

func (l Log) Errorln(args ...interface{}) {
	logging.println(errorLog, args...)

}

func (l Log) Errorf(format string, args ...interface{}) {
	logging.printf(errorLog, format, args...)
}

func (l Log) Fatalln(args ...interface{}) {
	logging.println(fatalLog, args...)

}

func (l Log) Fatalf(format string, args ...interface{}) {
	logging.printf(fatalLog, format, args...)
}

func (l Log) Flush() {
	logging.lockAndFlushAll()
}
