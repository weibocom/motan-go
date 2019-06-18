package registry

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
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
	Mesh   = "mesh"
)

type SnapshotNodeInfo struct {
	ExtInfo string `json:"extInfo"`
	Addr    string `json:"address"`
}

type ServiceNode struct {
	Group string             `json:"group"`
	Path  string             `json:"path"`
	Nodes []SnapshotNodeInfo `json:"nodes"`
}

var (
	once               sync.Once
	nodeRsSnapshotLock sync.RWMutex
	snapshot           map[string]map[string]ServiceNode
	snapshotConf       = &motan.SnapshotConf{SnapshotInterval: DefaultSnapshotInterval, SnapshotDir: DefaultSnapshotDir}
)

func CheckSnapshotDir() {
	dir := snapshotConf.SnapshotDir
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.Mkdir(dir, 0775); err != nil {
			vlog.Errorf("registry make directory error. dir:%s, err:%s", dir, err.Error())
			return
		}
	}
}

func flushSnapshot() {
	vlog.Infoln("registry start snapshot, dir:", snapshotConf.SnapshotDir)
	ticker := time.NewTicker(snapshotConf.SnapshotInterval)
	for range ticker.C {
		if snapshot != nil {
			nodeRsSnapshotLock.RLock()
			newNodes := map[string]ServiceNode{}
			for _, serviceNodes := range snapshot {
				for key, serviceNode := range serviceNodes {
					if n, ok := newNodes[key]; ok {
						n.Nodes = append(n.Nodes, serviceNode.Nodes...)
						newNodes[key] = n
					} else {
						newNodes[key] = serviceNode
					}
				}
			}
			for key, node := range newNodes {
				nodeRsSnapshot := JSONString(node)
				writeErr := ioutil.WriteFile(filepath.Join(snapshotConf.SnapshotDir, key), StringToSliceByte(nodeRsSnapshot), 0777)
				if writeErr != nil {
					vlog.Errorf("Write snapshot file err, Err: %+v\n", writeErr)
				}
			}
			nodeRsSnapshotLock.RUnlock()
		}
	}
}

func SetSnapshotConf(snapshotInterval time.Duration, snapshotDir string) {
	snapshotConf.SnapshotDir = snapshotDir
	snapshotConf.SnapshotInterval = snapshotInterval
}

func GetSnapshotConf() *motan.SnapshotConf {
	return snapshotConf
}

func RegistDefaultRegistry(extFactory motan.ExtensionFactory) {
	extFactory.RegistExtRegistry(Direct, func(url *motan.URL) motan.Registry {
		return &DirectRegistry{url: url}
	})

	extFactory.RegistExtRegistry(ZK, func(url *motan.URL) motan.Registry {
		return &ZkRegistry{url: url}
	})

	extFactory.RegistExtRegistry(Consul, func(url *motan.URL) motan.Registry {
		return &ConsulRegistry{url: url}
	})

	extFactory.RegistExtRegistry(Mesh, func(url *motan.URL) motan.Registry {
		return &MeshRegistry{url: url}
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

func SaveSnapshot(registry string, nodeKey string, nodes ServiceNode) {
	once.Do(func() {
		CheckSnapshotDir()
		go flushSnapshot()
	})
	nodeRsSnapshotLock.Lock()
	defer nodeRsSnapshotLock.Unlock()
	if snapshot == nil {
		snapshot = make(map[string]map[string]ServiceNode)
	}
	if snapshot[registry] == nil {
		snapshot[registry] = make(map[string]ServiceNode)
	}
	snapshot[registry][nodeKey] = nodes
}

func JSONString(v interface{}) string {
	bytes, _ := json.Marshal(v)
	return string(bytes)
}
