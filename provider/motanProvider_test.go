package provider

import (
	"fmt"
	cfg "github.com/weibocom/motan-go/config"
	motan "github.com/weibocom/motan-go/core"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestGetName(t *testing.T) {
	//cfg.Config的字段为私有属性，不能直接赋值，使用文件赋值？
	//mProvider := map[string]interface{}{"host": "127.0.0.1", "port": "8105", "serialization": "simple"}
	//m1 := map[string]interface{}{"motantest-zha0": mProvider}
	//m2 := &map[string]interface{}{ReverseProxyServiceConfKey: m1}
	//c := cfg.Config{m2, nil, nil}

	//m := StartTestServer(8105)
	//defer m.Close()

	mProvider := MotanProvider{gctx: &motan.Context{}}
	mProvider.gctx.Config, _ = cfg.NewConfigFromFile("../main/agentdemo.yaml")
	mProvider.url = &motan.URL{Host: "127.0.0.1", Port: 8105, Protocol: "motan2", Parameters: map[string]string{"conf-id": "motantest-zha0"}}
	mProvider.Initialize()
	zhaArgsStr := map[string]string{"num1": "12345", "num2": "54321"}
	request := &motan.MotanRequest{ServiceName: "com.weibo.motan.demo.service.MotanService0", Method: "MotanAddStr", Arguments: []interface{}{zhaArgsStr}}
	request.Attachment = make(map[string]string, 0)
	res := mProvider.Call(request)
	fmt.Printf("res:%+v\n", res.GetValue())
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
		fmt.Printf("listen port:%d fail. err: %v\n", m.Port, err)
		return err
	}
	go handle(m.lis)
	return nil
}

func (m *Mockserver) Close() {
	if m != nil {
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
