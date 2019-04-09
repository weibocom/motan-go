package vlog

import (
	"bytes"
	"flag"
	"fmt"
	goLog "log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	log    Logger
	once   sync.Once
	logDir = flag.String("log_dir", "", "If non-empty, write log files in this directory")
)

const (
	defaultLogSuffix      = ".log"
	defaultAccessLogName  = "access"
	defaultMetricsLogName = "metrics"
	defaultLogLevel       = InfoLevel
	defaultFlushInterval  = time.Second
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

func (l *LogLevel) UnmarshalText(text []byte) error {
	if l == nil {
		return errors.New("level is nil")
	}
	if !l.unmarshalText(text) && !l.unmarshalText(bytes.ToLower(text)) {
		return errors.New("unrecognized level:" + string(text))
	}
	return nil
}

func (l *LogLevel) unmarshalText(text []byte) bool {
	switch string(text) {
	case "":
		*l = defaultLogLevel
	case "trace", "TRACE":
		*l = TraceLevel
	case "debug", "DEBUG":
		*l = DebugLevel
	case "info", "INFO":
		*l = InfoLevel
	case "warn", "WARN":
		*l = WarnLevel
	case "error", "ERROR":
		*l = ErrorLevel
	case "dpanic", "DPANIC":
		*l = DPanicLevel
	case "panic", "PANIC":
		*l = PanicLevel
	case "fatal", "FATAL":
		*l = FatalLevel
	default:
		return false
	}
	return true
}

func (l *LogLevel) Set(s string) error {
	return l.UnmarshalText([]byte(s))
}

type Logger interface {
	Infoln(...interface{})
	Infof(string, ...interface{})
	Warningln(...interface{})
	Warningf(string, ...interface{})
	Errorln(...interface{})
	Errorf(string, ...interface{})
	Fatalln(...interface{})
	Fatalf(string, ...interface{})
	AccessLog(string, ...interface{})
	MetricsLog(string)
	Flush()
	GetLevel() LogLevel
	SetLevel(LogLevel)
	SetAccessLog(bool)
	SetMetricsLog(bool)
}

func LogInit(logger Logger) {
	once.Do(func() {
		if logger == nil {
			log = newDefaultLog(*logDir)
		} else {
			log = logger
		}
		go Flush()
	})
}

func Infoln(args ...interface{}) {
	if log != nil {
		log.Infoln(args...)
	} else {
		goLog.Println(args...)
	}
}

func Infof(format string, args ...interface{}) {
	if log != nil {
		log.Infof(format, args...)
	} else {
		goLog.Printf(format, args...)
	}
}

func Warningln(args ...interface{}) {
	if log != nil {
		log.Warningln(args...)
	} else {
		goLog.Println(args...)
	}
}

func Warningf(format string, args ...interface{}) {
	if log != nil {
		log.Warningf(format, args...)
	} else {
		goLog.Printf(format, args...)
	}
}

func Errorln(args ...interface{}) {
	if log != nil {
		log.Errorln(args...)
	} else {
		goLog.Println(args...)
	}
}

func Errorf(format string, args ...interface{}) {
	if log != nil {
		log.Errorf(format, args...)
	} else {
		goLog.Printf(format, args...)
	}
}

func Fatalln(args ...interface{}) {
	if log != nil {
		log.Fatalln(args...)
	} else {
		goLog.Println(args...)
	}
}

func Fatalf(format string, args ...interface{}) {
	if log != nil {
		log.Fatalf(format, args...)
	} else {
		goLog.Printf(format, args...)
	}
}

func AccessLog(filterName string, keValue ...interface{}) {
	if log != nil {
		log.AccessLog(filterName, keValue...)
	} else {
		goLog.Println(keValue...)
	}
}

func MetricsLog(msg string) {
	if log != nil {
		log.MetricsLog(msg)
	} else {
		goLog.Println(msg)
	}
}

func Flush() {
	if log != nil {
		for range time.NewTicker(defaultFlushInterval).C {
			log.Flush()
		}
	}
}

func GetLevel() LogLevel {
	return log.GetLevel()
}

func SetLevel(level LogLevel) {
	log.SetLevel(level)
}

func SetAccessLog(status bool) {
	log.SetAccessLog(status)
}

func SetMetricsLog(status bool) {
	log.SetMetricsLog(status)
}

func newDefaultLog(logDir string) Logger {
	//Construct log's logger
	hook := lumberjack.Logger{
		LocalTime: true,
		Filename:  filepath.Join(logDir, filepath.Base(os.Args[0])+defaultLogSuffix),
	}
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:      "time",
		LevelKey:     "level",
		CallerKey:    "caller",
		MessageKey:   "message",
		EncodeLevel:  zapcore.CapitalLevelEncoder,
		EncodeTime:   zapcore.ISO8601TimeEncoder,
		EncodeCaller: zapcore.ShortCallerEncoder,
	}
	level := zap.NewAtomicLevel()
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(&hook)),
		level,
	)
	caller := zap.AddCaller()
	skip := zap.AddCallerSkip(2)
	logger := zap.New(core, caller, skip)

	//Construct accessLog's logger
	accessHook := lumberjack.Logger{
		LocalTime: true,
		Filename:  filepath.Join(logDir, defaultAccessLogName+defaultLogSuffix),
	}
	accessEncoderConfig := zapcore.EncoderConfig{
		TimeKey:    "time",
		MessageKey: "message",
		EncodeTime: zapcore.ISO8601TimeEncoder,
	}
	accessLevel := zap.NewAtomicLevel()
	accessCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(accessEncoderConfig),
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(&accessHook)),
		accessLevel,
	)
	accessLogger := zap.New(accessCore)

	//Construct metricsLog's logger
	metricsHook := lumberjack.Logger{
		LocalTime: true,
		Filename:  filepath.Join(logDir, defaultMetricsLogName+defaultLogSuffix),
	}
	metricsEncoderConfig := zapcore.EncoderConfig{
		TimeKey:    "time",
		MessageKey: "message",
		EncodeTime: zapcore.ISO8601TimeEncoder,
	}
	metricsLevel := zap.NewAtomicLevel()
	metricsCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(metricsEncoderConfig),
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(&metricsHook)),
		metricsLevel,
	)
	metricsLogger := zap.New(metricsCore)

	return &defaultLogger{
		logger:        logger.Sugar(),
		level:         level,
		accessLogger:  accessLogger.Sugar(),
		accessLevel:   accessLevel,
		metricsLogger: metricsLogger,
		metricsLevel:  metricsLevel,
	}
}

type defaultLogger struct {
	logger        *zap.SugaredLogger
	level         zap.AtomicLevel
	accessLogger  *zap.SugaredLogger
	accessLevel   zap.AtomicLevel
	metricsLogger *zap.Logger
	metricsLevel  zap.AtomicLevel
}

func (d *defaultLogger) Infoln(fields ...interface{}) {
	d.logger.Info(fields...)
}

func (d *defaultLogger) Infof(format string, fields ...interface{}) {
	d.logger.Infof(format, fields...)
}

func (d *defaultLogger) Warningln(fields ...interface{}) {
	d.logger.Warn(fields...)
}

func (d *defaultLogger) Warningf(format string, fields ...interface{}) {
	d.logger.Warnf(format, fields...)
}

func (d *defaultLogger) Errorln(fields ...interface{}) {
	d.logger.Error(fields...)
}

func (d *defaultLogger) Errorf(format string, fields ...interface{}) {
	d.logger.Errorf(format, fields...)
}

func (d *defaultLogger) Fatalln(fields ...interface{}) {
	d.logger.Fatal(fields...)
}

func (d *defaultLogger) Fatalf(format string, fields ...interface{}) {
	d.logger.Fatalf(format, fields...)
}

func (d *defaultLogger) AccessLog(filterName string, keyValue ...interface{}) {
	d.accessLogger.Infow(filterName, keyValue...)
}

func (d *defaultLogger) MetricsLog(msg string) {
	d.metricsLogger.Info(msg)
}

func (d *defaultLogger) Flush() {
	_ = d.logger.Sync()
	_ = d.accessLogger.Sync()
}

func (d *defaultLogger) GetLevel() LogLevel {
	return LogLevel(d.level.Level())
}

func (d *defaultLogger) SetLevel(level LogLevel) {
	d.level.SetLevel(zapcore.Level(level))
}

func (d *defaultLogger) SetAccessLog(status bool) {
	if status {
		d.accessLevel.SetLevel(zapcore.Level(defaultLogLevel))
	} else {
		d.accessLevel.SetLevel(zapcore.Level(defaultLogLevel + 1))
	}
}

func (d *defaultLogger) SetMetricsLog(status bool) {
	if status {
		d.metricsLevel.SetLevel(zapcore.Level(defaultLogLevel))
	} else {
		d.metricsLevel.SetLevel(zapcore.Level(defaultLogLevel + 1))
	}

}
