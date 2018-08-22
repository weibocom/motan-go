package registry

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	motan "github.com/weibocom/motan-go/core"
)

var (
	//zk server url
	zkURL           = &motan.URL{Host: "127.0.0.1", Port: 2181}
	defaultWaitTime = 100 * time.Millisecond
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
	serverPath = zkRegistryNamespace + zkPathSeparator + testURL.Group + zkPathSeparator + testURL.Path + zkPathSeparator + zkNodeTypeServer + zkPathSeparator + testURL.Host + ":" + testURL.GetPortStr()
	//unavailableServerPath = "/motan/zkTestGroup/zkTestPath/unavailableServer/127.0.0.1:1234"
	unavailableServerPath = zkRegistryNamespace + zkPathSeparator + testURL.Group + zkPathSeparator + testURL.Path + zkPathSeparator + zkNodeTypeUnavailableServer + zkPathSeparator + testURL.Host + ":" + testURL.GetPortStr()
	//agentPath = "/motan/agent/zkTestApp/node/127.0.0.1:1234"
	agentPath = zkRegistryNamespace + zkPathSeparator + zkNodeTypeAgent + zkPathSeparator + testURL.GetParam(motan.ApplicationKey, "") + zkRegistryNode + zkPathSeparator + testURL.Host + ":" + testURL.GetPortStr()
	//commandPath = "/motan/zkTestGroup/command"
	commandPath = zkRegistryNamespace + zkPathSeparator + testURL.Group + zkRegistryCommand
	//agentCommandPath = "/motan/agent/zkTestApp/command"
	agentCommandPath = zkRegistryNamespace + zkPathSeparator + zkNodeTypeAgent + zkPathSeparator + testURL.GetParam(motan.ApplicationKey, "") + zkRegistryCommand
	z                = &ZkRegistry{}
)

//Test path generation methods.
func TestZkRegistryToPath(t *testing.T) {
	//Test path create methods.
	if p := toNodePath(testURL, zkNodeTypeServer); p != serverPath {
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
	assert.Equal(t, z.GetURL(), testURL)

	//Test GetName method.
	assert.Equal(t, z.GetName(), "zookeeper")
}

func TestZkRegistryBasic(t *testing.T) {
	if getZK() {
		//Test createNode method: server path.
		z.createNode(testURL, zkNodeTypeServer)
		time.Sleep(defaultWaitTime)
		isExist, _, err := z.zkConn.Exists(serverPath)
		if err != nil || !isExist {
			t.Error("Create server node fail. exist:", isExist, " err:", err)
		}

		//Test createNode method: agent path.
		z.createNode(testURL, zkNodeTypeAgent)
		time.Sleep(defaultWaitTime)
		isExist, _, err = z.zkConn.Exists(agentPath)
		if err != nil || !isExist {
			t.Error("Create agent node fail. exist:", isExist, " err:", err)
		}

		//Test Discover method.
		testURL.ClearCachedInfo()
		disURL := z.Discover(testURL)
		time.Sleep(defaultWaitTime)
		assert.Equal(t, disURL[0], testURL)

		//Test DiscoverCommand method.
		z.createPersistent(commandPath, true)
		commandReq := "hello"
		z.zkConn.Set(commandPath, []byte(commandReq), -1)
		commandRes := z.DiscoverCommand(testURL)
		time.Sleep(defaultWaitTime)
		assert.Equal(t, commandReq, commandRes)

		//Test DiscoverCommand method.
		z.createPersistent(agentCommandPath, true)
		z.zkConn.Set(agentCommandPath, []byte(commandReq), -1)
		testURL.PutParam("nodeType", zkNodeTypeAgent)
		commandRes = z.DiscoverCommand(testURL)
		testURL.PutParam("nodeType", "")
		time.Sleep(defaultWaitTime)
		assert.Equal(t, commandReq, commandRes)

		//Test removeNode method.
		z.removeNode(testURL, zkNodeTypeServer)
		time.Sleep(defaultWaitTime)
		if isExist, _, err := z.zkConn.Exists(serverPath); err == nil {
			if isExist {
				t.Error("removeNode fail.")
			}
		} else {
			t.Error("removeNode err:", err)
		}
	}
}

func TestZkRegistryRegister(t *testing.T) {
	if getZK() {
		//Test Available method: with parameter.
		z.Register(testURL)
		z.Available(testURL)
		time.Sleep(defaultWaitTime)
		if isExist, _, err := z.zkConn.Exists(serverPath); err == nil {
			if !isExist {
				t.Error("Register fail.")
			}
		} else {
			t.Error("Register err:", err)
		}

		//Test Unavailable method: without parameter.
		z.Unavailable(testURL)
		time.Sleep(defaultWaitTime)
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
		time.Sleep(defaultWaitTime)
		if isExist, _, err := z.zkConn.Exists(serverPath); err == nil {
			if !isExist {
				t.Error("Register fail.")
			}
		} else {
			t.Error("Register err:", err)
		}

		//Test Unavailable method: with parameter.
		z.Unavailable(nil)
		time.Sleep(defaultWaitTime)
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

func TestZkRegistrySubscribe(t *testing.T) {
	if getZK() {
		//Test Register method.
		z.Register(testURL)
		time.Sleep(defaultWaitTime)
		if isExist, _, err := z.zkConn.Exists(unavailableServerPath); !isExist || err != nil {
			t.Error("Register fail:", err)
		}
		testURL.PutParam("nodeType", zkNodeTypeAgent)
		testURL.Group = "agent" //build different urlID
		testURL.ClearCachedInfo()
		z.Register(testURL)
		testURL.Group = "zkTestGroup" //revert urlID
		testURL.ClearCachedInfo()
		if isExist, _, err := z.zkConn.Exists(agentPath); !isExist || err != nil {
			t.Error("Register fail:", err)
		}
		testURL.PutParam("nodeType", "")

		//Test GetRegisteredServices method.
		assert.Equal(t, z.GetRegisteredServices()[0], testURL)

		//Test Subscribe method.
		lis := MockListener{registryURL: &motan.URL{}}
		z.Subscribe(testURL, &lis)
		z.createNode(testURL, zkNodeTypeServer)
		time.Sleep(defaultWaitTime)
		urlRes := &motan.URL{
			Host: zkURL.Host,
			Port: zkURL.Port,
		}
		lis.registryURL.ClearCachedInfo()
		time.Sleep(defaultWaitTime)
		assert.Equal(t, urlRes, lis.registryURL)

		//Test UnSubscribe method.
		lis = MockListener{}
		z.Unsubscribe(testURL, &lis)
		time.Sleep(defaultWaitTime)
		if listeners, ok := z.subscribedServiceMap[serverPath]; ok {
			if _, ok := listeners[&lis]; ok {
				t.Error("UnSubscribe fail. registryURL:", lis.registryURL)
			}
		}

		//Test SubscribeCommand method: service command path.
		lis = MockListener{}
		z.createPersistent(commandPath, true)
		z.SubscribeCommand(testURL, &lis)
		commandReq := "hello"
		z.zkConn.Set(commandPath, []byte(commandReq), -1)
		time.Sleep(defaultWaitTime)
		//assert.Equal(t, commandReq, lis.command)

		//Test SubscribeCommand method: agent command path.
		lis = MockListener{}
		testURL.PutParam("nodeType", zkNodeTypeAgent)
		testURL.Group = "agentCommand" //build different urlID
		testURL.ClearCachedInfo()
		z.createPersistent(agentCommandPath, true)
		z.SubscribeCommand(testURL, &lis)
		testURL.Group = "zkTestGroup" //revert urlID
		testURL.ClearCachedInfo()
		testURL.PutParam("nodeType", "")
		z.zkConn.Set(agentCommandPath, []byte(commandReq), -1)
		time.Sleep(defaultWaitTime)
		assert.Equal(t, commandReq, lis.command)

		//Test UnSubscribeCommand method: service command path.
		z.UnSubscribeCommand(testURL, &lis)
		time.Sleep(defaultWaitTime)
		if _, ok := z.switcherMap[commandPath]; ok {
			t.Error("UnSubscribe command fail.")
		}

		//Test UnSubscribeCommand method: agent command path.
		testURL.PutParam("nodeType", zkNodeTypeAgent)
		z.UnSubscribeCommand(testURL, &lis)
		testURL.PutParam("nodeType", "")
		time.Sleep(defaultWaitTime)
		if _, ok := z.switcherMap[commandPath]; ok {
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

func getZK() bool {
	tcpAddr, _ := net.ResolveTCPAddr("tcp4", zkURL.GetAddressStr())
	if _, err := net.DialTCP("tcp", nil, tcpAddr); err == nil {
		if !z.IsAvailable() {
			z.url = zkURL
			z.Initialize()
		}
		return true
	}
	return false
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
