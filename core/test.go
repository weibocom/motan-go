package core

import "fmt"

//structs for test
type TestFilter struct {
	Index int
	Url   *Url
	next  ClusterFilter
}

func (t *TestFilter) GetName() string {
	return "TestFilter"
}
func (t *TestFilter) NewFilter(url *Url) Filter {
	//init with url in here
	return &TestFilter{Url: url}
}

func (t *TestFilter) Filter(haStrategy HaStrategy, loadBalance LoadBalance, request Request) Response {
	fmt.Println("do mock in testfilter with cluster mode")

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
	Url   *Url
	next  EndPointFilter
}

func (t *TestEndPointFilter) GetName() string {
	return "TestEndPointFilter"
}
func (t *TestEndPointFilter) NewFilter(url *Url) Filter {
	//init with url in here
	return &TestEndPointFilter{Url: url}
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
	Url *Url
}

func (t *TestEndPoint) GetUrl() *Url {
	return t.Url
}
func (t *TestEndPoint) SetUrl(url *Url) {
	t.Url = url
}
func (t *TestEndPoint) GetName() string {
	return "testEndPoint"
}
func (t *TestEndPoint) Call(request Request) Response {
	fmt.Println("mock rpc request..")
	response := &MotanResponse{RequestId: request.GetRequestId(), Value: &TestObject{}, ProcessTime: 12}
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
	Url *Url
}

func (t *TestHaStrategy) GetUrl() *Url {
	return t.Url
}
func (t *TestHaStrategy) SetUrl(url *Url) {
	t.Url = url
}
func (t *TestHaStrategy) Call(request Request, loadBalance LoadBalance) Response {
	fmt.Println("in testHastrategy call")
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
	fmt.Println("in testLoadbalance select")
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
	Url *Url
}

func (t *TestRegistry) GetName() string {
	return "testRegistry"
}
func (t *TestRegistry) Subscribe(url *Url, listener NotifyListener) {

}
func (t *TestRegistry) Unsubscribe(url *Url, listener NotifyListener) {

}
func (t *TestRegistry) Discover(url *Url) []*Url {
	return make([]*Url, 0)
}
func (t *TestRegistry) Register(serverUrl *Url) {

}
func (t *TestRegistry) UnRegister(serverUrl *Url) {

}
func (t *TestRegistry) Available(serverUrl *Url) {

}
func (t *TestRegistry) Unavailable(serverUrl *Url) {

}
func (t *TestRegistry) GetRegisteredServices() []*Url {
	return make([]*Url, 0)
}
func (t *TestRegistry) GetUrl() *Url {
	if t.Url == nil {
		t.Url = &Url{}
	}
	return t.Url
}
func (t *TestRegistry) SetUrl(url *Url) {
	t.Url = url
}
func (t *TestRegistry) InitRegistry() {

}
func (t *TestRegistry) StartSnapshot(conf *SnapshotConf) {
}
