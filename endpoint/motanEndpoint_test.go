package endpoint

import (
	"bufio"
	"fmt"
	"github.com/stretchr/testify/assert"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	"github.com/weibocom/motan-go/protocol"
	"github.com/weibocom/motan-go/serialize"
	"net"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	server := StartTestServer(8989)
	defer server.Close()
	m.Run()
}

// TODO more UT
func TestGetName(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motan2"}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanEndpoint{}
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

func TestRecordErrEmptyThreshold(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motan2"}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.ClientConnectionKey, "1")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanEndpoint{}
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

	assertV2ChanelStreamEmpty(ep, t)
	ep.Destroy()
}

func TestRecordErrWithErrThreshold(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motan2"}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.ErrorCountThresholdKey, "5")
	url.PutParam(motan.ClientConnectionKey, "1")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanEndpoint{}
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
	ep.channels.channels <- buildV2Channel(conn, ep.channels.config, ep.channels.serialization)
	time.Sleep(time.Second * 2)
	//assert.True(t, ep.IsAvailable())

	assertV2ChanelStreamEmpty(ep, t)
	ep.Destroy()
}

func TestMotanEndpoint_SuccessCall(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motan2"}
	url.PutParam(motan.TimeOutKey, "2000")
	url.PutParam(motan.ErrorCountThresholdKey, "1")
	url.PutParam(motan.ClientConnectionKey, "1")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanEndpoint{}
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

	assertV2ChanelStreamEmpty(ep, t)
	ep.Destroy()
}

func TestMotanEndpoint_ErrorCall(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motan2"}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.ErrorCountThresholdKey, "1")
	url.PutParam(motan.ClientConnectionKey, "1")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanEndpoint{}
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

	assertV2ChanelStreamEmpty(ep, t)
	ep.Destroy()
}

func TestMotanEndpoint_RequestTimeout(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motan2"}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.ErrorCountThresholdKey, "1")
	url.PutParam(motan.ClientConnectionKey, "1")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanEndpoint{}
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

	assertV2ChanelStreamEmpty(ep, t)
	ep.Destroy()
}

func TestLazyInit(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motan2", Parameters: map[string]string{"lazyInit": "true"}}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.ErrorCountThresholdKey, "1")
	url.PutParam(motan.ClientConnectionKey, "1")
	url.PutParam(motan.AsyncInitConnection, "false")
	ep := &MotanEndpoint{}
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

	assertV2ChanelStreamEmpty(ep, t)
	ep.Destroy()
}

func TestAsyncInit(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motan2", Parameters: map[string]string{"asyncInitConnection": "true"}}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.ErrorCountThresholdKey, "1")
	url.PutParam(motan.ClientConnectionKey, "1")
	ep := &MotanEndpoint{}
	ep.SetURL(url)
	ep.SetProxy(true)
	ep.SetSerialization(&serialize.SimpleSerialization{})
	ep.Initialize()
	time.Sleep(time.Second * 5)
}

func TestV2StreamPool(t *testing.T) {
	var oldStream *V2Stream
	// consume v2stream poll until call New func
	for {
		oldStream = acquireV2Stream()
		if v, ok := oldStream.canRelease.Load().(bool); !ok || !v {
			break
		}
	}
	// test new Stream
	assert.NotNil(t, oldStream)
	assert.NotNil(t, oldStream.recvNotifyCh)
	oldStream.streamId = GenerateRequestID()
	// verify reset
	oldStream.recvNotifyCh <- struct{}{}

	// test canRelease
	// oldStream.canRelease is not ture，release fail
	// test reset recvNotifyCh
	assert.Equal(t, 1, len(oldStream.recvNotifyCh))
	releaseV2Stream(oldStream)
	assert.Equal(t, 1, len(oldStream.recvNotifyCh))
	// canRelease success
	oldStream.canRelease.Store(true)
	releaseV2Stream(oldStream)
	assert.Equal(t, 0, len(oldStream.recvNotifyCh))

	// test put nil
	var nilStream *V2Stream
	// can not put nil to pool
	releaseV2Stream(nilStream)
	newStream3 := acquireV2Stream()
	assert.NotEqual(t, nil, newStream3)
}

func assertV2ChanelStreamEmpty(ep *MotanEndpoint, t *testing.T) {
	if ep == nil {
		return
	}
	channels := ep.channels.getChannels()
	for {
		select {
		case c, ok := <-channels:
			if !ok || c == nil {
				return
			} else {
				c.streamLock.Lock()
				// it should be zero
				assert.Equal(t, 0, len(c.streams))
				c.streamLock.Unlock()

				c.heartbeatLock.Lock()
				assert.Equal(t, 0, len(c.heartbeats))
				c.heartbeatLock.Unlock()
			}
		default:
			return
		}
	}
}

func StartTestServer(port int) *MockServer {
	m := &MockServer{Port: port}
	m.Start()
	return m
}

type MockServer struct {
	Port int
	lis  net.Listener
}

func (m *MockServer) Start() (err error) {
	//async
	m.lis, err = net.Listen("tcp", ":"+strconv.Itoa(m.Port))
	if err != nil {
		vlog.Errorf("listen port:%d fail. err: %v", m.Port, err)
		return err
	}
	go handle(m.lis)
	return nil
}

func (m *MockServer) Close() {
	if m.lis != nil {
		m.lis.Close()
	}
}

func handle(netListen net.Listener) {
	for {
		conn, err := netListen.Accept()
		if err != nil {
			fmt.Printf("accept connection fail. err:%v", err)
			return
		}

		go handleConnection(conn, 5000)
	}
}

func handleConnection(conn net.Conn, timeout int) {
	reader := bufio.NewReader(conn)
	decodeBuf := make([]byte, 100)
	msg, err := protocol.Decode(reader, &decodeBuf)
	if err != nil {
		time.Sleep(time.Millisecond * 1000)
		conn.Close()
		return
	}
	processMsg(msg, conn)
}

func processMsg(msg *protocol.Message, conn net.Conn) {
	// mock async call, but server not reply response
	if _, ok := msg.Metadata.Load("no_response"); ok {
		return
	}
	var res *protocol.Message
	var tc *motan.TraceContext
	var err error
	lastRequestID := msg.Header.RequestID
	if msg.Header.IsHeartbeat() {
		res = protocol.BuildHeartbeat(msg.Header.RequestID, protocol.Res)
	} else {
		time.Sleep(time.Millisecond * 1000)
		serialization := &serialize.SimpleSerialization{}
		resp := &motan.MotanResponse{
			RequestID:   lastRequestID,
			Value:       "hello",
			ProcessTime: 1000,
		}
		res, err = protocol.ConvertToResMessage(resp, serialization)
		if err != nil {
			conn.Close()
		}
	}
	res.Header.RequestID = lastRequestID
	resBuf := res.Encode()
	if tc != nil {
		tc.PutResSpan(&motan.Span{Name: motan.Encode, Time: time.Now()})
	}
	conn.SetWriteDeadline(time.Now().Add(motan.DefaultWriteTimeout))
	_, err = conn.Write(resBuf.Bytes())
	if err != nil {
		conn.Close()
	}
}
