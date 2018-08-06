package registry

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"github.com/weibocom/motan-go/log"
	motan "github.com/weibocom/motan-go/core"
	"os"
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
	once                sync.Once
	nodeRsSnapshotLock  sync.RWMutex
	snapshot            map[string]map[string]ServiceNode
	isSnapshotAvailable bool
	snapshotConf        = &motan.SnapshotConf{SnapshotInterval: DefaultSnapshotInterval, SnapshotDir: DefaultSnapshotDir}
)

func SetSnapshotConf(snapshotInterval time.Duration, snapshotDir string) {
	snapshotConf.SnapshotDir = snapshotDir
	snapshotConf.SnapshotInterval = snapshotInterval
}

func GetSnapshotConf() *motan.SnapshotConf {
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

func GetNodeKey(url *motan.URL) string {
	return url.Group + "_" + url.Path
}

func StringToSliceByte(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

func SaveSnapshot(registry string, nodes map[string]ServiceNode) {
	if isSnapshotAvailable {
		nodeRsSnapshotLock.Lock()
		defer nodeRsSnapshotLock.Unlock()
		snapshot[registry] = nodes
	}
}

func InitSnapshot(conf *motan.SnapshotConf) {
	once.Do(func() {
		snapshot = make(map[string]map[string]ServiceNode)
		if _, err := os.Stat(conf.SnapshotDir); os.IsNotExist(err) {
			if err := os.Mkdir(conf.SnapshotDir, 0774); err != nil {
				vlog.Errorf("registry make directory error. dir:%s, err:%s\n", conf.SnapshotDir, err.Error())
				return
			}
		}
		isSnapshotAvailable = true
		vlog.Infoln("registry start snapshot, dir:", conf.SnapshotDir)
		flushSnapshot(conf)
	})
}
func flushSnapshot(conf *motan.SnapshotConf) {
	ticker := time.NewTicker(conf.SnapshotInterval)
	for range ticker.C {
		nodeRsSnapshotLock.RLock()
		newNodes := map[string]ServiceNode{}
		for _, nodes := range snapshot {
			for key, node := range nodes {
				if n, ok := newNodes[key]; ok {
					n.Nodes = append(n.Nodes, node.Nodes...)
				} else {
					newNodes[key] = node
				}
			}
		}
		for key, node := range newNodes {
			nodeRsSnapshot := JSONString(node)
			ioutil.WriteFile(filepath.Join(conf.SnapshotDir, key), StringToSliceByte(nodeRsSnapshot), 0777)
		}
		nodeRsSnapshotLock.RUnlock()
	}
}

func JSONString(v interface{}) string {
	bytes, _ := json.Marshal(v)
	return string(bytes)
}
