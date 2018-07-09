package registry

import (
	motan "github.com/weibocom/motan-go/core"
	"testing"
	"reflect"
	"time"
	"net"
	"sync"
)

var (
	//zk server url
	zkURL = &motan.URL{Host: "127.0.0.1", Port: 2181}
	//unified test url
	testURL = &motan.URL{
		Protocol:   "zookeeper",
		Group:      "zkTestGroup",
		Path:       "zkTestPath",
		Host:       "127.0.0.1",
		Port:       1234,
		Parameters: map[string]string{motan.ApplicationKey: "zkTestApp"},
	}
	//serverPath = "/motan/zkTestGroup/zkTestPath/server/127.0.0.1:1234"
	serverPath = ZkRegistryNamespace + ZkPathSeparator + testURL.Group + ZkPathSeparator + testURL.Path + ZkPathSeparator + ZkNodeTypeServer + ZkPathSeparator + testURL.Host + ":" + testURL.GetPortStr()
	//unavailableServerPath = "/motan/zkTestGroup/zkTestPath/unavailableServer/127.0.0.1:1234"
	unavailableServerPath = ZkRegistryNamespace + ZkPathSeparator + testURL.Group + ZkPathSeparator + testURL.Path + ZkPathSeparator + ZkNodeTypeUnavailbleServer + ZkPathSeparator + testURL.Host + ":" + testURL.GetPortStr()
	//agentPath = "/motan/agent/zkTestApp/node/127.0.0.1:1234"
	agentPath = ZkRegistryNamespace + ZkPathSeparator + ZkNodeTypeAgent + ZkPathSeparator + testURL.GetParam(motan.ApplicationKey, "") + ZkRegistryNode + ZkPathSeparator + testURL.Host + ":" + testURL.GetPortStr()
	//commandPath = "/motan/zkTestGroup/command"
	commandPath = ZkRegistryNamespace + ZkPathSeparator + testURL.Group + ZkRegistryCommand
	//agentCommandPath = "/motan/agent/zkTestApp/command"
	agentCommandPath = ZkRegistryNamespace + ZkPathSeparator + ZkNodeTypeAgent + ZkPathSeparator + testURL.GetParam(motan.ApplicationKey, "") + ZkRegistryCommand
	z                = &ZkRegistry{}
	hasZKServer      = false
	once             sync.Once
)

//Test path generation methods.
func TestZkRegistryToPath(t *testing.T) {
	//Test path create methods.
	if p := toNodePath(testURL, ZkNodeTypeServer); p != serverPath {
		t.Error("toNodePath err. result:", p)
	}
	if p := toCommandPath(testURL); p != commandPath {
		t.Error("toCommandPath err. result:", p)
	}
	if p := toAgentNodePath(testURL); p != agentPath {
		t.Error("toAgentNodePath err. result:", p)
	}
	if p := toAgentCommandPath(testURL); p != agentCommandPath {
		t.Error("toAgentCommandPath err. result:", p)
	}

	//Test SetURL method and GetURL method.
	z.SetURL(testURL)
	if !reflect.DeepEqual(z.GetURL(), testURL) {
		t.Error("GetURL fail. set:", testURL, "get:", z.GetURL())
	}

	//Test GetName method.
	if !reflect.DeepEqual(z.GetName(), "zookeeper") {
		t.Error("GetName fail:", z.GetName())
	}
}

func TestZkRegistryBasic(t *testing.T) {
	if once.Do(initZK); hasZKServer {
		//Test createNode method: server path.
		z.createNode(testURL, ZkNodeTypeServer)
		isExist, _, err := z.zkConn.Exists(serverPath)
		if err != nil || !isExist {
			t.Error("Create server node fail. exist:", isExist, " err:", err)
		}

		//Test createNode method: agent path.
		z.createNode(testURL, ZkNodeTypeAgent)
		isExist, _, err = z.zkConn.Exists(agentPath)
		if err != nil || !isExist {
			t.Error("Create agent node fail. exist:", isExist, " err:", err)
		}

		//Test Discover method.
		testURL.ClearCachedInfo()
		if !reflect.DeepEqual(z.Discover(testURL)[0], testURL) {
			t.Error("Discover fail:", z.Discover(testURL)[0], testURL)
		}

		//Test DiscoverCommand method.
		z.createPersistent(commandPath, true)
		commandReq := "hello"
		z.zkConn.Set(commandPath, []byte(commandReq), -1)
		commandRes := z.DiscoverCommand(testURL)
		if !reflect.DeepEqual(commandReq, commandRes) {
			t.Error("Discover command fail. commandReq:", commandReq, "commandRes:", commandRes)
		}

		//Test DiscoverCommand method.
		z.createPersistent(agentCommandPath, true)
		z.zkConn.Set(agentCommandPath, []byte(commandReq), -1)
		testURL.PutParam("nodeType", ZkNodeTypeAgent)
		commandRes = z.DiscoverCommand(testURL)
		testURL.PutParam("nodeType", "")
		if !reflect.DeepEqual(commandReq, commandRes) {
			t.Error("Discover command fail. commandReq:", commandReq, "commandRes:", commandRes)
		}

		//Test removeNode method.
		z.removeNode(testURL, ZkNodeTypeServer)
		if isExist, _, err := z.zkConn.Exists(serverPath); err == nil {
			if isExist {
				t.Error("removeNode fail.")
			}
		} else {
			t.Error("removeNode err:", err)
		}
	}
}

func TestZkRegistryAvailable(t *testing.T) {
	if once.Do(initZK); hasZKServer {
		//Test Available method: with parameter.
		z.Register(testURL)
		z.Available(testURL)
		if isExist, _, err := z.zkConn.Exists(serverPath); err == nil {
			if !isExist {
				t.Error("Register fail.")
			}
		} else {
			t.Error("Register err:", err)
		}

		//Test Unavailable method: without parameter.
		z.Unavailable(testURL)
		isExistUnAvail, _, errUnAvail := z.zkConn.Exists(unavailableServerPath)
		isExistAvail, _, errAvail := z.zkConn.Exists(serverPath)
		if errUnAvail == nil && errAvail == nil {
			if !isExistUnAvail || isExistAvail {
				t.Error("Unavailable fail.")
			}
		} else {
			t.Error("Unavailable err:", errUnAvail, errAvail)
		}

		//Test Available method: without parameter.
		z.Register(testURL)
		z.Available(nil)
		if isExist, _, err := z.zkConn.Exists(serverPath); err == nil {
			if !isExist {
				t.Error("Register fail.")
			}
		} else {
			t.Error("Register err:", err)
		}

		//Test Unavailable method: with parameter.
		z.Unavailable(nil)
		isExistUnAvail, _, errUnAvail = z.zkConn.Exists(unavailableServerPath)
		isExistAvail, _, errAvail = z.zkConn.Exists(serverPath)
		if errUnAvail == nil && errAvail == nil {
			if !isExistUnAvail || isExistAvail {
				t.Error("Unavailable fail.")
			}
		} else {
			t.Error("Unavailable err:", errUnAvail, errAvail)
		}
	}
}

func TestZkRegistryRegister(t *testing.T) {
	if once.Do(initZK); hasZKServer {
		//Test Register method.
		z.Register(testURL)
		if isExist, _, err := z.zkConn.Exists(unavailableServerPath); !isExist || err != nil {
			t.Error("Register fail:", err)
		}
		testURL.PutParam("nodeType", ZkNodeTypeAgent)
		z.Register(testURL)
		if isExist, _, err := z.zkConn.Exists(agentPath); !isExist || err != nil {
			t.Error("Register fail:", err)
		}
		testURL.PutParam("nodeType", "")

		//Test GetRegisteredServices method.
		if !reflect.DeepEqual(z.GetRegisteredServices()[0], testURL) {
			t.Error("GetRegisteredServices fail. get:", *z.GetRegisteredServices()[0])
		}

		//Test Subscribe method.
		lis := MockListener{}
		z.Subscribe(testURL, &lis)
		z.createNode(testURL, ZkNodeTypeServer)
		time.Sleep(10 * time.Millisecond)
		urlRes := &motan.URL{
			Host: zkURL.Host,
			Port: zkURL.Port,
		}
		lis.registryURL.ClearCachedInfo()
		time.Sleep(10 * time.Millisecond)
		if !reflect.DeepEqual(lis.registryURL, urlRes) {
			t.Error("Subscribe fail. registryURL:", lis.registryURL)
		}

		//Test UnSubscribe method.
		lis = MockListener{}
		z.Unsubscribe(testURL, &lis)
		subKey := GetSubKey(testURL)
		idt := lis.GetIdentity()
		time.Sleep(10 * time.Millisecond)
		if listeners, ok := z.subscribeMap[subKey]; ok {
			if _, ok := listeners[idt]; ok {
				t.Error("UnSubscribe fail. registryURL:", lis.registryURL)
			}
		}

		//Test SubscribeCommand method: service command path.
		lis = MockListener{}
		z.createPersistent(commandPath, true)
		z.SubscribeCommand(testURL, &lis)
		commandReq := "hello"
		z.zkConn.Set(commandPath, []byte(commandReq), -1)
		time.Sleep(10 * time.Millisecond)
		if !reflect.DeepEqual(commandReq, lis.command) {
			t.Error("Subscribe command fail. commandReq:", commandReq, "lis.command:", lis.command)
		}

		//Test SubscribeCommand method: agent command path.
		lis = MockListener{}
		testURL.PutParam("nodeType", ZkNodeTypeAgent)
		z.createPersistent(agentCommandPath, true)
		z.SubscribeCommand(testURL, &lis)
		testURL.PutParam("nodeType", "")
		z.zkConn.Set(agentCommandPath, []byte(commandReq), -1)
		time.Sleep(10 * time.Millisecond)
		if !reflect.DeepEqual(commandReq, lis.command) {
			t.Error("Subscribe agent command fail. commandReq:", commandReq, "lis.command:", lis.command)
		}

		//Test UnSubscribeCommand method: service command path.
		z.UnSubscribeCommand(testURL, &lis)
		time.Sleep(10 * time.Millisecond)
		if _, ok := <-z.watchSwitcherMap[commandPath]; ok {
			t.Error("UnSubscribe command fail.")
		}

		//Test UnSubscribeCommand method: agent command path.
		testURL.PutParam("nodeType", ZkNodeTypeAgent)
		z.UnSubscribeCommand(testURL, &lis)
		testURL.PutParam("nodeType", "")
		time.Sleep(10 * time.Millisecond)
		if _, ok := <-z.watchSwitcherMap[commandPath]; ok {
			t.Error("UnSubscribe command fail.")
		}

		//Test UnRegister method.
		z.UnRegister(testURL)
		isExistUnReg, _, errUnReg := z.zkConn.Exists(unavailableServerPath)
		isExistGeg, _, errReg := z.zkConn.Exists(serverPath)
		if errUnReg == nil && errReg == nil {
			if isExistUnReg || isExistGeg {
				t.Error("UnRegister fail.")
			}
		} else {
			t.Error("UnRegister err:", errUnReg, errReg)
		}
	}
}

func initZK() {
	tcpAddr, _ := net.ResolveTCPAddr("tcp4", zkURL.GetAddressStr())
	if _, err := net.DialTCP("tcp", nil, tcpAddr); err == nil {
		z = &ZkRegistry{url: zkURL}
		z.Initialize()
		hasZKServer = true
	}
}

type MockListener struct {
	registryURL *motan.URL
	urls        []*motan.URL
	command     string
}

func (m *MockListener) Notify(registryURL *motan.URL, urls []*motan.URL) {
	m.registryURL = registryURL
	m.urls = urls
}

func (m *MockListener) NotifyCommand(registryURL *motan.URL, commandType int, commandInfo string) {
	m.registryURL = registryURL
	m.command = commandInfo
}

func (m *MockListener) GetIdentity() string {
	return "mocklistener"
}
