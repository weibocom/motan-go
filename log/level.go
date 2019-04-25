package vlog

import (
	"errors"
	"fmt"
	"strings"
)

const (
	TraceLevel LogLevel = iota - 2
	DebugLevel
	InfoLevel
	WarnLevel
	ErrorLevel
	DPanicLevel
	PanicLevel
	FatalLevel
)

type LogLevel int8

func (l LogLevel) String() string {
	switch l {
	case TraceLevel:
		return "trace"
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	case DPanicLevel:
		return "dpanic"
	case PanicLevel:
		return "panic"
	case FatalLevel:
		return "fatal"
	default:
		return fmt.Sprintf("Level(%d)", l)
	}
}

func (l *LogLevel) Set(level string) error {
	if l == nil {
		return errors.New("level is nil")
	}
	switch strings.ToLower(level) {
	case "":
		*l = defaultLogLevel
	case "trace":
		*l = TraceLevel
	case "debug":
		*l = DebugLevel
	case "info":
		*l = InfoLevel
	case "warn":
		*l = WarnLevel
	case "error":
		*l = ErrorLevel
	case "dpanic":
		*l = DPanicLevel
	case "panic":
		*l = PanicLevel
	case "fatal":
		*l = FatalLevel
	default:
		return errors.New("unrecognized level:" + string(level))
	}
	return nil
}
