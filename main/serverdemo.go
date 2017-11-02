package main

import (
	"bytes"
	"fmt"
	"time"

	motan "github.com/weibocom/motan-go"
)

func main() {
	runServerDemo()
}

func runServerDemo() {
	mscontext := motan.GetMotanServerContext("./serverdemo.yaml")
	mscontext.RegisterService(&Motan2TestService{}, "")
	mscontext.RegisterService(&MotanDemoService{}, "")
	mscontext.Start(nil)
	time.Sleep(time.Second * 50000000)
}

type MotanDemoService struct{}

func (m *MotanDemoService) Hello(name string) string {
	fmt.Printf("MotanDemoService hello:%s\n", name)
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
	fmt.Printf("Motan2TestService hello:%s\n", buffer.String())
	return buffer.String()
}
