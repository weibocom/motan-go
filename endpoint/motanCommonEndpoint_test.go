package endpoint

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/protocol"
	"github.com/weibocom/motan-go/serialize"
	"net"
	"runtime"
	"testing"
	"time"
)

func TestGetV1Name(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motanV1Compatible"}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanCommonEndpoint{}
	ep.SetURL(url)

	ep.SetProxy(true)
	ep.SetSerialization(&serialize.SimpleSerialization{})
	ep.Initialize()
	assert.Equal(t, defaultChannelPoolSize, ep.clientConnection)
	fmt.Printf("format\n")
	request := &motan.MotanRequest{ServiceName: "test", Method: "test"}
	//request.Attachment = motan.NewStringMap(0)
	res := ep.Call(request)
	fmt.Printf("res:%+v\n", res)
	ep.Destroy()
}

func TestV1RecordErrEmptyThreshold(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motanV1Compatible"}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.ClientConnectionKey, "1")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanCommonEndpoint{}
	ep.SetURL(url)
	ep.SetProxy(true)
	ep.SetSerialization(&serialize.SimpleSerialization{})
	ep.Initialize()
	assert.Equal(t, 1, ep.clientConnection)
	for j := 0; j < 5; j++ {
		request := &motan.MotanRequest{ServiceName: "test", Method: "test"}
		request.Attachment = motan.NewStringMap(0)
		ep.Call(request)
		assert.True(t, ep.IsAvailable())
	}
	ep.Destroy()
}

func TestV1RecordErrWithErrThreshold(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motanV1Compatible"}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.ErrorCountThresholdKey, "5")
	url.PutParam(motan.ClientConnectionKey, "1")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanCommonEndpoint{}
	ep.SetURL(url)
	ep.SetProxy(true)
	ep.SetSerialization(&serialize.SimpleSerialization{})
	ep.Initialize()
	assert.Equal(t, 1, ep.clientConnection)
	for j := 0; j < 10; j++ {
		request := &motan.MotanRequest{ServiceName: "test", Method: "test"}
		request.Attachment = motan.NewStringMap(0)
		ep.Call(request)
		if j < 4 {
			assert.True(t, ep.IsAvailable())
		} else {
			assert.False(t, ep.IsAvailable())
		}
	}
	<-ep.channels.channels
	conn, err := ep.channels.factory()
	assert.Nil(t, err)
	_ = conn.(*net.TCPConn).SetNoDelay(true)
	ep.channels.channels <- buildChannel(conn, ep.channels.config, ep.channels.serialization)
	time.Sleep(time.Second * 2)
	//assert.True(t, ep.IsAvailable())
	ep.Destroy()
}

func TestMotanCommonEndpoint_SuccessCall(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motanV1Compatible"}
	url.PutParam(motan.TimeOutKey, "2000")
	url.PutParam(motan.ErrorCountThresholdKey, "1")
	url.PutParam(motan.ClientConnectionKey, "1")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanCommonEndpoint{}
	ep.SetURL(url)
	ep.SetSerialization(&serialize.SimpleSerialization{})
	ep.Initialize()
	assert.Equal(t, 1, ep.clientConnection)
	request := &motan.MotanRequest{ServiceName: "test", Method: "test"}
	request.Attachment = motan.NewStringMap(0)
	res := ep.Call(request)
	assert.Nil(t, res.GetException())
	v := res.GetValue()
	s, ok := v.(string)
	assert.True(t, ok)
	assert.Equal(t, s, "hello")
}

func TestMotanCommonEndpoint_AsyncCall(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motanV1Compatible"}
	url.PutParam(motan.TimeOutKey, "2000")
	url.PutParam(motan.ErrorCountThresholdKey, "1")
	url.PutParam(motan.ClientConnectionKey, "1")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanCommonEndpoint{}
	ep.SetURL(url)
	ep.SetSerialization(&serialize.SimpleSerialization{})
	ep.Initialize()
	assert.Equal(t, 1, ep.clientConnection)
	var resStr string
	request := &motan.MotanRequest{ServiceName: "test", Method: "test", RPCContext: &motan.RPCContext{AsyncCall: true, Result: &motan.AsyncResult{Reply: &resStr, Done: make(chan *motan.AsyncResult, 5)}}}
	request.Attachment = motan.NewStringMap(0)
	res := ep.Call(request)
	assert.Nil(t, res.GetException())
	resp := <-request.GetRPCContext(false).Result.Done
	assert.Nil(t, resp.Error)
	assert.Equal(t, resStr, "hello")
}

func TestMotanCommonEndpoint_ErrorCall(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motanV1Compatible"}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.ErrorCountThresholdKey, "1")
	url.PutParam(motan.ClientConnectionKey, "1")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanCommonEndpoint{}
	ep.SetURL(url)
	ep.SetProxy(true)
	ep.SetSerialization(&serialize.SimpleSerialization{})
	ep.Initialize()
	assert.Equal(t, 1, ep.clientConnection)
	request := &motan.MotanRequest{ServiceName: "test", Method: "test"}
	request.Attachment = motan.NewStringMap(0)
	res := ep.Call(request)
	fmt.Println(res.GetException().ErrMsg)
	assert.False(t, ep.IsAvailable())
	time.Sleep(1 * time.Millisecond)
	beforeNGoroutine := runtime.NumGoroutine()
	ep.Call(request)
	time.Sleep(1 * time.Millisecond)
	assert.Equal(t, beforeNGoroutine, runtime.NumGoroutine())
	ep.Destroy()
}

func TestMotanCommonEndpoint_RequestTimeout(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motanV1Compatible"}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.ErrorCountThresholdKey, "1")
	url.PutParam(motan.ClientConnectionKey, "1")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanCommonEndpoint{}
	ep.SetURL(url)
	ep.SetProxy(true)
	ep.SetSerialization(&serialize.SimpleSerialization{})
	ep.Initialize()
	assert.Equal(t, 1, ep.clientConnection)
	request := &motan.MotanRequest{ServiceName: "test", Method: "test"}
	request.Attachment = motan.NewStringMap(0)
	request.Attachment.Store(protocol.MTimeout, "150")
	res := ep.Call(request)
	fmt.Println(res.GetException().ErrMsg)
	assert.False(t, ep.IsAvailable())
	time.Sleep(1 * time.Millisecond)
	beforeNGoroutine := runtime.NumGoroutine()
	ep.Call(request)
	time.Sleep(1 * time.Millisecond)
	assert.Equal(t, beforeNGoroutine, runtime.NumGoroutine())
	ep.Destroy()
}

func TestV1LazyInit(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motanV1Compatible", Parameters: map[string]string{"lazyInit": "true"}}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.ErrorCountThresholdKey, "1")
	url.PutParam(motan.ClientConnectionKey, "1")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanCommonEndpoint{}
	ep.SetURL(url)
	ep.SetProxy(true)
	ep.SetSerialization(&serialize.SimpleSerialization{})
	ep.Initialize()
	c := <-ep.channels.channels
	assert.Nil(t, c)
	ep.channels.channels <- nil
	request := &motan.MotanRequest{ServiceName: "test", Method: "test"}
	request.Attachment = motan.NewStringMap(0)
	res := ep.Call(request)
	fmt.Println(res.GetException().ErrMsg)
	assert.False(t, ep.IsAvailable())
	time.Sleep(1 * time.Millisecond)
	beforeNGoroutine := runtime.NumGoroutine()
	ep.Call(request)
	time.Sleep(1 * time.Millisecond)
	assert.Equal(t, beforeNGoroutine, runtime.NumGoroutine())
	ep.Destroy()
}

func TestV1AsyncInit(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motanV1Compatible", Parameters: map[string]string{"asyncInitConnection": "true"}}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.ErrorCountThresholdKey, "1")
	url.PutParam(motan.ClientConnectionKey, "1")
	ep := &MotanCommonEndpoint{}
	ep.SetURL(url)
	ep.SetProxy(true)
	ep.SetSerialization(&serialize.SimpleSerialization{})
	ep.Initialize()
	time.Sleep(time.Second * 5)
}

// TestMotanCommonEndpoint_AsyncCallNoResponse verify V2Channel streams memory leak when server not reply response
// TODO::  bugs to be fixed
func TestMotanCommonEndpoint_AsyncCallNoResponse(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motanV1Compatible"}
	url.PutParam(motan.TimeOutKey, "2000")
	url.PutParam(motan.ErrorCountThresholdKey, "1")
	url.PutParam(motan.ClientConnectionKey, "1")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanCommonEndpoint{}
	ep.SetURL(url)
	ep.SetSerialization(&serialize.SimpleSerialization{})
	ep.Initialize()
	assert.Equal(t, 1, ep.clientConnection)
	var resStr string
	request := &motan.MotanRequest{ServiceName: "test", Method: "test", RPCContext: &motan.RPCContext{AsyncCall: true, Result: &motan.AsyncResult{Reply: &resStr, Done: make(chan *motan.AsyncResult, 5)}}}
	request.Attachment = motan.NewStringMap(0)
	// server not reply
	request.SetAttachment("no_response", "true")

	res := ep.Call(request)
	assert.Nil(t, res.GetException())
	timeoutTimer := time.NewTimer(time.Second * 3)
	defer timeoutTimer.Stop()
	select {
	case <-request.GetRPCContext(false).Result.Done:
		t.Errorf("unexpect condition, recv response singnal")
	case <-timeoutTimer.C:
		t.Logf("expect condition, not recv response singnal")
	}

	// Channel.streams will not release stream
	c := <-ep.channels.getChannels()
	// it will be zero if server not reply response, bug to be fixed
	assert.Equal(t, 1, len(c.streams))
}

func TestStreamPool(t *testing.T) {
	var oldStream *Stream
	// consume stream poll until call New func
	for {
		oldStream = AcquireStream()
		if oldStream.release == false {
			break
		}
	}
	// test new Stream
	assert.NotNil(t, oldStream)
	assert.NotNil(t, oldStream.timer)
	assert.NotNil(t, oldStream.recvNotifyCh)
	oldStream.streamId = GenerateRequestID()
	assert.Equal(t, false, oldStream.release)

	// test release and acquire
	// release false, oldStream.release is false
	ReleaseStream(oldStream)
	// newStream1 not oldStream
	newStream1 := AcquireStream()
	assert.Equal(t, uint64(0), newStream1.streamId)
	// release success
	oldStream.release = true
	ReleaseStream(oldStream)
	newStream2 := AcquireStream()
	// newStream2 is oldStream
	assert.Equal(t, oldStream.streamId, newStream2.streamId)

	// test put nil
	var nilStream *Stream
	// can not put nil to pool
	streamPool.Put(nilStream)
	newStream3 := streamPool.Get()
	assert.NotEqual(t, nil, newStream3)

	// test reset recvNotifyCh
	resetStream := AcquireStream()
	resetStream.recvNotifyCh <- struct{}{}
	assert.Equal(t, 1, len(resetStream.recvNotifyCh))

	resetStream.release = true
	ReleaseStream(resetStream)
	assert.Equal(t, 0, len(resetStream.recvNotifyCh))
}
