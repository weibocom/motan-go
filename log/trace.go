package vlog

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// traceLocation represents the setting of the -log_backtrace_at flag.
type traceLocation struct {
	file string
	line int
}

func (t *traceLocation) isSet() bool {
	return t.line > 0
}

func (t *traceLocation) match(file string, line int) bool {
	if t.line != line {
		return false
	}
	if i := strings.LastIndex(file, "/"); i >= 0 {
		file = file[i+1:]
	}
	return t.file == file
}

func (t *traceLocation) String() string {
	// Lock because the type is not atomic. TODO: clean this up.
	logging.mu.Lock()
	defer logging.mu.Unlock()
	return fmt.Sprintf("%s:%d", t.file, t.line)
}

func (t *traceLocation) Get() interface{} {
	return nil
}

var errTraceSyntax = errors.New("syntax error: expect file.go:234")

func (t *traceLocation) Set(value string) error {
	if value == "" {
		// Unset.
		t.line = 0
		t.file = ""
	}
	fields := strings.Split(value, ":")
	if len(fields) != 2 {
		return errTraceSyntax
	}
	file, line := fields[0], fields[1]
	if !strings.Contains(file, ".") {
		return errTraceSyntax
	}
	v, err := strconv.Atoi(line)
	if err != nil {
		return errTraceSyntax
	}
	if v <= 0 {
		return errors.New("negative or zero value for level")
	}
	logging.mu.Lock()
	defer logging.mu.Unlock()
	t.line = v
	t.file = file
	return nil
}
