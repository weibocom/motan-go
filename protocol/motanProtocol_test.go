package protocol

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/serialize"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/weibocom/motan-go/core"
)

func TestVersion(t *testing.T) {
	h := &Header{}
	h.SetVersion(2)
	check(h.GetVersion, 2, t)
	h.SetVersion(8)
	check(h.GetVersion, 8, t)
	h.SetVersion(0)
	check(h.GetVersion, 0, t)
	err := h.SetVersion(32)
	if err == nil {
		t.Fatalf("header version test fail. expect err")
	}

}

func TestMsgType(t *testing.T) {
	//heartbeat
	h := &Header{}
	b := h.IsHeartbeat()
	if b {
		t.Fatalf("default header should not heartbeat msg")
	}
	h.SetHeartbeat(true)
	b = h.IsHeartbeat()
	if !b {
		t.Fatalf("header message type test fail. type heartbeat")
	}

	//gzip
	h = &Header{}
	b = h.IsGzip()
	if b {
		t.Fatalf("default header should not gzip msg")
	}
	h.SetGzip(true)
	b = h.IsGzip()
	if !b {
		t.Fatalf("header message type test fail. type gzip")
	}

	//oneway
	h = &Header{}
	b = h.IsOneWay()
	if b {
		t.Fatalf("default header should not oneway msg")
	}
	h.SetOneWay(true)
	b = h.IsOneWay()
	if !b {
		t.Fatalf("header message type test fail. type oneway")
	}

	//proxy
	h = &Header{}
	b = h.IsProxy()
	if b {
		t.Fatalf("default header should not proxy msg")
	}
	h.SetProxy(true)
	b = h.IsProxy()
	if !b {
		t.Fatalf("header message type test fail. type proxy")
	}

	//request
	h = &Header{}
	b = h.isRequest()
	if !b {
		t.Fatalf("default header should request msg")
	}
	h.SetRequest(false)
	b = h.isRequest()
	if b {
		t.Fatalf("header message type test fail. type request")
	}

}

func TestStatus(t *testing.T) {
	h := &Header{}
	h.SetStatus(2)
	check(h.GetStatus, 2, t)
	h.SetStatus(0)
	check(h.GetStatus, 0, t)
	err := h.SetStatus(8)
	if err == nil {
		t.Fatalf("header test fail. expect err")
	}
}

func TestSerialize(t *testing.T) {
	h := &Header{}
	h.SetSerialize(2)
	check(h.GetSerialize, 2, t)
	h.SetSerialize(8)
	check(h.GetSerialize, 8, t)
	h.SetSerialize(0)
	check(h.GetSerialize, 0, t)
	err := h.SetSerialize(32)
	if err == nil {
		t.Fatalf("header test fail. expect err")
	}
}

func check(f func() int, ev int, t *testing.T) {
	rv := f()
	if rv != ev {
		t.Fatalf("header test fail. expect: %d, real %d", ev, rv)
	}
}

func TestEncode(t *testing.T) {
	h := &Header{}
	h.SetVersion(Version2)
	h.SetStatus(6)
	h.SetOneWay(true)
	h.SetSerialize(5)
	h.SetGzip(true)
	h.SetHeartbeat(true)
	h.SetProxy(true)
	h.SetRequest(true)
	h.Magic = MotanMagic
	h.RequestID = 2349789
	meta := core.NewStringMap(0)
	meta.Store("k1", "v1")
	body := []byte("testbody")
	msg := &Message{Header: h, Metadata: meta, Body: body}
	ebytes := msg.Encode()

	fmt.Println("len:", ebytes.Len())
	readSlice := make([]byte, 100)
	newMsg, err := Decode(bufio.NewReader(ebytes), &readSlice)
	if newMsg == nil {
		t.Fatalf("encode message fail")
	}
	assertTrue(newMsg.Header.IsOneWay(), "oneway", t)
	assertTrue(newMsg.Header.IsGzip(), "gzip", t)
	assertTrue(newMsg.Header.IsHeartbeat(), "heartbeat", t)
	assertTrue(newMsg.Header.IsProxy(), "proxy", t)
	assertTrue(newMsg.Header.isRequest(), "request", t)
	assertTrue(newMsg.Header.GetVersion() == Version2, "version", t)
	assertTrue(newMsg.Header.GetSerialize() == 5, "serialize", t)
	assertTrue(newMsg.Header.GetStatus() == 6, "status", t)
	assertTrue(newMsg.Metadata.LoadOrEmpty("k1") == "v1", "meta", t)
	assertTrue(len(newMsg.Body) == len(msg.Body), "body", t)

	msg.Header.SetProxy(false)
	msg.Header.SetGzip(true)
	msg.Body, _ = EncodeGzip([]byte("gzip encode"))
	b := msg.Encode()
	newMsg, _ = Decode(bufio.NewReader(b), &readSlice)
	// should not decode gzip
	if !newMsg.Header.IsGzip() {
		t.Fatalf("encode message fail")
	}
	nb, err := DecodeGzip(newMsg.Body)
	if err != nil {
		t.Errorf("decode gzip fail. err:%v", err)
	}
	assertTrue(string(nb) == "gzip encode", "body", t)
}

func TestMessage_GetEncodedBytes(t *testing.T) {
	h := &Header{}
	h.SetVersion(Version2)
	h.SetStatus(6)
	h.SetOneWay(true)
	h.SetSerialize(5)
	h.SetGzip(true)
	h.SetHeartbeat(true)
	h.SetProxy(true)
	h.SetRequest(true)
	h.Magic = MotanMagic
	h.RequestID = 2349789
	meta := core.NewStringMap(0)
	meta.Store("k1", "v1")
	body := []byte("testbody")
	msg := &Message{Header: h, Metadata: meta, Body: body}
	msg.Encode0()
	encodedBytes := msg.GetEncodedBytes()
	buf := core.CreateBytesBuffer(encodedBytes[0])

	assert.Equal(t, "testbody", string(encodedBytes[1]))

	// append body to buf
	buf.Write(msg.Body)
	// verify decode
	readSlice := make([]byte, 100)
	newMsg, err := Decode(bufio.NewReader(buf), &readSlice)
	if err != nil || newMsg == nil {
		t.Fatalf("encode message fail")
	}

	// verify header
	assertTrue(newMsg.Header.IsOneWay(), "oneway", t)
	assertTrue(newMsg.Header.IsGzip(), "gzip", t)
	assertTrue(newMsg.Header.IsHeartbeat(), "heartbeat", t)
	assertTrue(newMsg.Header.IsProxy(), "proxy", t)
	assertTrue(newMsg.Header.isRequest(), "request", t)
	assertTrue(newMsg.Header.GetVersion() == Version2, "version", t)
	assertTrue(newMsg.Header.GetSerialize() == 5, "serialize", t)
	assertTrue(newMsg.Header.GetStatus() == 6, "status", t)

	// verify meta
	assertTrue(newMsg.Metadata.LoadOrEmpty("k1") == "v1", "meta", t)

	// verify body nil
	assertTrue(len(newMsg.Body) == len(msg.Body), "body", t)
}

func TestPool(t *testing.T) {
	h := &Header{}
	h.SetVersion(Version2)
	h.SetStatus(6)
	h.SetOneWay(true)
	h.SetSerialize(5)
	h.SetGzip(true)
	h.SetHeartbeat(true)
	h.SetProxy(true)
	h.SetRequest(true)
	h.Magic = MotanMagic
	h.RequestID = 2349789
	meta := core.NewStringMap(0)
	for mi := 0; mi < 10000; mi++ {
		meta.Store(strconv.Itoa(mi), strconv.Itoa(mi))
	}
	body := []byte("testbodytestbodytestbodytestbodytestbodytestbodytestbodytestbodytestbodytestbodytestbody")
	msg := &Message{Header: h, Metadata: meta, Body: body}
	ebytes := msg.Encode()

	fmt.Println("len:", ebytes.Len())
	readSlice := make([]byte, 100)
	newMsg, err := Decode(bufio.NewReader(ebytes), &readSlice)
	if newMsg == nil {
		t.Fatalf("encode message fail")
	}
	assertTrue(newMsg.Header.IsOneWay(), "oneway", t)
	assertTrue(newMsg.Header.IsGzip(), "gzip", t)
	assertTrue(newMsg.Header.IsHeartbeat(), "heartbeat", t)
	assertTrue(newMsg.Header.IsProxy(), "proxy", t)
	assertTrue(newMsg.Header.isRequest(), "request", t)
	assertTrue(newMsg.Header.GetVersion() == Version2, "version", t)
	assertTrue(newMsg.Header.GetSerialize() == 5, "serialize", t)
	assertTrue(newMsg.Header.GetStatus() == 6, "status", t)
	assertTrue(newMsg.Metadata.LoadOrEmpty("1") == "1", "meta", t)
	assertTrue(cap(readSlice) > 200, "readSlice", t)
	assertTrue(len(newMsg.Body) == len(msg.Body), "body", t)
	assert.Nil(t, err)
	ReleaseMessage(newMsg)
	body1 := []byte("testbody")
	msg1 := &Message{Header: h, Metadata: meta, Body: body1}
	ebytes1 := msg1.Encode()
	newMsg, err = Decode(bufio.NewReader(ebytes1), &readSlice)
	if newMsg == nil {
		t.Fatalf("encode message fail")
	}
	assertTrue(newMsg.Header.IsOneWay(), "oneway", t)
	assertTrue(newMsg.Header.IsGzip(), "gzip", t)
	assertTrue(newMsg.Header.IsHeartbeat(), "heartbeat", t)
	assertTrue(newMsg.Header.IsProxy(), "proxy", t)
	assertTrue(newMsg.Header.isRequest(), "request", t)
	assertTrue(newMsg.Header.GetVersion() == Version2, "version", t)
	assertTrue(newMsg.Header.GetSerialize() == 5, "serialize", t)
	assertTrue(newMsg.Header.GetStatus() == 6, "status", t)
	assertTrue(newMsg.Metadata.LoadOrEmpty("1") == "1", "meta", t)
	assertTrue(cap(readSlice) > 200, "readSlice", t)
	assertTrue(len(newMsg.Body) == len(msg1.Body), "body", t)
}

func assertTrue(b bool, msg string, t *testing.T) {
	if !b {
		t.Fatalf("test fail, %s not correct.", msg)
	}
}

func TestConvertToResponse(t *testing.T) {
	h := &Header{}
	h.SetVersion(Version2)
	h.SetStatus(6)
	h.SetOneWay(true)
	h.SetSerialize(6)
	h.SetStatus(0)
	h.SetGzip(false)
	h.SetHeartbeat(true)
	h.SetProxy(true)
	h.SetRequest(true)
	h.Magic = MotanMagic
	h.RequestID = 2349789
	meta := core.NewStringMap(0)
	meta.Store("k1", "v1")
	meta.Store(MGroup, "group")
	meta.Store(MMethod, "method")
	meta.Store(MPath, "path")
	body := []byte("testbody")
	msg := &Message{Header: h, Metadata: meta, Body: body}
	// To test convert when the method use pool
	pMap := make(map[string]string)
	for i := 0; i < 10000; i++ {
		resp, err := ConvertToResponse(msg, &serialize.SimpleSerialization{})
		assertTrue(err == nil, "conver to request err", t)
		assertTrue(resp.GetAttachment(MGroup) == "group", "response group", t)
		assertTrue(resp.GetAttachment(MMethod) == "method", "response method", t)
		assertTrue(resp.GetAttachment(MPath) == "path", "response path", t)
		assertTrue(string(resp.GetValue().(*core.DeserializableValue).Body) == "testbody", "response body", t)
		pMap[fmt.Sprintf("%p", resp)] = "1"
		core.ReleaseMotanResponse(resp.(*core.MotanResponse))
	}
	// check if responses are reused
	assert.True(t, len(pMap) < 10000)
	// To test if convert is correct when the method is called in concurrent situation
	h1 := &Header{}
	h1.SetVersion(Version2)
	h1.SetStatus(1)
	h1.SetOneWay(true)
	h1.SetSerialize(6)
	h1.SetGzip(false)
	h1.SetHeartbeat(true)
	h1.SetProxy(true)
	h1.SetRequest(true)
	h1.Magic = MotanMagic
	h1.RequestID = 1234456
	meta1 := core.NewStringMap(0)
	meta1.Store("k2", "v2")
	meta1.Store(MGroup, "group1")
	meta1.Store(MMethod, "method1")
	meta1.Store(MPath, "path1")
	meta1.Store(MException, `{"errcode": 0, "errmsg": "test exception", "errtype": 1}`)
	msg1 := &Message{Header: h1, Metadata: meta1, Body: nil}
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		time.Sleep(time.Millisecond * 500)
		for i := 0; i < 10000; i++ {
			resp, err := ConvertToResponse(msg, &serialize.SimpleSerialization{})
			assertTrue(resp.GetRequestID() == 2349789, "request id is incorrect", t)
			assertTrue(err == nil, "convert to response err", t)
			assertTrue(resp.GetAttachment(MGroup) == "group", "response group", t)
			assertTrue(resp.GetAttachment(MMethod) == "method", "response method", t)
			assertTrue(resp.GetAttachment(MPath) == "path", "response path", t)
			assertTrue(string(resp.GetValue().(*core.DeserializableValue).Body) == "testbody", "response body", t)
			core.ReleaseMotanResponse(resp.(*core.MotanResponse))
		}
		wg.Done()
	}()
	go func() {
		time.Sleep(time.Millisecond * 500)
		for i := 0; i < 10000; i++ {
			resp, err := ConvertToResponse(msg1, &serialize.SimpleSerialization{})
			assertTrue(resp.GetRequestID() == 1234456, "request id is incorrect", t)
			assertTrue(err == nil, "convert to response err", t)
			assertTrue(resp.GetAttachment(MGroup) == "group1", "response group", t)
			assertTrue(resp.GetAttachment(MMethod) == "method1", "response method", t)
			assertTrue(resp.GetAttachment(MPath) == "path1", "response path", t)
			assertTrue(resp.GetValue() == nil, "response body", t)
			assertTrue(resp.GetException().ErrMsg == "test exception", "response exception error message", t)
			assertTrue(resp.GetException().ErrCode == 0, "response exception error code", t)
			assertTrue(resp.GetException().ErrType == 1, "response exception error type", t)
			core.ReleaseMotanResponse(resp.(*core.MotanResponse))
		}
		wg.Done()
	}()
	wg.Wait()
}

func TestConvertToRequest(t *testing.T) {
	h := &Header{}
	h.SetVersion(Version2)
	h.SetStatus(6)
	h.SetOneWay(true)
	h.SetSerialize(6)
	h.SetGzip(false)
	h.SetHeartbeat(true)
	h.SetProxy(true)
	h.SetRequest(true)
	h.Magic = MotanMagic
	h.RequestID = 2349789
	meta := core.NewStringMap(0)
	meta.Store("k1", "v1")
	meta.Store(MGroup, "group")
	meta.Store(MMethod, "method")
	meta.Store(MPath, "path")
	body := []byte("testbody")
	msg := &Message{Header: h, Metadata: meta, Body: body}
	pMap := make(map[string]string)
	// To test convert when the method use pool
	for i := 0; i < 10000; i++ {
		req, err := ConvertToRequest(msg, &serialize.SimpleSerialization{})
		assertTrue(req.GetRequestID() == 2349789, "request id", t)
		assertTrue(err == nil, "conver to request err", t)
		assertTrue(req.GetAttachment(MGroup) == "group", "request group", t)
		assertTrue(req.GetAttachment(MMethod) == "method", "request method", t)
		assertTrue(req.GetAttachment(MPath) == "path", "request path", t)
		assertTrue(len(req.GetArguments()) == 1, "request argument", t)
		pMap[fmt.Sprintf("%p", req)] = "1"
		core.ReleaseMotanRequest(req.(*core.MotanRequest))
	}
	// check if requests are reused
	assert.True(t, len(pMap) < 10000)
	// To test if convert is correct when the method is called in concurrent situation
	h1 := &Header{}
	h1.SetVersion(Version2)
	h1.SetStatus(6)
	h1.SetOneWay(true)
	h1.SetSerialize(6)
	h1.SetGzip(false)
	h1.SetHeartbeat(true)
	h1.SetProxy(true)
	h1.SetRequest(true)
	h1.Magic = MotanMagic
	h1.RequestID = 1234456
	meta1 := core.NewStringMap(0)
	meta1.Store("k2", "v2")
	meta1.Store(MGroup, "group1")
	meta1.Store(MMethod, "method1")
	meta1.Store(MPath, "path1")
	msg1 := &Message{Header: h1, Metadata: meta1, Body: nil}
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		time.Sleep(time.Millisecond * 500)
		for i := 0; i < 10000; i++ {
			req, err := ConvertToRequest(msg, &serialize.SimpleSerialization{})
			assertTrue(req.GetRequestID() == 2349789, "request id is incorrect", t)
			assertTrue(err == nil, "conver to request err", t)
			assertTrue(req.GetAttachment(MGroup) == "group", "request group", t)
			assertTrue(req.GetAttachment(MMethod) == "method", "request method", t)
			assertTrue(req.GetAttachment(MPath) == "path", "request path", t)
			assertTrue(len(req.GetArguments()) == 1, "request argument", t)
			core.ReleaseMotanRequest(req.(*core.MotanRequest))
		}
		wg.Done()
	}()
	go func() {
		time.Sleep(time.Millisecond * 500)
		for i := 0; i < 10000; i++ {
			req, err := ConvertToRequest(msg1, &serialize.SimpleSerialization{})
			assertTrue(req.GetRequestID() == 1234456, "request id is incorrect", t)
			assertTrue(err == nil, "conver to request err", t)
			assertTrue(req.GetAttachment(MGroup) == "group1", "request group", t)
			assertTrue(req.GetAttachment(MMethod) == "method1", "request method", t)
			assertTrue(req.GetAttachment(MPath) == "path1", "request path", t)
			assertTrue(len(req.GetArguments()) == 0, "request argument", t)
			core.ReleaseMotanRequest(req.(*core.MotanRequest))
		}
		wg.Done()
	}()
	wg.Wait()
	// test request clone
	req, err := ConvertToRequest(msg, &serialize.SimpleSerialization{})
	cloneReq := req.Clone().(core.Request)
	assertTrue(err == nil, "conver to request err", t)
	assertTrue(cloneReq.GetAttachment(MGroup) == "group", "clone request group", t)
	assertTrue(cloneReq.GetAttachment(MMethod) == "method", "clone request method", t)
	assertTrue(cloneReq.GetAttachment(MPath) == "path", "clone request path", t)
	assertTrue(cloneReq.GetRPCContext(true).OriginalMessage.(*Message).Header.Serialize == msg.Header.Serialize, "clone request originMessage", t)
	testCloneOriginMeta := []map[string]interface{}{
		{
			"key":    MMethod,
			"expect": "method",
			"msg":    "clone originMessage meta method",
		},
		{
			"key":    MGroup,
			"expect": "group",
			"msg":    "clone originMessage meta group",
		},
		{
			"key":    MPath,
			"expect": "path",
			"msg":    "clone originMessage meta path",
		},
	}
	for _, m := range testCloneOriginMeta {
		key := m["key"].(string)
		expect := m["expect"].(string)
		tips := m["msg"].(string)
		value, ok := cloneReq.GetRPCContext(true).OriginalMessage.(*Message).Metadata.Load(key)
		assertTrue(ok == true, "load clone originMessage meta "+key, t)
		assertTrue(expect == value, tips, t)
	}
}

func BenchmarkEncodeGzip(b *testing.B) {
	DefaultGzipLevel = gzip.BestSpeed
	bs := buildBytes(10 * 1024)
	for i := 0; i < b.N; i++ {
		EncodeGzip(bs)
	}
}

func BenchmarkDecodeGzip(b *testing.B) {
	bs := buildBytes(10 * 1024)
	result, _ := EncodeGzip(bs)
	for i := 0; i < b.N; i++ {
		DecodeGzip(result)
	}
}

func BenchmarkEncodeGzipConcurrent(b *testing.B) {
	DefaultGzipLevel = gzip.BestSpeed
	bs := buildBytes(10 * 1024)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			EncodeGzip(bs)
		}
	})
}

func BenchmarkDecodeGzipConcurrent(b *testing.B) {
	bs := buildBytes(10 * 1024)
	result, _ := EncodeGzip(bs)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			DecodeGzip(result)
		}
	})
}

func TestConcurrentMessageGzip(t *testing.T) {
	size := 100
	wg := sync.WaitGroup{}
	wg.Add(size)
	var count, errCount int64
	messages := make([]*Message, size)
	originalBody := make([][]byte, size)
	for i := 0; i < size; i++ {
		messages[i] = &Message{Header: &Header{}}
		originalBody[i] = buildBytes(10 * 1024)
		messages[i].Body = originalBody[i]
	}
	for i := 0; i < size; i++ {
		j := i
		go func() {
			EncodeMessageGzip(messages[j], 1)
			nb := DecodeGzipBody(messages[j].Body)
			if string(nb) != string(originalBody[j]) {
				t.Errorf("concurrent message gzip not correct.\n")
				atomic.AddInt64(&errCount, 1)
			} else {
				atomic.AddInt64(&count, 1)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	t.Logf("count:%v, errCount: %v\n", count, errCount)
}

func TestConcurrentGzip(t *testing.T) {
	size := 100
	wg := sync.WaitGroup{}
	wg.Add(size)
	var count, errCount int64
	datas := make([][]byte, size)
	for i := 0; i < size; i++ {
		datas[i] = buildBytes(10 * 1024)
	}
	for i := 0; i < size; i++ {
		j := i
		go func() {
			temp, _ := EncodeGzip(datas[j])
			nb, _ := DecodeGzip(temp)
			if string(nb) != string(datas[j]) {
				t.Errorf("concurrent gzip not correct.\n")
				atomic.AddInt64(&errCount, 1)
			} else {
				atomic.AddInt64(&count, 1)

			}
			wg.Done()
		}()
	}
	wg.Wait()
	fmt.Printf("count:%v, errCount: %v\n", count, errCount)
}

func TestBuildExceptionResponse(t *testing.T) {
	// BuildExceptionResponse
	var requestId uint64 = 1234
	err := fmt.Errorf("test error")
	exception := &core.Exception{ErrCode: 500, ErrMsg: err.Error(), ErrType: core.ServiceException}
	msg := ExceptionToJSON(exception)

	// verify exception message
	message := BuildExceptionResponse(requestId, msg)
	assert.Equal(t, requestId, message.Header.RequestID)
	assert.Equal(t, false, message.Header.isRequest())
	assert.Equal(t, Res, int(message.Header.MsgType))
	assert.Equal(t, false, message.Header.IsProxy())
	assert.Equal(t, Exception, message.Header.GetStatus())
	assert.Equal(t, msg, message.Metadata.LoadOrEmpty(MException))

	buf := message.Encode()
	readSlice := make([]byte, 100)
	newMessage, err := Decode(bufio.NewReader(buf), &readSlice)

	// verify encode and decode exception message
	assert.Equal(t, message.Header.RequestID, newMessage.Header.RequestID)
	assert.Equal(t, message.Header.IsProxy(), newMessage.Header.IsProxy())
	assert.Equal(t, Res, int(message.Header.MsgType))
	assert.Equal(t, message.Header.isRequest(), newMessage.Header.isRequest())
	assert.Equal(t, Exception, newMessage.Header.GetStatus())
	assert.Equal(t, msg, message.Metadata.LoadOrEmpty(MException))
}

func buildBytes(size int) []byte {
	baseBytes := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	result := bytes.NewBuffer(make([]byte, 0, size))
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < size; i++ {
		result.WriteByte(baseBytes[r.Intn(len(baseBytes))])
	}
	return result.Bytes()
}
