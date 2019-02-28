package core

import (
	"errors"
	"fmt"
	"time"
)

//structs for test

type TestFilter struct {
	Index int
	URL   *URL
	next  ClusterFilter
}

func (t *TestFilter) GetName() string {
	return "TestFilter"
}
func (t *TestFilter) NewFilter(url *URL) Filter {
	//init with url in here
	return &TestFilter{URL: url}
}

func (t *TestFilter) Filter(haStrategy HaStrategy, loadBalance LoadBalance, request Request) Response {
	fmt.Println("do mock in testFilter with cluster mode")

	return t.GetNext().Filter(haStrategy, loadBalance, request)

}
func (t *TestFilter) HasNext() bool {
	return t.next != nil
}
func (t *TestFilter) SetNext(nextFilter ClusterFilter) {
	t.next = nextFilter
}
func (t *TestFilter) GetNext() ClusterFilter {
	return t.next
}
func (t *TestFilter) GetIndex() int {
	return t.Index
}
func (t *TestFilter) GetType() int32 {
	return ClusterFilterType
}

type TestEndPointFilter struct {
	Index int
	URL   *URL
	next  EndPointFilter
}

func (t *TestEndPointFilter) GetName() string {
	return "TestEndPointFilter"
}
func (t *TestEndPointFilter) NewFilter(url *URL) Filter {
	//init with url in here
	return &TestEndPointFilter{URL: url}
}

func (t *TestEndPointFilter) Filter(caller Caller, request Request) Response {
	fmt.Println("do mock in TestEndPointFilter with endpoint mode")
	//start
	response := t.GetNext().Filter(caller, request)
	//end
	return response

}

func (t *TestEndPointFilter) HasNext() bool {
	return t.next != nil
}
func (t *TestEndPointFilter) SetNext(nextFilter EndPointFilter) {
	t.next = nextFilter
}
func (t *TestEndPointFilter) GetNext() EndPointFilter {
	return t.next
}
func (t *TestEndPointFilter) GetIndex() int {
	return t.Index
}
func (t *TestEndPointFilter) GetType() int32 {
	return EndPointFilterType
}

type TestEndPoint struct {
	URL         *URL
	ProcessTime int64
}

func (t *TestEndPoint) GetURL() *URL {
	return t.URL
}
func (t *TestEndPoint) SetURL(url *URL) {
	t.URL = url
}
func (t *TestEndPoint) GetName() string {
	return "testEndPoint"
}
func (t *TestEndPoint) Call(request Request) Response {
	fmt.Println("mock rpc request..")
	if t.ProcessTime != 0 {
		time.Sleep(time.Duration(t.ProcessTime) * time.Millisecond)
	}
	response := &MotanResponse{RequestID: request.GetRequestID(), Value: &TestObject{}, ProcessTime: t.ProcessTime}
	return response
}

func (t *TestEndPoint) IsAvailable() bool {
	return true
}

func (t *TestEndPoint) Destroy() {}

func (t *TestEndPoint) SetProxy(proxy bool) {}

func (t *TestEndPoint) SetSerialization(s Serialization) {}

type TestObject struct {
	Str string
}

type TestHaStrategy struct {
	URL *URL
}

func (t *TestHaStrategy) GetName() string {
	return "TestHaStrategy"
}

func (t *TestHaStrategy) GetURL() *URL {
	return t.URL
}
func (t *TestHaStrategy) SetURL(url *URL) {
	t.URL = url
}
func (t *TestHaStrategy) Call(request Request, loadBalance LoadBalance) Response {
	fmt.Println("in testHaStrategy call")
	refer := loadBalance.Select(request)
	return refer.Call(request)
}

type TestLoadBalance struct {
	Endpoints []EndPoint
}

func (t *TestLoadBalance) OnRefresh(endpoints []EndPoint) {
	t.Endpoints = endpoints
}
func (t *TestLoadBalance) Select(request Request) EndPoint {
	fmt.Println("in testLoadBalance select")
	endpoint := &TestEndPoint{}
	filterEndPoint := &FilterEndPoint{}
	efilter1 := &TestEndPointFilter{}
	efilter2 := &TestEndPointFilter{}
	efilter1.SetNext(efilter2)
	efilter2.SetNext(GetLastEndPointFilter())
	filterEndPoint.Caller = endpoint
	filterEndPoint.Filter = efilter1
	return filterEndPoint
}
func (t *TestLoadBalance) SelectArray(request Request) []EndPoint {
	return []EndPoint{&TestEndPoint{}}
}
func (t *TestLoadBalance) SetWeight(weight string) {

}

type TestRegistry struct {
	URL           *URL
	GroupService  map[string][]string
	DiscoverError bool
}

func (t *TestRegistry) GetName() string {
	return "testRegistry"
}
func (t *TestRegistry) Subscribe(url *URL, listener NotifyListener) {

}
func (t *TestRegistry) Unsubscribe(url *URL, listener NotifyListener) {

}
func (t *TestRegistry) Discover(url *URL) []*URL {
	return make([]*URL, 0)
}
func (t *TestRegistry) Register(serverURL *URL) {

}
func (t *TestRegistry) UnRegister(serverURL *URL) {

}
func (t *TestRegistry) Available(serverURL *URL) {

}
func (t *TestRegistry) Unavailable(serverURL *URL) {

}
func (t *TestRegistry) GetRegisteredServices() []*URL {
	return make([]*URL, 0)
}
func (t *TestRegistry) GetURL() *URL {
	if t.URL == nil {
		t.URL = &URL{}
	}
	return t.URL
}
func (t *TestRegistry) SetURL(url *URL) {
	t.URL = url
}
func (t *TestRegistry) InitRegistry() {

}
func (t *TestRegistry) StartSnapshot(conf *SnapshotConf) {
}

func (t *TestRegistry) DiscoverAllServices(group string) ([]string, error) {
	if t.DiscoverError {
		return nil, errors.New("service discover error")
	}
	return t.GroupService[group], nil
}

func (t *TestRegistry) DiscoverAllGroups() ([]string, error) {
	if t.DiscoverError {
		return nil, errors.New("group discover error")
	}
	groups := make([]string, 0, len(t.GroupService))
	for k := range t.GroupService {
		groups = append(groups, k)
	}
	return groups, nil
}
