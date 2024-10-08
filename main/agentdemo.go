package main

import (
	"fmt"
	"github.com/weibocom/motan-go/filter"
	"net/http"
	"net/http/httputil"

	motan "github.com/weibocom/motan-go"

	motancore "github.com/weibocom/motan-go/core"
)

func main() {
	runAgentDemo()
}

func runAgentDemo() {
	go func() {
		var addr = ":9090"
		handler := &http.ServeMux{}
		handler.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Add("server-address", motancore.GetLocalIP()+addr)
			_, _ = fmt.Fprint(writer, "request url: "+request.URL.String()+"\n")
			rawRequestBytes, err := httputil.DumpRequest(request, true)
			if err != nil {
				http.Error(writer, fmt.Sprint(err), http.StatusInternalServerError)
				return
			}
			fmt.Println(string(rawRequestBytes))
		})
		http.ListenAndServe(addr, handler)
	}()

	agent := motan.NewAgent(nil)
	agent.ConfigFile = "./agentdemo.yaml"
	// you can registry custom extension implements to defaultExtFactory. extensions includes ha, lb, endpoint, regisry,filter
	// the default implements of extension is already registered in defaultExtFactory.
	weiboExtFactory := motan.GetDefaultExtFactory()
	weiboExtFactory.RegistExtFilter("myfilter", func() motancore.Filter {
		return &MyEndPointFilter{}
	})
	agent.StartMotanAgent()
}

// MyEndPointFilter is a custom filter demo
type MyEndPointFilter struct {
	url  *motancore.URL
	next motancore.EndPointFilter
}

func (m *MyEndPointFilter) GetRuntimeInfo() map[string]interface{} {
	return filter.GetFilterRuntimeInfo(m)
}

func (m *MyEndPointFilter) GetIndex() int {
	return 20
}

func (m *MyEndPointFilter) GetName() string {
	return "myfilter"
}

// NewFilter create a new filter instance
func (m *MyEndPointFilter) NewFilter(url *motancore.URL) motancore.Filter {
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
