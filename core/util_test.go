package core

import (
	"fmt"
	"os"
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
	hostname := "testHostname"
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
	diffCount := 0
	for i := 0; i < size; i++ {
		if ns[i] != s[i] {
			diffCount++
		}
	}
	fmt.Printf("shuffle diff count:%d\n", diffCount)
	if diffCount < 2 {
		t.Errorf("shuffle fail. diff count:%d", diffCount)
	}
}

func TestFirstUpper(t *testing.T) {
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

func TestGetReqInfo(t *testing.T) {
	req := &MotanRequest{RequestID: 34789798073, ServiceName: "testServiceName", Method: "testMethod"}
	assert.Equal(t, "", GetReqInfo(nil))
	assert.Equal(t, "req{34789798073,testServiceName,testMethod}", GetReqInfo(req))
}

func TestGetResInfo(t *testing.T) {
	res := &MotanResponse{RequestID: 374867809809}
	resE := &MotanResponse{RequestID: 374867809809, Exception: &Exception{ErrType: ServiceException, ErrCode: 503, ErrMsg: "testErrMsg"}}
	assert.Equal(t, "", GetResInfo(nil))
	assert.Equal(t, "res{374867809809,}", GetResInfo(res))
	assert.Equal(t, "res{374867809809,testErrMsg}", GetResInfo(resE))
}

func TestGetEPFilterInfo(t *testing.T) {
	filter1 := &TestEndPointFilter{Index: 1}
	filter2 := &TestEndPointFilter{Index: 2}
	filter3 := &TestEndPointFilter{Index: 3}
	filter1.SetNext(filter2)
	filter2.SetNext(filter3)
	str := GetEPFilterInfo(filter1)
	assert.Equal(t, "TestEndPointFilter->TestEndPointFilter->TestEndPointFilter", str)
}

func TestGetDirectEnvRegistry(t *testing.T) {
	os.Unsetenv(DirectRPCEnvironmentName)
	ClearDirectEnvRegistry()
	// test init value
	assert.Nil(t, directRpc)

	u := &URL{Path: "com.weibo.helloService"}
	u1 := &URL{Path: "com.weibo.testService"}
	u2 := &URL{Path: "com.weibo.tempService"}

	// test not set env
	reg := GetDirectEnvRegistry(u)
	assert.Nil(t, directRpc)
	assert.Nil(t, reg)

	// test parse
	ClearDirectEnvRegistry()
	os.Setenv(DirectRPCEnvironmentName, "helloService>127.0.0.1:8005")
	reg = GetDirectEnvRegistry(u)
	assert.True(t, directRpc != nil && len(directRpc) == 1)
	checkReg(reg, t, "127.0.0.1:8005", "")

	// test parse multi
	ClearDirectEnvRegistry()
	os.Setenv(DirectRPCEnvironmentName, "helloService>127.0.0.1:8005;testService>10.123.123.123:7777,10.123.123.123:8888,10.123.123.123:9999;tempService>10.111.111.111:7777")
	reg = GetDirectEnvRegistry(u)
	assert.Equal(t, 3, len(directRpc))
	checkReg(reg, t, "127.0.0.1:8005", "")

	reg = GetDirectEnvRegistry(u1)
	checkReg(reg, t, "10.123.123.123:7777,10.123.123.123:8888,10.123.123.123:9999", "")

	reg = GetDirectEnvRegistry(u2)
	checkReg(reg, t, "10.111.111.111:7777", "")

	// test parse group
	ClearDirectEnvRegistry()
	os.Setenv(DirectRPCEnvironmentName, "helloService>change_group@127.0.0.1:8005;testService>temp-group@10.123.123.123:7777,10.123.123.123:8888,10.123.123.123:9999;")
	reg = GetDirectEnvRegistry(u)
	assert.Equal(t, 2, len(directRpc))
	checkReg(reg, t, "127.0.0.1:8005", "change_group")

	reg = GetDirectEnvRegistry(u1)
	checkReg(reg, t, "10.123.123.123:7777,10.123.123.123:8888,10.123.123.123:9999", "temp-group")

	// test prefix match
	ClearDirectEnvRegistry()
	os.Setenv(DirectRPCEnvironmentName, "com.weibo.t*>newGroup@127.0.0.1:8005,127.0.0.1:8006")
	reg = GetDirectEnvRegistry(u1)
	assert.Equal(t, 1, len(directRpc))
	checkReg(reg, t, "127.0.0.1:8005,127.0.0.1:8006", "newGroup")

	reg = GetDirectEnvRegistry(u2)
	checkReg(reg, t, "127.0.0.1:8005,127.0.0.1:8006", "newGroup")

	// test not match
	reg = GetDirectEnvRegistry(u)
	assert.Nil(t, reg)

	os.Unsetenv(DirectRPCEnvironmentName)
	ClearDirectEnvRegistry()
}

func checkReg(reg *URL, t *testing.T, addr string, group string) {
	assert.NotNil(t, reg)
	assert.Equal(t, "direct", reg.Protocol)
	assert.Equal(t, addr, reg.GetParam(AddressKey, ""))
	assert.Equal(t, group, reg.Group)
}
