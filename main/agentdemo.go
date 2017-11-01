package main

import (
	"fmt"

	motan "github.com/weibocom/motan-go"
	motancore "github.com/weibocom/motan-go/core"
)

func main() {
	runAgentDemo()
}

func runAgentDemo() {
	agent := motan.NewAgent(nil)
	agent.ConfigFile = ".main/agentdemo.yaml"
	// you can registry custom extension implements to defaultExtFactory. extensions includes ha, lb, endpoint, regisry,filter
	// the default implements of extension is already registed in defaultExtFactory.
	weiboExtFactory := motan.GetDefaultExtFactory()
	weiboExtFactory.RegistExtFilter("myfilter", func() motancore.Filter {
		return &MyEndPointFilter{}
	})
	agent.StartMotanAgent()
}

// custom filter demo
type MyEndPointFilter struct {
	url  *motancore.Url
	next motancore.EndPointFilter
}

func (m *MyEndPointFilter) GetIndex() int {
	return 20
}

func (m *MyEndPointFilter) GetName() string {
	return "myfilter"
}

// create a new filter
func (m *MyEndPointFilter) NewFilter(url *motancore.Url) motancore.Filter {
	return &MyEndPointFilter{url: url}
}

func (m *MyEndPointFilter) Filter(caller motancore.Caller, request motancore.Request) motancore.Response {
	fmt.Printf("before call. request:%+v\n", request)
	// must call next filter in Filter implement
	response := m.GetNext().Filter(caller, request)
	fmt.Printf("after call. response:%+v\n", response)
	return response
}

func (m *MyEndPointFilter) HasNext() bool {
	return m.next != nil
}

func (m *MyEndPointFilter) SetNext(nextFilter motancore.EndPointFilter) {
	m.next = nextFilter
}

func (m *MyEndPointFilter) GetNext() motancore.EndPointFilter {
	return m.next
}

func (m *MyEndPointFilter) GetType() int32 {
	return motancore.EndPointFilterType
}
