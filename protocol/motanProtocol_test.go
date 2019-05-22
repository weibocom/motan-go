package protocol

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"math/rand"
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
	newMsg, err := Decode(bufio.NewReader(ebytes))
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
	newMsg, _ = Decode(bufio.NewReader(b))
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

func assertTrue(b bool, msg string, t *testing.T) {
	if !b {
		t.Fatalf("test fail, %s not correct.", msg)
	}
}

//TODO convert

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

func buildBytes(size int) []byte {
	baseBytes := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	result := bytes.NewBuffer(make([]byte, 0, size))
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < size; i++ {
		result.WriteByte(baseBytes[r.Intn(len(baseBytes))])
	}
	return result.Bytes()
}
