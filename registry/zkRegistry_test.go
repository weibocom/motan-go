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
		Host:       "127.0.0.1",
		Port:       2181,
		Path:       "zkTestPath",
		Group:      "zkTestGroup",
		Parameters: map[string]string{motan.ApplicationKey: "zkTestApp"},
	}
	ServerPath            = "/motan/zkTestGroup/zkTestPath/server/127.0.0.1:2181"
	UnavailableServerPath = "/motan/zkTestGroup/zkTestPath/unavailableServer/127.0.0.1:2181"
	AgentPath             = "/motan/agent/zkTestApp/node/127.0.0.1:2181"
	CommandPath           = "/motan/zkTestGroup/command"
	AgentCommandPath      = "/motan/agent/zkTestApp/command"
)

//Test path generation methods.
func TestZkRegistryToPath(t *testing.T) {
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
}

func TestZkRegistryCreateNode(t *testing.T) {
	if z, ok := hasZKServer(*ZkURL); ok {
		//Test CreateNode method, verify if the specified path exists.
		z.CreateNode(ZkURL, ZkNodeTypeServer)
		isExist, _, err := z.zkConn.Exists(ServerPath)
		if err != nil || !isExist {
			t.Error("Create server node fail. exist:", isExist, " err:", err)
		}
		z.CreateNode(ZkURL, ZkNodeTypeAgent)
		isExist, _, err = z.zkConn.Exists(AgentPath)
		if err != nil || !isExist {
			t.Error("Create agent node fail. exist:", isExist, " err:", err)
		}

		//Test Discover method, verify if the specified path exists.
		ZkURL.ClearCachedInfo()
		if !reflect.DeepEqual(z.Discover(ZkURL)[0], ZkURL) {
			t.Error("Discover fail:", z.Discover(ZkURL)[0], ZkURL)
		}

		//Test DiscoverCommand method, verify if the specified path exists.
		z.CreatePersistent(CommandPath, true)
		commandReq := "hello"
		z.zkConn.Set(CommandPath, []byte(commandReq), -1)
		commandRes := z.DiscoverCommand(ZkURL)
		if !reflect.DeepEqual(commandReq, commandRes) {
			t.Error("Discover command fail. commandReq:", commandReq, "commandRes:", commandRes)
		}

		//Test RemoveNode method, verify if the specified path exists.
		z.RemoveNode(ZkURL, ZkNodeTypeServer)
		if isExist, _, err := z.zkConn.Exists(ServerPath); err == nil {
			if isExist {
				t.Error("RemoveNode fail.")
			}
		} else {
			t.Error("RemoveNode err:", err)
		}
	}
}

func TestZkRegistryAvailable(t *testing.T) {
	if z, ok := hasZKServer(*ZkURL); ok {
		//Test Available method, verify if the specified path exists.
		z.Available(ZkURL)
		if isExist, _, err := z.zkConn.Exists(ServerPath); err == nil {
			if !isExist {
				t.Error("Register fail.")
			}
		} else {
			t.Error("Register err:", err)
		}

		//Test Unavailable method, verify if the specified path exists.
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
	}
}

func TestZkRegistryRegister(t *testing.T) {
	if z, ok := hasZKServer(*ZkURL); ok {
		//Test Register method, verify if the specified path exists.
		z.Register(ZkURL)
		if isExist, _, err := z.zkConn.Exists(UnavailableServerPath); err == nil {
			if !isExist {
				t.Error("Register fail.")
			}
		} else {
			t.Error("Register err:", err)
		}

		//Test Subscribe method, verify if the specified path exists.
		lis := MockListener{}
		z.Subscribe(ZkURL, &lis)
		z.CreateNode(ZkURL, ZkNodeTypeServer)
		time.Sleep(10 * time.Millisecond)
		urlRes := &motan.URL{
			Protocol:   "zookeeper",
			Host:       "127.0.0.1",
			Port:       2181,
			Path:       "zkTestPath",
			Group:      "zkTestGroup",
			Parameters: map[string]string{"application": "zkTestApp", "nodeType": "referer"},
		}
		lis.registryURL.ClearCachedInfo()
		time.Sleep(10 * time.Millisecond)
		if !reflect.DeepEqual(lis.registryURL, urlRes) {
			t.Error("Subscribe fail. registryURL:", lis.registryURL)
		}

		//Test UnSubscribe method, verify if the specified path exists.
		z.Unsubscribe(ZkURL, &lis)
		subKey := GetSubKey(ZkURL)
		idt := lis.GetIdentity()
		time.Sleep(10 * time.Millisecond)
		if listeners, ok := z.subscribeMap[subKey]; ok {
			if _, ok := listeners[idt]; ok {
				t.Error("UnSubscribe fail. registryURL:", lis.registryURL)
			}
		}

		//Test SubscribeCommand method, verify if the specified path exists.
		z.SubscribeCommand(ZkURL, &lis)
		commandReq := "hello"
		z.zkConn.Set(CommandPath, []byte(commandReq), -1)
		time.Sleep(10 * time.Millisecond)
		if !reflect.DeepEqual(commandReq, lis.command) {
			t.Error("Subscribe command fail. commandReq:", commandReq, "lis.command:", lis.command)
		}

		//Test UnSubscribeCommand method, verify if the specified path exists.
		z.UnSubscribeCommand(ZkURL, &lis)
		time.Sleep(10 * time.Millisecond)
		if _, ok := <-z.watchSwitcherMap[CommandPath]; ok {
			t.Error("UnSubscribe command fail.")
		}

		//Test UnRegister method, verify if the specified path exists.
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
