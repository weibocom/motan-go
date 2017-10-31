package vlog

func Infoln(args ...interface{}) {
	if log != nil {
		log.Infoln(args...)
	}
}

func Infof(format string, args ...interface{}) {
	if log != nil {
		log.Infof(format, args...)
	}
}

func Warningln(args ...interface{}) {
	if log != nil {
		log.Warningln(args...)
	}
}

func Warningf(format string, args ...interface{}) {
	if log != nil {
		log.Warningf(format, args...)
	}
}

func Errorln(args ...interface{}) {
	if log != nil {
		log.Errorln(args...)
	}
}

func Errorf(format string, args ...interface{}) {
	if log != nil {
		log.Errorf(format, args...)
	}
}

func Fatalln(args ...interface{}) {
	if log != nil {
		log.Fatalln(args...)
	}
}

func Fatalf(format string, args ...interface{}) {
	if log != nil {
		log.Fatalf(format, args...)
	}
}

func Flush() {
	if log != nil {
		log.Flush()
	}
}
