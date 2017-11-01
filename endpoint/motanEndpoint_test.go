package endpoint

import (
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	serialize "github.com/weibocom/motan-go/serialize"
)

//TODO more UT
func TestGetName(t *testing.T) {
	m := StartTestServer(8989)
	defer m.Close()

	url := &motan.Url{Port: 8989, Protocol: "motan2"}
	ep := &MotanEndpoint{}
	ep.SetUrl(url)
	ep.SetProxy(true)
	ep.SetSerialization(&serialize.SimpleSerialization{})
	ep.Initialize()
	fmt.Printf("format\n")
	request := &motan.MotanRequest{ServiceName: "test", Method: "test"}
	request.Attachment = make(map[string]string, 0)
	res := ep.Call(request)
	fmt.Printf("res:%+v\n", res)
}

func StartTestServer(port int) *Mockserver {
	m := &Mockserver{Port: port}
	m.Start()
	return m
}

type Mockserver struct {
	Port int
	lis  net.Listener
}

func (m *Mockserver) Start() (err error) {
	//async
	m.lis, err = net.Listen("tcp", ":"+strconv.Itoa(m.Port))
	if err != nil {
		vlog.Errorf("listen port:%d fail. err: %v\n", m.Port, err)
		return err
	}
	go handle(m.lis)
	return nil
}

func (m *Mockserver) Close() {
	if m.lis != nil {
		m.lis.Close()
	}
}

func handle(netListen net.Listener) {
	for {
		conn, err := netListen.Accept()
		if err != nil {
			fmt.Printf("accept connection fail. err:%v", err)
			continue
		}

		go handleConnection(conn, 5000)
	}
}

func handleConnection(conn net.Conn, timeout int) {
	time.Sleep(time.Millisecond * 1000)
	conn.Close()
}
