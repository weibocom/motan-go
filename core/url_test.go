package core

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestFromExtinfo(t *testing.T) {
	extinfo := "triggerMemcache://10.73.32.175:22222/com.weibo.trigger.common.bean.TriggerMemcacheClient?nodeType=service&version=1.0&group=status2-core"
	url := FromExtInfo(extinfo)
	if url == nil {
		t.Fatal("parse form extinfo fail")
	}
	fmt.Printf("url:%+v", url)
	if url.Host != "10.73.32.175" || url.Port != 22222 || url.Protocol != "triggerMemcache" ||
		url.Path != "com.weibo.trigger.common.bean.TriggerMemcacheClient" {
		t.Fatalf("parse url not correct. url: %+v", url)
	}
	if url.Group != "status2-core" || url.Parameters["version"] != "1.0" || url.Parameters["nodeType"] != "service" {
		t.Fatalf("parse url params not correct. url: %+v", url)
	}

	ext2 := url.ToExtInfo()
	if !strings.Contains(ext2, "triggerMemcache://10.73.32.175:22222/com.weibo.trigger.common.bean.TriggerMemcacheClient?") ||
		!strings.Contains(ext2, "group=status2-core") ||
		!strings.Contains(ext2, "nodeType=service") ||
		!strings.Contains(ext2, "version=1.0") {
		t.Fatalf("convert url to extinfo not correct. ext2: %s", ext2)
	}
	fmt.Println("ext2:", ext2)
	//invalid
	invalidInfo := "motan://123.23.33.32"
	url = FromExtInfo(invalidInfo)
	if url != nil {
		t.Fatal("url should be nil when parse invalid extinfo")
	}
}

func TestGetInt(t *testing.T) {
	url := &Url{}
	params := make(map[string]string)
	url.Parameters = params
	key := "keyt"
	method := "method1"
	methodDesc := "string,string"
	params[key] = "12"
	v, _ := url.GetInt(key)
	intequals(12, v, t)

	params[key] = "-20"
	v, _ = url.GetInt(key)
	intequals(-20, v, t)
	v = url.GetPositiveIntValue(key, 8)
	intequals(8, v, t)

	delete(params, key)
	v = url.GetMethodIntValue(method, methodDesc, key, 6)
	intequals(6, v, t)

	url.Parameters[method+"("+methodDesc+")."+key] = "-17"
	v = url.GetMethodIntValue(method, methodDesc, key, 6)
	intequals(-17, v, t)

	v = url.GetMethodPositiveIntValue(method, methodDesc, key, 9)
	intequals(9, v, t)

}

func intequals(expect int64, realvalue int64, t *testing.T) {
	if realvalue != expect {
		t.Fatalf("getint test fail, expect :%d, real :%d", expect, realvalue)
	}
}

func TestCopyAndMerge(t *testing.T) {
	url := &Url{}
	params := make(map[string]string)
	for i := 0; i < 5; i++ {
		params["key"+strconv.FormatInt(int64(i), 10)] = "value" + strconv.FormatInt(int64(i), 10)
	}
	url.Parameters = params

	newUrl := url.Copy()
	for k, v := range url.Parameters {
		v2 := newUrl.Parameters[k]
		if v != v2 {
			t.Fatalf("url copy not correct. k: %s, expect v :%s, real v: %s", k, v, v2)
		}
	}
	url.Parameters["key1"] = "test"
	if newUrl.Parameters["key1"] == "test" {
		t.Fatal("url copy not correct. ")
	}
	newParams := make(map[string]string)
	newParams["key1"] = "xxx"
	url.MergeParams(newParams)
	if url.Parameters["key1"] != "xxx" {
		t.Fatalf("url merge not correct. expect v :%s, real v: %s", "xxx", url.Parameters["key1"])
	}

}

func TestCanServer(t *testing.T) {
	params1 := make(map[string]string)
	params2 := make(map[string]string)
	url1 := &Url{Protocol: "motan", Path: "test/path", Parameters: params1}
	url2 := &Url{Protocol: "motan", Path: "test/path", Parameters: params2}
	if !url1.CanServe(url2) {
		t.Fatalf("url CanServe testFail url1: %+v, url2: %+v\n")
	}
	params1["version"] = "0.2"
	params2["version"] = "0.3"
	if url1.CanServe(url2) {
		t.Fatalf("url CanServe testFail url1: %+v, url2: %+v\n")
	}
	fmt.Printf("url1:%+v, url2:%+v\n", url1, url2)
	params1 = make(map[string]string)
	params2 = make(map[string]string)
	url1.Parameters = params1
	url2.Parameters = params2
	params1["serialization"] = "pb"
	params2["serialization"] = "json"

	if url1.CanServe(url2) {
		t.Fatalf("url CanServe testFail url1: %+v, url2: %+v\n")
	}
	fmt.Printf("url1:%+v, url2:%+v\n", url1, url2)
	params1 = make(map[string]string)
	params2 = make(map[string]string)
	url1.Parameters = params1
	url2.Parameters = params2
	url1.Protocol = "motan"
	url2.Protocol = "grpc"
	if url1.CanServe(url2) {
		t.Fatalf("url CanServe testFail url1: %+v, url2: %+v\n")
	}
	fmt.Printf("url1:%+v, url2:%+v\n", url1, url2)
	url1.Protocol = ""
	url2.Protocol = ""
	url1.Path = "test/path"
	url2.Path = "xxxx"
	if url1.CanServe(url2) {
		t.Fatalf("url CanServe testFail url1: %+v, url2: %+v\n")
	}
	fmt.Printf("url1:%+v, url2:%+v\n", url1, url2)
}

func TestGetPositiveIntValue(t *testing.T) {
	params := make(map[string]string, 0)
	params[SessionTimeOutKey] = "20000000"
	url := &Url{Parameters: params}
	v := url.GetPositiveIntValue(SessionTimeOutKey, 20)
	if v != 20000000 {
		t.Errorf("get positive int fail. v:%d", v)
	}
}
