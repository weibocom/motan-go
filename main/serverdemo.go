package main

import (
	"bytes"
	"fmt"
	"github.com/weibocom/motan-go"
	"net/http"
	"time"
)

func main() {
	runServerDemo()
	//runTestHttpServer()
}

//使用motan库创建motan服务器
func runServerDemo() {
	mscontext := motan.GetMotanServerContext("main/serverdemo.yaml")
	mscontext.RegisterService(&Motan2TestService{}, "")
	mscontext.RegisterService(&MotanDemoService{}, "")

	//zha测试
	mscontext.RegisterService(&MotanTestService0{}, "")
	mscontext.RegisterService(&MotanTestService1{}, "")

	mscontext.Start(nil)
	mscontext.ServicesAvailable() //注册服务后，默认并不提供服务，调用此方法后才会正式提供服务。需要根据实际使用场景决定提供服务的时机。作用与java版本中的服务端心跳开关一致。
	time.Sleep(time.Second * 50000000)
}

//创建http服务器
func runTestHttpServer() {
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("a=11111"))
		fmt.Println("httpServer call success!!!")
	})
	http.ListenAndServe(":8105", nil)

}

type MotanDemoService struct{}

func (m *MotanDemoService) Hello(name string) string {
	fmt.Printf("MotanTestService hello:%s\n", name)
	return "hello " + name
}

type Motan2TestService struct{}

func (m *Motan2TestService) Hello(params map[string]string) string {
	if params == nil {
		return "params is nil!"
	}
	var buffer bytes.Buffer
	for k, v := range params {
		if buffer.Len() > 0 {
			buffer.WriteString(",")
		}
		buffer.WriteString(k)
		buffer.WriteString("=")
		buffer.WriteString(v)

	}
	fmt.Printf("HttpTestService1 hello:%s\n", buffer.String())
	return buffer.String()
}

//zha测试
type MotanTestService0 struct{}

func (m *MotanTestService0) MotanAddStr(nums map[string]string) string {
	fmt.Println("Server000 call success~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~")
	return nums["num1"] + nums["num2"]
}

type MotanTestService1 struct{}

func (m *MotanTestService1) MotanAddStr(nums map[string]string) string {
	fmt.Println("Server111 call success~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~")
	return nums["num1"] + nums["num2"]
}
