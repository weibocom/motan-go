package registry

import (
	motan "github.com/weibocom/motan-go/core"
	"testing"
	"reflect"
	"time"
)

var (
	ZkUrl = &motan.URL{
		Protocol:   "zookeeper",
		Host:       "127.0.0.1",
		Port:       2181,
		Path:       "zkTestPath",
		Group:      "zkTestGroup",
		Parameters: map[string]string{motan.ApplicationKey: "zkTestApp"},
	}
)

func TestZkRegistryToPath(t *testing.T) {
	if p := toNodePath(ZkUrl, ZkNodeTypeServer); p != "/motan/zkTestGroup/zkTestPath/server/127.0.0.1:2181" {
		t.Error("toNodePath err. result:", p)
	}
	if p := toCommandPath(ZkUrl); p != "/motan/zkTestGroup/command" {
		t.Error("toCommandPath err. result:", p)
	}
	if p := toAgentNodePath(ZkUrl); p != "/motan/agent/zkTestApp/node/127.0.0.1:2181" {
		t.Error("toAgentNodePath err. result:", p)
	}
	if p := toAgentCommandPath(ZkUrl); p != "/motan/agent/zkTestApp/command" {
		t.Error("toAgentCommandPath err. result:", p)
	}
}

func TestZkRegistryCreateNode(t *testing.T) {
	zk := ZkRegistry{url: ZkUrl}
	zk.Initialize()
	zk.CreateNode(ZkUrl, ZkNodeTypeServer)
	if isExist, _, err := zk.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeServer)); err == nil {
		if !isExist {
			t.Error("CreateNode fail.")
		}
	} else {
		t.Error("CreateNode err:", err)
	}
	zk.RemoveNode(ZkUrl, ZkNodeTypeServer)
	if isExist, _, err := zk.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeServer)); err == nil {
		if isExist {
			t.Error("RemoveNode fail.")
		}
	} else {
		t.Error("RemoveNode err:", err)
	}
}

func TestZkRegistryAvailable(t *testing.T) {
	zk := ZkRegistry{url: ZkUrl}
	zk.Initialize()
	zk.Available(ZkUrl)
	if isExist, _, err := zk.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeServer)); err == nil {
		if !isExist {
			t.Error("Register fail.")
		}
	} else {
		t.Error("Register err:", err)
	}
	zk.Unavailable(ZkUrl)
	isExistUnAvail, _, errUnAvail := zk.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeUnavailbleServer))
	isExistAvail, _, errAvail := zk.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeServer))
	if errUnAvail == nil && errAvail == nil {
		if !isExistUnAvail || isExistAvail {
			t.Error("Unavailable fail.")
		}
	} else {
		t.Error("Unavailable err:", errUnAvail, errAvail)
	}
}

func TestZkRegistryRegister(t *testing.T) {
	zk := ZkRegistry{url: ZkUrl}
	zk.Initialize()
	zk.Register(ZkUrl)
	if isExist, _, err := zk.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeUnavailbleServer)); err == nil {
		if !isExist {
			t.Error("Register fail.")
		}
	} else {
		t.Error("Register err:", err)
	}
	zk.UnRegister(ZkUrl)
	isExistUnReg, _, errUnReg := zk.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeUnavailbleServer))
	isExistGeg, _, errReg := zk.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeServer))
	if errUnReg == nil && errReg == nil {
		if isExistUnReg || isExistGeg {
			t.Error("UnRegister fail.")
		}
	} else {
		t.Error("UnRegister err:", errUnReg, errReg)
	}
}

func TestZkRegistryDiscover(t *testing.T) {
	zk := ZkRegistry{url: ZkUrl}
	zk.Initialize()
	zk.CreateNode(ZkUrl, ZkNodeTypeServer)
	ZkUrl.ClearCachedInfo()
	if !reflect.DeepEqual(zk.Discover(ZkUrl)[0], ZkUrl) {
		t.Error("Discover fail:", zk.Discover(ZkUrl)[0], ZkUrl)
	}
	zk.CreatePersistent(toCommandPath(ZkUrl), true)
	commandReq := "hello"
	zk.zkConn.Set(toCommandPath(ZkUrl), []byte(commandReq), -1)
	commandRes := zk.DiscoverCommand(ZkUrl)
	if !reflect.DeepEqual(commandReq, commandRes) {
		t.Error("Discover command fail. commandReq:", commandReq, "commandRes:", commandRes)
	}
}

func TestZkRegistrySubscribe(t *testing.T) {
	zk := ZkRegistry{url: ZkUrl}
	zk.Initialize()
	lis := MockListener{}
	zk.Register(ZkUrl)
	zk.Subscribe(ZkUrl, &lis)
	zk.Available(ZkUrl)
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
	zk.Unsubscribe(ZkUrl, &lis)
	subKey := GetSubKey(ZkUrl)
	idt := lis.GetIdentity()
	time.Sleep(500 * time.Millisecond)
	if listeners, ok := zk.subscribeMap[subKey]; ok {
		if _, ok := listeners[idt]; ok {
			t.Error("UnSubscribe fail. registryURL:", lis.registryURL)
		}
	}
	zk.SubscribeCommand(ZkUrl, &lis)
	commandReq := "hello"
	zk.zkConn.Set(toCommandPath(ZkUrl), []byte(commandReq), -1)
	time.Sleep(500 * time.Millisecond)
	if !reflect.DeepEqual(commandReq, lis.command) {
		t.Error("Subscribe command fail. commandReq:", commandReq, "lis.command:", lis.command)
	}
	zk.UnSubscribeCommand(ZkUrl, &lis)
	time.Sleep(500 * time.Millisecond)
	if _, ok := <-zk.watchSwitcherMap[toCommandPath(ZkUrl)]; ok {
		t.Error("Subscribe command fail.")
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
