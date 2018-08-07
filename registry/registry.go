package registry

import (
	"os"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"github.com/weibocom/motan-go/log"
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
	once               sync.Once
	nodeRsSnapshotLock sync.RWMutex
	snapshot           map[string]map[string]ServiceNode
	snapshotConf       = &motan.SnapshotConf{SnapshotInterval: DefaultSnapshotInterval, SnapshotDir: DefaultSnapshotDir}
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

func SaveSnapshot(registry string, nodeKey string, nodes ServiceNode) {
	nodeRsSnapshotLock.Lock()
	defer nodeRsSnapshotLock.Unlock()
	if len(snapshot) == 0 {
		snapshot = make(map[string]map[string]ServiceNode)
	}
	if len(snapshot[registry]) == 0 {
		snapshot[registry] = make(map[string]ServiceNode)
	}
	snapshot[registry][nodeKey] = nodes
}

func InitSnapshot(conf *motan.SnapshotConf) {
	once.Do(func() {
		if _, err := os.Stat(conf.SnapshotDir); os.IsNotExist(err) {
			if err := os.Mkdir(conf.SnapshotDir, 0774); err != nil {
				vlog.Errorf("registry make directory error. dir:%s, err:%s\n", conf.SnapshotDir, err.Error())
				return
			}
		}
		vlog.Infoln("registry start snapshot, dir:", conf.SnapshotDir)
		flushSnapshot(conf)
	})
}
func flushSnapshot(conf *motan.SnapshotConf) {
	ticker := time.NewTicker(conf.SnapshotInterval)
	for range ticker.C {
		nodeRsSnapshotLock.RLock()
		newNodes := map[string]ServiceNode{}
		for _, serviceNodes := range snapshot {
			for key, serviceNode := range serviceNodes {
				if n, ok := newNodes[key]; ok {
					n.Nodes = deduplicateMerge(n.Nodes, serviceNode.Nodes)
					newNodes[key] = n
				} else {
					newNodes[key] = serviceNode
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

func deduplicateMerge(l1 []SnapShotNodeInfo, l2 []SnapShotNodeInfo) []SnapShotNodeInfo {
	var result []SnapShotNodeInfo
	temp := make(map[SnapShotNodeInfo]bool, len(l1)+len(l2))
	for _, e := range l1 {
		temp[e] = true
		result = append(result, e)
	}
	for _, e := range l2 {
		if _, ok := temp[e]; !ok {
			temp[e] = true
			result = append(result, e)
		}
	}
	return result
}

func JSONString(v interface{}) string {
	bytes, _ := json.Marshal(v)
	return string(bytes)
}
