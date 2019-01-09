package vlog

import (
	"fmt"
	goLog "log"
)

const defaultOutPutChanSize = 1000

func Infoln(args ...interface{}) {
	if log != nil {
		writeLogln(args...)
	} else {
		goLog.Println(args...)
	}
}

func Infof(format string, args ...interface{}) {
	if log != nil {
		writeLogf(format, args...)
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

func Flush() {
	if log != nil {
		log.Flush()
	}
}

// Temporarily solved the vlog asynchronous output.
func outputLoop() {
	go func() {
		for {
			select {
			case logStr := <-outputChan:
				log.Infof(logStr)
			}
		}
	}()
}

func writeLogf(format string, args ...interface{}) {
	outputChan <- fmt.Sprintf(format, args...)
}

func writeLogln(args ...interface{}) {
	outputChan <- fmt.Sprintln(args...)
}
