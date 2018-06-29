package registry

import (
	motan "github.com/weibocom/motan-go/core"
	"testing"
	"reflect"
	"time"
	"net"
)

var (
	ZkURL = &motan.URL{
		Protocol:   "zookeeper",
		Group:      "zkTestGroup",
		Path:       "zkTestPath",
		Host:       "127.0.0.1",
		Port:       2181,
		Parameters: map[string]string{motan.ApplicationKey: "zkTestApp"},
	}
	//ServerPath = "/motan/zkTestGroup/zkTestPath/server/127.0.0.1:2181"
	ServerPath = ZkRegistryNamespace + ZkPathSeparator + ZkURL.Group + ZkPathSeparator + ZkURL.Path + ZkPathSeparator + ZkNodeTypeServer + ZkPathSeparator + ZkURL.Host + ":" + ZkURL.GetPortStr()
	//UnavailableServerPath = "/motan/zkTestGroup/zkTestPath/unavailableServer/127.0.0.1:2181"
	UnavailableServerPath = ZkRegistryNamespace + ZkPathSeparator + ZkURL.Group + ZkPathSeparator + ZkURL.Path + ZkPathSeparator + ZkNodeTypeUnavailbleServer + ZkPathSeparator + ZkURL.Host + ":" + ZkURL.GetPortStr()
	//AgentPath = "/motan/agent/zkTestApp/node/127.0.0.1:2181"
	AgentPath = ZkRegistryNamespace + ZkPathSeparator + ZkNodeTypeAgent + ZkPathSeparator + ZkURL.GetParam(motan.ApplicationKey, "") + ZkRegistryNode + ZkPathSeparator + ZkURL.Host + ":" + ZkURL.GetPortStr()
	//CommandPath = "/motan/zkTestGroup/command"
	CommandPath = ZkRegistryNamespace + ZkPathSeparator + ZkURL.Group + ZkRegistryCommand
	//AgentCommandPath = "/motan/agent/zkTestApp/command"
	AgentCommandPath = ZkRegistryNamespace + ZkPathSeparator + ZkNodeTypeAgent + ZkPathSeparator + ZkURL.GetParam(motan.ApplicationKey, "") + ZkRegistryCommand
)

//Test path generation methods.
func TestZkRegistryToPath(t *testing.T) {
	z := ZkRegistry{}
	//Test path create methods.
	if p := toNodePath(ZkURL, ZkNodeTypeServer); p != ServerPath {
		t.Error("toNodePath err. result:", p)
	}
	if p := toCommandPath(ZkURL); p != CommandPath {
		t.Error("toCommandPath err. result:", p)
	}
	if p := toAgentNodePath(ZkURL); p != AgentPath {
		t.Error("toAgentNodePath err. result:", p)
	}
	if p := toAgentCommandPath(ZkURL); p != AgentCommandPath {
		t.Error("toAgentCommandPath err. result:", p)
	}

	//Test SetURL method and GetURL method.
	z.SetURL(ZkURL)
	if !reflect.DeepEqual(z.GetURL(), ZkURL) {
		t.Error("GetURL fail. set:", ZkURL, "get:", z.GetURL())
	}

	//Test GetName method.
	if !reflect.DeepEqual(z.GetName(), "zookeeper") {
		t.Error("GetName fail:", z.GetName())
	}
}

func TestZkRegistryBasic(t *testing.T) {
	if z, ok := hasZKServer(*ZkURL); ok {
		//Test createNode method: server path.
		z.createNode(ZkURL, ZkNodeTypeServer)
		isExist, _, err := z.zkConn.Exists(ServerPath)
		if err != nil || !isExist {
			t.Error("Create server node fail. exist:", isExist, " err:", err)
		}

		//Test createNode method: agent path.
		z.createNode(ZkURL, ZkNodeTypeAgent)
		isExist, _, err = z.zkConn.Exists(AgentPath)
		if err != nil || !isExist {
			t.Error("Create agent node fail. exist:", isExist, " err:", err)
		}

		//Test Discover method.
		ZkURL.ClearCachedInfo()
		if !reflect.DeepEqual(z.Discover(ZkURL)[0], ZkURL) {
			t.Error("Discover fail:", z.Discover(ZkURL)[0], ZkURL)
		}

		//Test DiscoverCommand method.
		z.createPersistent(CommandPath, true)
		commandReq := "hello"
		z.zkConn.Set(CommandPath, []byte(commandReq), -1)
		commandRes := z.DiscoverCommand(ZkURL)
		if !reflect.DeepEqual(commandReq, commandRes) {
			t.Error("Discover command fail. commandReq:", commandReq, "commandRes:", commandRes)
		}

		//Test DiscoverCommand method.
		z.createPersistent(AgentCommandPath, true)
		z.zkConn.Set(AgentCommandPath, []byte(commandReq), -1)
		ZkURL.PutParam("nodeType", ZkNodeTypeAgent)
		commandRes = z.DiscoverCommand(ZkURL)
		ZkURL.PutParam("nodeType", "")
		if !reflect.DeepEqual(commandReq, commandRes) {
			t.Error("Discover command fail. commandReq:", commandReq, "commandRes:", commandRes)
		}

		//Test removeNode method.
		z.removeNode(ZkURL, ZkNodeTypeServer)
		if isExist, _, err := z.zkConn.Exists(ServerPath); err == nil {
			if isExist {
				t.Error("removeNode fail.")
			}
		} else {
			t.Error("removeNode err:", err)
		}
	}
}

func TestZkRegistryAvailable(t *testing.T) {
	if z, ok := hasZKServer(*ZkURL); ok {
		//Test Available method: with parameter.
		z.Register(ZkURL)
		z.Available(ZkURL)
		if isExist, _, err := z.zkConn.Exists(ServerPath); err == nil {
			if !isExist {
				t.Error("Register fail.")
			}
		} else {
			t.Error("Register err:", err)
		}

		//Test Unavailable method: without parameter.
		z.Unavailable(ZkURL)
		isExistUnAvail, _, errUnAvail := z.zkConn.Exists(UnavailableServerPath)
		isExistAvail, _, errAvail := z.zkConn.Exists(ServerPath)
		if errUnAvail == nil && errAvail == nil {
			if !isExistUnAvail || isExistAvail {
				t.Error("Unavailable fail.")
			}
		} else {
			t.Error("Unavailable err:", errUnAvail, errAvail)
		}

		//Test Available method: without parameter.
		z.Register(ZkURL)
		z.Available(nil)
		if isExist, _, err := z.zkConn.Exists(ServerPath); err == nil {
			if !isExist {
				t.Error("Register fail.")
			}
		} else {
			t.Error("Register err:", err)
		}

		//Test Unavailable method: with parameter.
		z.Unavailable(nil)
		isExistUnAvail, _, errUnAvail = z.zkConn.Exists(UnavailableServerPath)
		isExistAvail, _, errAvail = z.zkConn.Exists(ServerPath)
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
	if z, ok := hasZKServer(*ZkURL); ok {
		//Test Register method.
		z.Register(ZkURL)
		if isExist, _, err := z.zkConn.Exists(UnavailableServerPath); !isExist || err != nil {
			t.Error("Register fail:", err)
		}
		ZkURL.PutParam("nodeType", ZkNodeTypeAgent)
		z.Register(ZkURL)
		if isExist, _, err := z.zkConn.Exists(AgentPath); !isExist || err != nil {
			t.Error("Register fail:", err)
		}
		ZkURL.PutParam("nodeType", "")

		//Test GetRegisteredServices method.
		if !reflect.DeepEqual(z.GetRegisteredServices()[0], ZkURL) {
			t.Error("GetRegisteredServices fail. get:", *z.GetRegisteredServices()[0])
		}

		//Test Subscribe method.
		lis := MockListener{}
		z.Subscribe(ZkURL, &lis)
		z.createNode(ZkURL, ZkNodeTypeServer)
		time.Sleep(10 * time.Millisecond)
		urlRes := &motan.URL{
			Protocol:   ZkURL.Protocol,
			Host:       ZkURL.Host,
			Port:       ZkURL.Port,
			Path:       ZkURL.Path,
			Group:      ZkURL.Group,
			Parameters: map[string]string{motan.ApplicationKey: ZkURL.GetParam(motan.ApplicationKey, ""), "nodeType": "referer"},
		}
		lis.registryURL.ClearCachedInfo()
		time.Sleep(10 * time.Millisecond)
		if !reflect.DeepEqual(lis.registryURL, urlRes) {
			t.Error("Subscribe fail. registryURL:", lis.registryURL)
		}

		//Test UnSubscribe method.
		lis = MockListener{}
		z.Unsubscribe(ZkURL, &lis)
		subKey := GetSubKey(ZkURL)
		idt := lis.GetIdentity()
		time.Sleep(10 * time.Millisecond)
		if listeners, ok := z.subscribeMap[subKey]; ok {
			if _, ok := listeners[idt]; ok {
				t.Error("UnSubscribe fail. registryURL:", lis.registryURL)
			}
		}

		//Test SubscribeCommand method: service command path.
		lis = MockListener{}
		z.createPersistent(CommandPath, true)
		z.SubscribeCommand(ZkURL, &lis)
		commandReq := "hello"
		z.zkConn.Set(CommandPath, []byte(commandReq), -1)
		time.Sleep(10 * time.Millisecond)
		if !reflect.DeepEqual(commandReq, lis.command) {
			t.Error("Subscribe command fail. commandReq:", commandReq, "lis.command:", lis.command)
		}

		//Test SubscribeCommand method: agent command path.
		lis = MockListener{}
		ZkURL.PutParam("nodeType", ZkNodeTypeAgent)
		z.createPersistent(AgentCommandPath, true)
		z.SubscribeCommand(ZkURL, &lis)
		ZkURL.PutParam("nodeType", "")
		z.zkConn.Set(AgentCommandPath, []byte(commandReq), -1)
		time.Sleep(10 * time.Millisecond)
		if !reflect.DeepEqual(commandReq, lis.command) {
			t.Error("Subscribe agent command fail. commandReq:", commandReq, "lis.command:", lis.command)
		}

		//Test UnSubscribeCommand method: service command path.
		z.UnSubscribeCommand(ZkURL, &lis)
		time.Sleep(10 * time.Millisecond)
		if _, ok := <-z.watchSwitcherMap[CommandPath]; ok {
			t.Error("UnSubscribe command fail.")
		}

		//Test UnSubscribeCommand method: agent command path.
		ZkURL.PutParam("nodeType", ZkNodeTypeAgent)
		z.UnSubscribeCommand(ZkURL, &lis)
		ZkURL.PutParam("nodeType", "")
		time.Sleep(10 * time.Millisecond)
		if _, ok := <-z.watchSwitcherMap[CommandPath]; ok {
			t.Error("UnSubscribe command fail.")
		}

		//Test UnRegister method.
		z.UnRegister(ZkURL)
		isExistUnReg, _, errUnReg := z.zkConn.Exists(UnavailableServerPath)
		isExistGeg, _, errReg := z.zkConn.Exists(ServerPath)
		if errUnReg == nil && errReg == nil {
			if isExistUnReg || isExistGeg {
				t.Error("UnRegister fail.")
			}
		} else {
			t.Error("UnRegister err:", errUnReg, errReg)
		}
	}
}

func hasZKServer(url motan.URL) (ZkRegistry, bool) {
	tcpAddr, _ := net.ResolveTCPAddr("tcp4", url.GetAddressStr())
	if _, err := net.DialTCP("tcp", nil, tcpAddr); err == nil {
		z := ZkRegistry{url: ZkURL}
		z.Initialize()
		return z, true
	}
	return ZkRegistry{}, false
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
