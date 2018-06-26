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
)

func TestZkRegistryToPath(t *testing.T) {
	if p := toNodePath(ZkURL, ZkNodeTypeServer); p != "/motan/zkTestGroup/zkTestPath/server/127.0.0.1:2181" {
		t.Error("toNodePath err. result:", p)
	}
	if p := toCommandPath(ZkURL); p != "/motan/zkTestGroup/command" {
		t.Error("toCommandPath err. result:", p)
	}
	if p := toAgentNodePath(ZkURL); p != "/motan/agent/zkTestApp/node/127.0.0.1:2181" {
		t.Error("toAgentNodePath err. result:", p)
	}
	if p := toAgentCommandPath(ZkURL); p != "/motan/agent/zkTestApp/command" {
		t.Error("toAgentCommandPath err. result:", p)
	}
}

func TestZkRegistryCreateNode(t *testing.T) {
	if z, ok := hasZKServer(*ZkURL); ok {
		z.CreateNode(ZkURL, ZkNodeTypeServer)
		if isExist, _, err := z.zkConn.Exists(toNodePath(ZkURL, ZkNodeTypeServer)); err == nil {
			if !isExist {
				t.Error("CreateNode fail.")
			}
		} else {
			t.Error("CreateNode err:", err)
		}
		z.RemoveNode(ZkURL, ZkNodeTypeServer)
		if isExist, _, err := z.zkConn.Exists(toNodePath(ZkURL, ZkNodeTypeServer)); err == nil {
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
		z.Available(ZkURL)
		if isExist, _, err := z.zkConn.Exists(toNodePath(ZkURL, ZkNodeTypeServer)); err == nil {
			if !isExist {
				t.Error("Register fail.")
			}
		} else {
			t.Error("Register err:", err)
		}
		z.Unavailable(ZkURL)
		isExistUnAvail, _, errUnAvail := z.zkConn.Exists(toNodePath(ZkURL, ZkNodeTypeUnavailbleServer))
		isExistAvail, _, errAvail := z.zkConn.Exists(toNodePath(ZkURL, ZkNodeTypeServer))
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
		z.Register(ZkURL)
		if isExist, _, err := z.zkConn.Exists(toNodePath(ZkURL, ZkNodeTypeUnavailbleServer)); err == nil {
			if !isExist {
				t.Error("Register fail.")
			}
		} else {
			t.Error("Register err:", err)
		}
		z.UnRegister(ZkURL)
		isExistUnReg, _, errUnReg := z.zkConn.Exists(toNodePath(ZkURL, ZkNodeTypeUnavailbleServer))
		isExistGeg, _, errReg := z.zkConn.Exists(toNodePath(ZkURL, ZkNodeTypeServer))
		if errUnReg == nil && errReg == nil {
			if isExistUnReg || isExistGeg {
				t.Error("UnRegister fail.")
			}
		} else {
			t.Error("UnRegister err:", errUnReg, errReg)
		}
	}
}

func TestZkRegistryDiscover(t *testing.T) {
	if z, ok := hasZKServer(*ZkURL); ok {
		z.CreateNode(ZkURL, ZkNodeTypeServer)
		ZkURL.ClearCachedInfo()
		if !reflect.DeepEqual(z.Discover(ZkURL)[0], ZkURL) {
			t.Error("Discover fail:", z.Discover(ZkURL)[0], ZkURL)
		}
		z.CreatePersistent(toCommandPath(ZkURL), true)
		commandReq := "hello"
		z.zkConn.Set(toCommandPath(ZkURL), []byte(commandReq), -1)
		commandRes := z.DiscoverCommand(ZkURL)
		if !reflect.DeepEqual(commandReq, commandRes) {
			t.Error("Discover command fail. commandReq:", commandReq, "commandRes:", commandRes)
		}
	}
}

func TestZkRegistrySubscribe(t *testing.T) {
	if z, ok := hasZKServer(*ZkURL); ok {
		lis := MockListener{}
		z.Register(ZkURL)
		z.Subscribe(ZkURL, &lis)
		z.Available(ZkURL)
		urlRes := &motan.URL{
			Protocol:   "zookeeper",
			Host:       "127.0.0.1",
			Port:       2181,
			Path:       "zkTestPath",
			Group:      "zkTestGroup",
			Parameters: map[string]string{"application": "zkTestApp", "nodeType": "referer"},
		}
		lis.registryURL.ClearCachedInfo()
		time.Sleep(500 * time.Millisecond)
		if !reflect.DeepEqual(lis.registryURL, urlRes) {
			t.Error("Subscribe fail. registryURL:", lis.registryURL)
		}
		z.Unsubscribe(ZkURL, &lis)
		subKey := GetSubKey(ZkURL)
		idt := lis.GetIdentity()
		time.Sleep(500 * time.Millisecond)
		if listeners, ok := z.subscribeMap[subKey]; ok {
			if _, ok := listeners[idt]; ok {
				t.Error("UnSubscribe fail. registryURL:", lis.registryURL)
			}
		}
		z.SubscribeCommand(ZkURL, &lis)
		commandReq := "hello"
		z.zkConn.Set(toCommandPath(ZkURL), []byte(commandReq), -1)
		time.Sleep(500 * time.Millisecond)
		if !reflect.DeepEqual(commandReq, lis.command) {
			t.Error("Subscribe command fail. commandReq:", commandReq, "lis.command:", lis.command)
		}
		z.UnSubscribeCommand(ZkURL, &lis)
		time.Sleep(500 * time.Millisecond)
		if _, ok := <-z.watchSwitcherMap[toCommandPath(ZkURL)]; ok {
			t.Error("Subscribe command fail.")
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
