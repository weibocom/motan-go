package vlog

import goLog "log"

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

func Flush() {
	if log != nil {
		log.Flush()
	}
}
