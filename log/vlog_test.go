package vlog

import (
	//"fmt"
	"flag"
	"testing"
	"time"
)

func TestInfo(t *testing.T) {
	flag.Parse()
	LogInit(nil)
	// 处理日志缓存
	defer Flush()

	// 日志输出到标准错误
	// flag.Set("logtostderr", "true")

	// 日志同时输出到标准错误
	//flag.Set("alsologtostderr", "true")

	// 输出日志某行的调用栈
	//flag.Set("log_backtrace_at", "vlog_test.go:17")

	// 设置日志目录
	flag.Set("log_dir", ".")
	// flag.Set("log_dir", "/Users/Arthur/Station/Go/src/motan-go/log")

	go func() {
		for {
			Infoln("info log 1")
			// Warningln("warning log")
			// Errorln("error log")
			time.Sleep(time.Millisecond * 10)
		}
	}()
	go func() {
		for {
			Infoln("info log 2")
			// Warningln("warning log")
			// Errorln("error log")
			time.Sleep(time.Millisecond * 10)
		}
	}()

	go func() {
		for {
			Infoln("info log 3")
			// Warningln("warning log")
			// Errorln("error log")
			time.Sleep(time.Millisecond * 10)
		}
	}()
	time.Sleep(time.Second * 1)
}
