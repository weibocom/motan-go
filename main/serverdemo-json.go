package main

import (
	"fmt"
	"strconv"
	"time"

	motan "github.com/weibocom/motan-go"
)

func main() {
	runServerDemo()
}

func runServerDemo() {
	mscontext := motan.GetMotanServerContext("./serverdemo-json.yaml")
	mscontext.RegisterService(&MotanDemoService{}, "")
	mscontext.Start(nil)
	time.Sleep(time.Second * 50000000)
}

type MotanDemoService struct{}

type People struct {
	Name string
	Age  int
}

func (m *MotanDemoService) Hello(name *People, age *People) string {
	fmt.Printf("MotanDemoService hello:%s\n", name.Name)
	return "[ From Server ] Hello " + name.Name + ", I'm " + strconv.Itoa(age.Age)
}
