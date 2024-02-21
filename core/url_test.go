package core

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"strconv"
	"strings"
	"testing"
)

func TestFromExtInfo(t *testing.T) {
	extInfo := "triggerMemcache://10.73.32.175:22222/com.weibo.trigger.common.bean.TriggerMemcacheClient?nodeType=service&version=1.0&group=status2-core"
	url := FromExtInfo(extInfo)
	if url == nil {
		t.Fatal("parse form extInfo fail")
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
		t.Fatalf("convert url to extInfo not correct. ext2: %s", ext2)
	}
	fmt.Println("ext2:", ext2)
	//invalid
	invalidInfo := "motan://123.23.33.32"
	url = FromExtInfo(invalidInfo)
	if url != nil {
		t.Fatal("url should be nil when parse invalid extInfo")
	}
}

func TestGetInt(t *testing.T) {
	url := &URL{}
	params := make(map[string]string)
	url.Parameters = params
	key := "keyT"
	method := "method1"
	methodDesc := "string,string"
	params[key] = "12"
	v, _ := url.GetInt(key)
	intEquals(12, v, t)

	url.PutParam(key, "-20") // use PutParam set value will update the cache
	v, _ = url.GetInt(key)
	intEquals(-20, v, t)
	v = url.GetPositiveIntValue(key, 8)
	intEquals(8, v, t)

	delete(params, key)
	url.ClearCachedInfo() // clear cache
	v = url.GetMethodIntValue(method, methodDesc, key, 6)
	intEquals(6, v, t)

	url.PutParam(method+"("+methodDesc+")."+key, "-17")
	v = url.GetMethodIntValue(method, methodDesc, key, 6)
	intEquals(-17, v, t)

	v = url.GetMethodPositiveIntValue(method, methodDesc, key, 9)
	intEquals(9, v, t)

}

func intEquals(expect int64, realValue int64, t *testing.T) {
	if realValue != expect {
		t.Fatalf("getint test fail, expect :%d, real :%d", expect, realValue)
	}
}

func TestCopyAndMerge(t *testing.T) {
	url := &URL{}
	params := make(map[string]string)
	for i := 0; i < 5; i++ {
		params["key"+strconv.FormatInt(int64(i), 10)] = "value" + strconv.FormatInt(int64(i), 10)
	}
	url.Parameters = params

	newURL := url.Copy()
	for k, v := range url.Parameters {
		v2 := newURL.Parameters[k]
		if v != v2 {
			t.Fatalf("url copy not correct. k: %s, expect v :%s, real v: %s", k, v, v2)
		}
	}
	url.Parameters["key1"] = "test"
	if newURL.Parameters["key1"] == "test" {
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
	url1 := &URL{Protocol: "motan", Path: "test/path", Parameters: params1}
	url2 := &URL{Protocol: "motan", Path: "test/path", Parameters: params2}
	if !url1.CanServe(url2) {
		t.Fatalf("url CanServe testFail url1: %+v, url2: %+v\n", url1, url2)
	}
	params1["version"] = "0.2"
	params2["version"] = "0.3"
	if url1.CanServe(url2) {
		t.Fatalf("url CanServe testFail url1: %+v, url2: %+v\n", url1, url2)
	}
	fmt.Printf("url1:%+v, url2:%+v\n", url1, url2)
	params1 = make(map[string]string)
	params2 = make(map[string]string)
	url1.Parameters = params1
	url2.Parameters = params2
	params1["serialization"] = "pb"
	params2["serialization"] = "json"

	if url1.CanServe(url2) {
		t.Fatalf("url CanServe testFail url1: %+v, url2: %+v\n", url1, url2)
	}
	fmt.Printf("url1:%+v, url2:%+v\n", url1, url2)
	params1 = make(map[string]string)
	params2 = make(map[string]string)
	url1.Parameters = params1
	url2.Parameters = params2
	url1.Protocol = "motan"
	url2.Protocol = "grpc"
	if url1.CanServe(url2) {
		t.Fatalf("url CanServe testFail url1: %+v, url2: %+v\n", url1, url2)
	}
	fmt.Printf("url1:%+v, url2:%+v\n", url1, url2)
	url1.Protocol = ""
	url2.Protocol = ""
	url1.Path = "test/path"
	url2.Path = "whatever"
	if url1.CanServe(url2) {
		t.Fatalf("url CanServe testFail url1: %+v, url2: %+v\n", url1, url2)
	}
	fmt.Printf("url1:%+v, url2:%+v\n", url1, url2)
}

func TestGetPositiveIntValue(t *testing.T) {
	params := make(map[string]string, 0)
	params[SessionTimeOutKey] = "20000000"
	url := &URL{Parameters: params}
	v := url.GetPositiveIntValue(SessionTimeOutKey, 20)
	if v != 20000000 {
		t.Errorf("get positive int fail. v:%d", v)
	}
}

func TestIntParamCache(t *testing.T) {
	url := &URL{}
	// test normal
	url.PutParam(SessionTimeOutKey, "20000000")
	_, ok := url.intParamCache.Load(SessionTimeOutKey) // cache will remove after new value set
	assert.False(t, ok)
	checkIntCache(t, url, SessionTimeOutKey, 20000000)

	url.PutParam(SessionTimeOutKey, "15")
	_, ok = url.intParamCache.Load(SessionTimeOutKey) // cache will remove after new value set
	assert.False(t, ok)
	checkIntCache(t, url, SessionTimeOutKey, 15)

	// clear cache
	url.ClearCachedInfo()
	_, ok = url.intParamCache.Load(SessionTimeOutKey) // cache will remove after new value set
	assert.False(t, ok)
	checkIntCache(t, url, SessionTimeOutKey, 15)

	// test miss cache
	_, ok = url.GetInt("notExist")
	assert.False(t, ok)
	nv, ok := url.intParamCache.Load("notExist")
	assert.True(t, ok)
	assert.NotNil(t, nv)
	if ic, ok := nv.(*int64Cache); ok {
		assert.True(t, ic.isMiss)
		assert.Equal(t, defaultMissCache, ic)
	}
	assert.True(t, ok)

	// test hasMethod cache
	v := url.GetMethodIntValue("method", "desc", "testKey", 10)
	assert.Equal(t, int64(10), v) // default value
	assert.False(t, url.hasMethodParams())
	url.PutParam("method(desc).testKey", "100")
	assert.True(t, url.hasMethodParams())
	v = url.GetMethodIntValue("method", "desc", "testKey", 10)
	assert.Equal(t, int64(100), v)
	url.ClearCachedInfo()
	// test init with method params
	assert.True(t, url.hasMethodParams())
	// test init without method params
	delete(url.Parameters, "method(desc).testKey")
	url.ClearCachedInfo()
	assert.False(t, url.hasMethodParams())

	// test GetPortStr
	url.Port = 8080
	assert.Equal(t, "", url.portStr.Load())
	assert.Equal(t, "8080", url.GetPortStr())
	assert.Equal(t, "8080", url.portStr.Load().(string))
}

func checkIntCache(t *testing.T, url *URL, k string, v int64) {
	dv := url.GetIntValue(k, 20)
	assert.Equal(t, v, dv)
	cv, ok := url.intParamCache.Load(k)
	assert.True(t, ok)
	assert.NotNil(t, cv)
	if i, ok := cv.(*int64Cache); ok {
		assert.Equal(t, v, i.value)
	}
	assert.True(t, ok)
}
