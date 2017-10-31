package core

import (
	"fmt"
	"strconv"
	"testing"
)

func TestParseExportString(t *testing.T) {
	export := "motan2:8002"
	proto, port, err := ParseExportInfo(export)
	if proto != "motan2" || port != 8002 || err != nil {
		t.Errorf("parse export string fail. proto:%s, port:%d, err:%s", proto, port, err.Error())
	}
}

func TestGetLocalIp(t *testing.T) {
	ip := GetLocalIp()
	fmt.Printf("get localip:%s\n", ip)
	if ip == "" {
		t.Errorf("get local ip fail. ip:%s", ip)
	}
	hostname := "testhostname"
	*LocalIp = hostname
	ip = GetLocalIp()
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
	Slice_shuffle(s)
	if len(ns) != len(s) || len(ns) != 32 {
		t.Errorf("slice shuffle fail. size not correct. size:%d", len(ns))
	}
	diffcount := 0
	for i := 0; i < size; i++ {
		if ns[i] != s[i] {
			diffcount += 1
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
