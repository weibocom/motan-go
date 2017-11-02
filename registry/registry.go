package registry

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	motan "github.com/weibocom/motan-go/core"
)

const (
	DefaultSnapshotInterval  = 10 * time.Second
	DefaultHeartbeatInterval = 10 * 1000 //ms
	DefaultTimeout           = 3 * 1000  //ms
	DefaultSnapshotDir       = "./snapshot"
)

//ext name
const (
	Direct = "direct"
	Consul = "consul"
	ZK     = "zookeeper"
)

type SnapShotNodeInfo struct {
	ExtInfo string `json:"extInfo"`
	Addr    string `json:"address"`
}

type ServiceNode struct {
	Group string             `json:"group"`
	Path  string             `json:"path"`
	Nodes []SnapShotNodeInfo `json:"nodes"`
}

var (
	// TODO support mulit registry
	snapshotConf       = &motan.SnapshotConf{SnapshotInterval: DefaultSnapshotInterval, SnapshotDir: DefaultSnapshotDir}
	nodeRsSnapshotLock sync.RWMutex
)

func SetSanpshotConf(snapshotInterval time.Duration, snapshotDir string) {
	snapshotConf.SnapshotDir = snapshotDir
	snapshotConf.SnapshotInterval = snapshotInterval
}

func GetSanpshotConf() *motan.SnapshotConf {
	return snapshotConf
}

func RegistDefaultRegistry(extFactory motan.ExtentionFactory) {
	extFactory.RegistExtRegistry(Direct, func(url *motan.URL) motan.Registry {
		return &DirectRegistry{url: url}
	})

	extFactory.RegistExtRegistry(ZK, func(url *motan.URL) motan.Registry {
		return &ZkRegistry{url: url}
	})

	extFactory.RegistExtRegistry(Consul, func(url *motan.URL) motan.Registry {
		return &ConsulRegistry{url: url}
	})
}

func IsAgent(url *motan.URL) bool {
	isAgent := false
	if t, ok := url.Parameters["nodeType"]; ok {
		if t == "agent" {
			isAgent = true
		}
	}
	return isAgent
}

func GetSubKey(url *motan.URL) string {
	return url.Group + "/" + url.Path + "/service"
}

func getNodeKey(url *motan.URL) string {
	return url.Group + "_" + url.Path
}

func SliceByteToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func StringToSliceByte(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

func saveSnapshot(d string, nodes map[string]ServiceNode) {
	nodeRsSnapshotLock.RLock()
	for key, node := range nodes {
		nodeRsSnapshot := JSONString(node)
		// ioutil.ReadFile(filepath.Join(d, key))
		ioutil.WriteFile(filepath.Join(d, key), StringToSliceByte(nodeRsSnapshot), 0777)
	}
	nodeRsSnapshotLock.RUnlock()
}

func JSONString(v interface{}) string {
	bytes, _ := json.Marshal(v)
	return string(bytes)
}
