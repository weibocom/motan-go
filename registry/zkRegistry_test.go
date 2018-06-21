package registry

import (
	motan "github.com/weibocom/motan-go/core"
	"testing"
	"reflect"
	"time"
	"github.com/samuel/go-zookeeper/zk"
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
	z := ZkRegistry{url: ZkUrl}
	z.Initialize()
	z.CreateNode(ZkUrl, ZkNodeTypeServer)
	if isExist, _, err := z.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeServer)); err == nil {
		if !isExist {
			t.Error("CreateNode fail.")
		}
	} else {
		t.Error("CreateNode err:", err)
	}
	z.RemoveNode(ZkUrl, ZkNodeTypeServer)
	if isExist, _, err := z.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeServer)); err == nil {
		if isExist {
			t.Error("RemoveNode fail.")
		}
	} else {
		t.Error("RemoveNode err:", err)
	}
}

func TestZkRegistryAvailable(t *testing.T) {
	z := ZkRegistry{url: ZkUrl}
	z.Initialize()
	z.Available(ZkUrl)
	if isExist, _, err := z.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeServer)); err == nil {
		if !isExist {
			t.Error("Register fail.")
		}
	} else {
		t.Error("Register err:", err)
	}
	z.Unavailable(ZkUrl)
	isExistUnAvail, _, errUnAvail := z.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeUnavailbleServer))
	isExistAvail, _, errAvail := z.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeServer))
	if errUnAvail == nil && errAvail == nil {
		if !isExistUnAvail || isExistAvail {
			t.Error("Unavailable fail.")
		}
	} else {
		t.Error("Unavailable err:", errUnAvail, errAvail)
	}
}

func TestZkRegistryRegister(t *testing.T) {
	z := ZkRegistry{url: ZkUrl}
	z.Initialize()
	z.Register(ZkUrl)
	if isExist, _, err := z.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeUnavailbleServer)); err == nil {
		if !isExist {
			t.Error("Register fail.")
		}
	} else {
		t.Error("Register err:", err)
	}
	z.UnRegister(ZkUrl)
	isExistUnReg, _, errUnReg := z.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeUnavailbleServer))
	isExistGeg, _, errReg := z.zkConn.Exists(toNodePath(ZkUrl, ZkNodeTypeServer))
	if errUnReg == nil && errReg == nil {
		if isExistUnReg || isExistGeg {
			t.Error("UnRegister fail.")
		}
	} else {
		t.Error("UnRegister err:", errUnReg, errReg)
	}
}

func TestZkRegistryDiscover(t *testing.T) {
	z := ZkRegistry{url: ZkUrl}
	z.Initialize()
	z.CreateNode(ZkUrl, ZkNodeTypeServer)
	ZkUrl.ClearCachedInfo()
	if !reflect.DeepEqual(z.Discover(ZkUrl)[0], ZkUrl) {
		t.Error("Discover fail:", z.Discover(ZkUrl)[0], ZkUrl)
	}
	z.CreatePersistent(toCommandPath(ZkUrl), true)
	commandReq := "hello"
	z.zkConn.Set(toCommandPath(ZkUrl), []byte(commandReq), -1)
	commandRes := z.DiscoverCommand(ZkUrl)
	if !reflect.DeepEqual(commandReq, commandRes) {
		t.Error("Discover command fail. commandReq:", commandReq, "commandRes:", commandRes)
	}
}

func TestZkRegistrySubscribe(t *testing.T) {
	z := ZkRegistry{url: ZkUrl}
	z.Initialize()
	lis := MockListener{}
	z.Register(ZkUrl)
	z.Subscribe(ZkUrl, &lis)
	z.Available(ZkUrl)
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
	z.Unsubscribe(ZkUrl, &lis)
	subKey := GetSubKey(ZkUrl)
	idt := lis.GetIdentity()
	time.Sleep(500 * time.Millisecond)
	if listeners, ok := z.subscribeMap[subKey]; ok {
		if _, ok := listeners[idt]; ok {
			t.Error("UnSubscribe fail. registryURL:", lis.registryURL)
		}
	}
	z.SubscribeCommand(ZkUrl, &lis)
	commandReq := "hello"
	z.zkConn.Set(toCommandPath(ZkUrl), []byte(commandReq), -1)
	time.Sleep(500 * time.Millisecond)
	if !reflect.DeepEqual(commandReq, lis.command) {
		t.Error("Subscribe command fail. commandReq:", commandReq, "lis.command:", lis.command)
	}
	z.UnSubscribeCommand(ZkUrl, &lis)
	time.Sleep(500 * time.Millisecond)
	if _, ok := <-z.watchSwitcherMap[toCommandPath(ZkUrl)]; ok {
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

type ZkConn interface {
	Exists(path string) (bool, *zk.Stat, error)
	Create(path string, data []byte, flags int32, acl []zk.ACL) (string, error)
	Delete(path string, version int32) error
	Children(path string) ([]string, *zk.Stat, error)
	ChildrenW(path string) ([]string, *zk.Stat, <-chan zk.Event, error)
	Get(path string) ([]byte, *zk.Stat, error)
	Set(path string, data []byte, version int32) (*zk.Stat, error)
	GetW(path string) ([]byte, *zk.Stat, <-chan zk.Event, error)
}

type MockConn struct {
}

func (c *MockConn) Exists(path string) (bool, *zk.Stat, error) {
	return false, nil, nil
}

func (c *MockConn) Create(path string, data []byte, flags int32, acl []zk.ACL) (string, error) {
	return "", nil
}

func (c *MockConn) Delete(path string, version int32) error {
	return nil
}

func (c *MockConn) Children(path string) ([]string, *zk.Stat, error) {
	return nil, nil, nil
}

func (c *MockConn) ChildrenW(path string) ([]string, *zk.Stat, <-chan zk.Event, error) {
	return nil, nil, nil, nil
}

func (c *MockConn) Get(path string) ([]byte, *zk.Stat, error) {
	return nil, nil, nil
}

func (c *MockConn) Set(path string, data []byte, version int32) (*zk.Stat, error) {
	return nil, nil
}

func (c *MockConn) GetW(path string) ([]byte, *zk.Stat, <-chan zk.Event, error) {
	return nil, nil, nil, nil
}
