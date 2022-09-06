package core

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseExportString(t *testing.T) {
	export := "motan2:8002"
	proto, port, err := ParseExportInfo(export)
	if proto != "motan2" || port != 8002 || err != nil {
		t.Errorf("parse export string fail. proto:%s, port:%d, err:%s", proto, port, err.Error())
	}
}

func TestGetLocalIP(t *testing.T) {
	ip := GetLocalIP()
	fmt.Printf("get localip:%s\n", ip)
	if ip == "" {
		t.Errorf("get local ip fail. ip:%s", ip)
	}
	hostname := "testhostname"
	*LocalIP = hostname
	ip = GetLocalIP()
	if ip != hostname {
		t.Errorf("get local ip fail. ip:%s", ip)
	}
}

func TestSliceShuffle(t *testing.T) {
	size := 32
	s := make([]string, 0, size)
	ns := make([]string, 0, size)
	for i := 0; i < size; i++ {
		s = append(s, strconv.Itoa(i))
		ns = append(ns, strconv.Itoa(i))
	}
	SliceShuffle(s)
	if len(ns) != len(s) || len(ns) != 32 {
		t.Errorf("slice shuffle fail. size not correct. size:%d", len(ns))
	}
	diffcount := 0
	for i := 0; i < size; i++ {
		if ns[i] != s[i] {
			diffcount++
		}
	}
	fmt.Printf("shuffle diff count:%d\n", diffcount)
	if diffcount < 2 {
		t.Errorf("shuffle fail. diff count:%d", diffcount)
	}
}

func TestFisrtUpper(t *testing.T) {
	s := "test"
	ns := FirstUpper(s)
	if ns != "Test" {
		t.Errorf("first upper fail. %s", ns)
	}
}

func TestHandlePanic(t *testing.T) {
	b := false
	defer func() {
		if !b {
			t.Errorf("test HandlePanic fail")
		} else {
			fmt.Println("test HandlePanic success")
		}
	}()

	defer HandlePanic(func() {
		b = true
	})
	panic("test panic")
}

func TestSplitTrim(t *testing.T) {
	type SplitTest struct {
		str    string
		sep    string
		expect []string
	}
	space := "\t\v\r\f\n\u0085\u00a0\u2000\u3000"
	var splitList = []SplitTest{
		{"", "", []string{}},
		{"abcd", "", []string{"a", "b", "c", "d"}},
		{"☺☻☹", "", []string{"☺", "☻", "☹"}},
		{"abcd", "a", []string{"", "bcd"}},
		{"abcd", "z", []string{"abcd"}},
		{space + "1....2....3....4" + space, "...", []string{"1", ".2", ".3", ".4"}},
		{"☺☻☹", "☹", []string{"☺☻", ""}},
		{"1\t " + space + "\n2\t", " ", []string{"1", "2"}},
		{"fd  , fds,  ,df\n, \v\ff ds ,,fd s , fds ,", ",", []string{"fd", "fds", "", "df", "f ds", "", "fd s", "fds", ""}},
	}
	for _, tt := range splitList {
		ret := TrimSplit(tt.str, tt.sep)
		assert.Equal(t, tt.expect, ret)
	}
}

func TestSlicesUnique(t *testing.T) {
	a := []string{"a", "a", "b"}
	b := []string{"a", "b"}
	var c []string
	assert.Equal(t, SlicesUnique(a), b)
	assert.Equal(t, SlicesUnique(b), b)
	assert.Equal(t, SlicesUnique(c), c)
}
