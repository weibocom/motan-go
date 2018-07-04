package vlog

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	stdLog "log"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// notice:this log is based on Glog [https://github.com/golang/glog].
// this log maybe deprecated in next few versions

// OutputStats tracks the number of output lines and bytes written.
type OutputStats struct {
	lines int64
	bytes int64
}

// Lines returns the number of lines written.
func (s *OutputStats) Lines() int64 {
	return atomic.LoadInt64(&s.lines)
}

// Bytes returns the number of bytes written.
func (s *OutputStats) Bytes() int64 {
	return atomic.LoadInt64(&s.bytes)
}

var Stats struct {
	Info, Warning, Error OutputStats
}

var severityStats = [numSeverity]*OutputStats{
	infoLog:    &Stats.Info,
	warningLog: &Stats.Warning,
	errorLog:   &Stats.Error,
}

// flushSyncWriter is the interface satisfied by logging destinations.
type flushSyncWriter interface {
	Flush() error
	Sync() error
	io.Writer
}

// loggingT collects all the global state of the logging setup.
type loggingT struct {
	toStderr     bool // The -logtostderr flag.
	alsoToStderr bool // The -alsologtostderr flag.

	stderrThreshold severity // The -stderrthreshold flag.

	freeList *buffer

	freeListMu sync.Mutex

	mu sync.Mutex

	file [numSeverity]flushSyncWriter

	pcs [1]uintptr

	vmap map[uintptr]Level

	filterLength int32

	traceLocation traceLocation

	vmodule   moduleSpec // The state of the -vmodule flag.
	verbosity Level      // V logging level, the value of the -v flag/
}

type buffer struct {
	bytes.Buffer
	tmp  [64]byte
	next *buffer
}

var logging loggingT

func (l *loggingT) setVState(verbosity Level, filter []modulePat, setFilter bool) {
	logging.verbosity.set(0)
	atomic.StoreInt32(&logging.filterLength, 0)

	if setFilter {
		logging.vmodule.filter = filter
		logging.vmap = make(map[uintptr]Level)
	}

	atomic.StoreInt32(&logging.filterLength, int32(len(filter)))
	logging.verbosity.set(verbosity)
}

func (l *loggingT) getBuffer() *buffer {
	l.freeListMu.Lock()
	b := l.freeList
	if b != nil {
		l.freeList = b.next
	}
	l.freeListMu.Unlock()
	if b == nil {
		b = new(buffer)
	} else {
		b.next = nil
		b.Reset()
	}
	return b
}

func (l *loggingT) putBuffer(b *buffer) {
	if b.Len() >= 256 {
		return
	}
	l.freeListMu.Lock()
	b.next = l.freeList
	l.freeList = b
	l.freeListMu.Unlock()
}

var timeNow = time.Now

func (l *loggingT) header(s severity, depth int) (*buffer, string, int) {
	_, file, line, ok := runtime.Caller(5 + depth)
	if !ok {
		file = "???"
		line = 1
	} else {
		slash := strings.LastIndex(file, "/")
		if slash >= 0 {
			file = file[slash+1:]
		}
	}
	return l.formatHeader(s, file, line), file, line
}

func (l *loggingT) formatHeader(s severity, file string, line int) *buffer {
	now := timeNow()
	if line < 0 {
		line = 0 // not a real line number, but acceptable to someDigits
	}
	if s > fatalLog {
		s = infoLog // for safety.
	}
	buf := l.getBuffer()

	// Avoid Fprintf, for speed. The format is so simple that we can do it quickly by hand.
	// It's worth about 3X. Fprintf is hard.
	_, month, day := now.Date()
	hour, minute, second := now.Clock()
	// Lmmdd hh:mm:ss.uuuuuu threadid file:line]
	buf.tmp[0] = severityChar[s]
	buf.twoDigits(1, int(month))
	buf.twoDigits(3, day)
	buf.tmp[5] = ' '
	buf.twoDigits(6, hour)
	buf.tmp[8] = ':'
	buf.twoDigits(9, minute)
	buf.tmp[11] = ':'
	buf.twoDigits(12, second)
	buf.tmp[14] = '.'
	buf.nDigits(6, 15, now.Nanosecond()/1000, '0')
	buf.tmp[21] = ' '
	buf.nDigits(7, 22, pid, ' ') // TODO: should be TID
	buf.tmp[29] = ' '
	buf.Write(buf.tmp[:30])
	buf.WriteString(file)
	buf.tmp[0] = ':'
	n := buf.someDigits(1, line)
	buf.tmp[n+1] = ']'
	buf.tmp[n+2] = ' '
	buf.Write(buf.tmp[:n+3])
	return buf
}

// Some custom tiny helper functions to print the log header efficiently.

const digits = "0123456789"

func (buf *buffer) twoDigits(i, d int) {
	buf.tmp[i+1] = digits[d%10]
	d /= 10
	buf.tmp[i] = digits[d%10]
}

func (buf *buffer) nDigits(n, i, d int, pad byte) {
	j := n - 1
	for ; j >= 0 && d > 0; j-- {
		buf.tmp[i+j] = digits[d%10]
		d /= 10
	}
	for ; j >= 0; j-- {
		buf.tmp[i+j] = pad
	}
}

func (buf *buffer) someDigits(i, d int) int {
	j := len(buf.tmp)
	for {
		j--
		buf.tmp[j] = digits[d%10]
		d /= 10
		if d == 0 {
			break
		}
	}
	return copy(buf.tmp[i:], buf.tmp[j:])
}

func (l *loggingT) println(s severity, args ...interface{}) {
	buf, file, line := l.header(s, 0)
	fmt.Fprintln(buf, args...)
	l.output(s, buf, file, line, false)
}

func (l *loggingT) print(s severity, args ...interface{}) {
	l.printDepth(s, 1, args...)
}

func (l *loggingT) printDepth(s severity, depth int, args ...interface{}) {
	buf, file, line := l.header(s, depth)
	fmt.Fprint(buf, args...)
	if buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	l.output(s, buf, file, line, false)
}

func (l *loggingT) printf(s severity, format string, args ...interface{}) {
	buf, file, line := l.header(s, 0)
	fmt.Fprintf(buf, format, args...)
	if buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	l.output(s, buf, file, line, false)
}

func (l *loggingT) printWithFileLine(s severity, file string, line int, alsoToStderr bool, args ...interface{}) {
	buf := l.formatHeader(s, file, line)
	fmt.Fprint(buf, args...)
	if buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	l.output(s, buf, file, line, alsoToStderr)
}

var fatalNoStacks uint32

func (l *loggingT) output(s severity, buf *buffer, file string, line int, alsoToStderr bool) {
	l.mu.Lock()
	if l.traceLocation.isSet() {
		if l.traceLocation.match(file, line) {
			buf.Write(stacks(false))
		}
	}
	data := buf.Bytes()
	if !flag.Parsed() {
		os.Stderr.Write([]byte("ERROR: logging before flag.Parse: "))
		os.Stderr.Write(data)
	} else if l.toStderr {
		os.Stderr.Write(data)
	} else {
		if alsoToStderr || l.alsoToStderr || s >= l.stderrThreshold.get() {
			os.Stderr.Write(data)
		}
		if l.file[s] == nil {
			if err := l.createFiles(s); err != nil {
				os.Stderr.Write(data) // Make sure the message appears somewhere.
				l.exit(errorFatal, err)
			}
		}
		switch s {
		case fatalLog:
			l.file[fatalLog].Write(data)
			fallthrough
		case errorLog:
			l.file[errorLog].Write(data)
			fallthrough
		case warningLog:
			l.file[warningLog].Write(data)
			fallthrough
		case infoLog:
			l.file[infoLog].Write(data)
		}
	}
	if s == fatalLog {
		// If we got here via Exit rather than Fatal, print no stacks.
		if atomic.LoadUint32(&fatalNoStacks) > 0 {
			l.mu.Unlock()
			timeoutFlush(10 * time.Second)
			os.Exit(1)
		}
		// Dump all goroutine stacks before exiting.
		// First, make sure we see the trace for the current goroutine on standard error.
		// If -logtostderr has been specified, the loop below will do that anyway
		// as the first stack in the full dump.
		if !l.toStderr {
			os.Stderr.Write(stacks(false))
		}
		// Write the stack trace for all goroutines to the files.
		trace := stacks(true)
		// logExitFunc = func(error) {} // If we get a write error, we'll still exit below.
		for log := fatalLog; log >= infoLog; log-- {
			if f := l.file[log]; f != nil { // Can be nil if -logtostderr is set.
				f.Write(trace)
			}
		}
		l.mu.Unlock()
		timeoutFlush(10 * time.Second)
		os.Exit(255) // C++ uses -1, which is silly because it's anded with 255 anyway.
	}
	l.putBuffer(buf)
	l.mu.Unlock()
	if stats := severityStats[s]; stats != nil {
		atomic.AddInt64(&stats.lines, 1)
		atomic.AddInt64(&stats.bytes, int64(len(data)))
	}
}

func timeoutFlush(timeout time.Duration) {
	done := make(chan bool, 1)
	go func() {
		Flush() // calls logging.lockAndFlushAll()
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		fmt.Fprintln(os.Stderr, "vlog: Flush took longer than", timeout)
	}
}

func stacks(all bool) []byte {
	n := 10000
	if all {
		n = 100000
	}
	var trace []byte
	for i := 0; i < 5; i++ {
		trace = make([]byte, n)
		nbytes := runtime.Stack(trace, all)
		if nbytes < len(trace) {
			return trace[:nbytes]
		}
		n *= 2
	}
	return trace
}

var logExitFunc = func(code int, err error) bool {
	if errorFatal == code {
		return false
	}
	return true
}

func (l *loggingT) exit(code int, err error) {
	fmt.Fprintf(os.Stderr, "vlog: something error: %s, code: %d\n", err, code)
	// if logExitFunc != nil {
	// logExitFunc(err)
	// return
	// }
	if logExitFunc(code, err) {
		return
	}
	l.flushAll()
	os.Exit(2)
}

type access struct {
	tick  <-chan time.Time
	event chan bool
}

func newAccess(d time.Duration) *access {
	return &access{
		tick:  time.Tick(d),
		event: make(chan bool, 2),
	}
}

type syncBuffer struct {
	logger *loggingT
	*bufio.Writer
	file   *os.File
	fname  string
	sev    severity
	nbytes uint64 // The number of bytes written to this file
	timer  *time.Timer
	a      *access
}

func (sb syncBuffer) fileAccess() error {
	if _, err := os.Stat(sb.fname); os.IsNotExist(err) {
		sb.a.event <- true
	}
	return nil
}

func (sb syncBuffer) rotateError() error {
	sb.a.event <- false
	return nil
}

func (sb *syncBuffer) Sync() error {
	return sb.file.Sync()
}

func (sb *syncBuffer) Write(p []byte) (n int, err error) {
	select {
	case <-sb.timer.C:
		now := time.Now()
		if err := sb.rotateFile(now); err != nil {
			sb.logger.exit(errorNormal, err)
		}
		duration := time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, time.Local).Sub(now)
		for 0 == duration {
			duration = time.Hour
		}
		sb.timer.Reset(duration)
	case <-sb.a.event:
		if err := sb.rotateFile(time.Now()); err != nil {
			sb.logger.exit(errorNormal, err)
		}
	default:
		if sb.nbytes+uint64(len(p)) >= MaxSize {
			if err := sb.rotateFile(time.Now()); err != nil {
				sb.logger.exit(errorNormal, err)
			}
		}
	}
	n, err = sb.Writer.Write(p)
	sb.nbytes += uint64(n)
	if err != nil {
		sb.logger.exit(errorNormal, err)
		sb.Writer.Reset(sb.file)
	}
	return
}

// rotateFile closes the syncBuffer's file and starts a new one.
func (sb *syncBuffer) rotateFile(now time.Time) error {
	if sb.file != nil {
		sb.Flush()
		sb.file.Close()
	}
	var err error
	sb.file, sb.fname, err = create(severityName[sb.sev], now)
	sb.nbytes = 0
	if err != nil {
		return err
	}

	sb.Writer = bufio.NewWriterSize(sb.file, bufferSize)

	// Write header.
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Log file created at: %s\n", now.Format("2006/01/02 15:04:05"))
	fmt.Fprintf(&buf, "Running on machine: %s\n", host)
	fmt.Fprintf(&buf, "Binary: Built with %s %s for %s/%s\n", runtime.Compiler, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&buf, "Log line format: [IWEF]mmdd hh:mm:ss.uuuuuu threadid file:line] msg\n")
	n, err := sb.file.Write(buf.Bytes())
	sb.nbytes += uint64(n)
	return err
}

const bufferSize = 256 * 1024

func (l *loggingT) createFiles(sev severity) error {
	now := time.Now()
	// Files are created in decreasing severity order, so as soon as we find one
	// has already been created, we can stop.
	duration := time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, time.Local).Sub(now)
	for 0 == duration {
		duration = time.Hour
	}
	// l.setTimer(duration)
	for s := sev; s >= infoLog && l.file[s] == nil; s-- {
		sb := &syncBuffer{
			logger: l,
			sev:    s,
			timer:  time.NewTimer(duration),
			a:      newAccess(FlushInterval),
		}
		if err := sb.rotateFile(now); err != nil {
			return err
		}
		go func() {
			for {
				<-sb.a.tick
				sb.fileAccess()
			}
		}()
		l.file[s] = sb
	}
	return nil
}

var FlushInterval = 5 * time.Second

func (l *loggingT) flushDaemon() {
	for range time.NewTicker(FlushInterval).C {
		l.lockAndFlushAll()
	}
}

func (l *loggingT) lockAndFlushAll() {
	l.mu.Lock()
	l.flushAll()
	l.mu.Unlock()
}

func (l *loggingT) flushAll() {
	// Flush from fatal down, in case there's trouble flushing.
	for s := fatalLog; s >= infoLog; s-- {
		file := l.file[s]
		if file != nil {
			file.Flush() // ignore error
			file.Sync()  // ignore error
		}
	}
}

func (l *loggingT) setV(pc uintptr) Level {
	fn := runtime.FuncForPC(pc)
	file, _ := fn.FileLine(pc)
	// The file is something like /a/b/c/d.go. We want just the d.
	if strings.HasSuffix(file, ".go") {
		file = file[:len(file)-3]
	}
	if slash := strings.LastIndex(file, "/"); slash >= 0 {
		file = file[slash+1:]
	}
	for _, filter := range l.vmodule.filter {
		if filter.match(file) {
			l.vmap[pc] = filter.level
			return filter.level
		}
	}
	l.vmap[pc] = 0
	return 0
}

func CopyStandardLogTo(name string) {
	sev, ok := severityByName(name)
	if !ok {
		fmt.Printf("vlog CopyStandardLog fail! log.CopyStandardLogTo(%q): unrecognized severity name", name)
	}
	stdLog.SetFlags(stdLog.Lshortfile)
	stdLog.SetOutput(logBridge(sev))
}
