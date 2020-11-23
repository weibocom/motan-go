package endpoint

import (
	"fmt"
	"net"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	"github.com/weibocom/motan-go/serialize"
)

func TestMain(m *testing.M) {
	server := StartTestServer(8989)
	defer server.Close()
	m.Run()
}

//TODO more UT
func TestGetName(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motan2"}
	url.PutParam(motan.TimeOutKey, "100")
	ep := &MotanEndpoint{}
	ep.SetURL(url)
	ep.SetProxy(true)
	ep.SetSerialization(&serialize.SimpleSerialization{})
	ep.Initialize()
	assert.Equal(t, defaultChannelPoolSize, ep.clientConnection)
	fmt.Printf("format\n")
	request := &motan.MotanRequest{ServiceName: "test", Method: "test"}
	request.Attachment = motan.NewStringMap(0)
	res := ep.Call(request)
	fmt.Printf("res:%+v\n", res)
}

func TestMotanEndpoint_Call(t *testing.T) {
	url := &motan.URL{Port: 8989, Protocol: "motan2"}
	url.PutParam(motan.TimeOutKey, "100")
	url.PutParam(motan.ErrorCountThresholdKey, "1")
	url.PutParam(motan.ClientConnectionKey, "1")
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
	ep.Destroy()
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
	time.Sleep(time.Millisecond * 1000)
	conn.Close()
}
