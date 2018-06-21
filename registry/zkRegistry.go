package registry

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/samuel/go-zookeeper/zk"
	"github.com/weibocom/motan-go/cluster"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	ZkRegistryNamespace = "/motan"
	ZkRegistryCommand   = "/command"
	ZkRegistryNode      = "/node"
	ZkPathSeparator     = "/"
)

const (
	ZkNodeTypeServer           = "server"
	ZkNodeTypeUnavailbleServer = "unavailableServer"
	ZkNodeTypeClient           = "client"
	ZkNodeTypeAgent            = "agent"
)

type ZkRegistry struct {
	zkConn             *zk.Conn
	url                *motan.URL
	timeout            time.Duration
	sessionTimeout     time.Duration
	subscribeCmdLock   sync.Mutex
	registerLock       sync.Mutex
	subscribeLock      sync.Mutex
	watchSwitcherMap   map[string]chan bool
	registeredServices map[string]*motan.URL
	nodeRs             map[string]ServiceNode
	subscribeMap       map[string]map[string]motan.NotifyListener
}

func (z *ZkRegistry) Initialize() {
	z.sessionTimeout = time.Duration(
		z.url.GetPositiveIntValue(motan.SessionTimeOutKey, DefaultHeartbeatInterval)) * time.Millisecond
	z.timeout = time.Duration(z.url.GetPositiveIntValue(motan.TimeOutKey, DefaultTimeout)) * time.Millisecond
	var addrs []string
	if z.url.Host != "" {
		addrs = []string{z.url.GetAddressStr()}
	} else {
		addrs = strings.Split(z.url.GetParam(motan.AddressKey, ""), ",")
	}
	if c, _, err := zk.Connect(addrs, z.sessionTimeout); err == nil {
		z.zkConn = c
	} else {
		vlog.Errorf("zk connect error:%+v\n", err)
	}
	z.subscribeMap = make(map[string]map[string]motan.NotifyListener)
	z.nodeRs = make(map[string]ServiceNode)
	z.StartSnapshot(GetSanpshotConf())
	z.watchSwitcherMap = make(map[string]chan bool)
	z.registeredServices = make(map[string]*motan.URL)
}

func toGroupPath(url *motan.URL) string {
	return ZkRegistryNamespace + ZkPathSeparator + url.Group
}

func toServicePath(url *motan.URL) string {
	return toGroupPath(url) + ZkPathSeparator + url.Path
}

func toCommandPath(url *motan.URL) string {
	return toGroupPath(url) + ZkRegistryCommand
}

func toNodeTypePath(url *motan.URL, nodeType string) string {
	return toServicePath(url) + ZkPathSeparator + nodeType
}

func toNodePath(url *motan.URL, nodeType string) string {
	return toNodeTypePath(url, nodeType) + ZkPathSeparator + url.GetAddressStr()
}

func toAgentPath(url *motan.URL) string {
	return ZkRegistryNamespace + ZkPathSeparator + ZkNodeTypeAgent + ZkPathSeparator + url.Parameters[motan.ApplicationKey]
}

func toAgentNodeTypePath(url *motan.URL) string {
	return toAgentPath(url) + ZkRegistryNode
}

func toAgentNodePath(url *motan.URL) string {
	return toAgentNodeTypePath(url) + ZkPathSeparator + url.GetAddressStr()
}

func toAgentCommandPath(url *motan.URL) string {
	return toAgentPath(url) + ZkRegistryCommand
}

func (z *ZkRegistry) RemoveNode(url *motan.URL, nodeType string) error {
	nodePath := toNodePath(url, nodeType)
	if IsAgent(url) {
		nodePath = toAgentNodePath(url)
	}
	isExist, stats, err := z.zkConn.Exists(nodePath)
	if err == nil && isExist {
		if err = z.zkConn.Delete(nodePath, stats.Version); err == nil {
			vlog.Infof("remove node success. node: %s\n", nodePath)
			return nil
		}
	}
	vlog.Infof("remove node error. err: %v, nodeIsExist: %v\n", err, isExist)
	return err
}

func (z *ZkRegistry) CreateNode(url *motan.URL, nodeType string) error {
	typePath := toNodeTypePath(url, nodeType)
	nodePath := toNodePath(url, nodeType)
	if IsAgent(url) {
		typePath = toAgentNodeTypePath(url)
		nodePath = toAgentNodePath(url)
	}
	z.RemoveNode(url, nodeType)
	isExist, _, err := z.zkConn.Exists(typePath)
	if err == nil {
		if !isExist {
			z.CreatePersistent(typePath, true)
		}
		if _, err = z.zkConn.Create(nodePath, []byte{}, zk.FlagEphemeral, zk.WorldACL(zk.PermAll)); err == nil {
			vlog.Infof("create node success. node: %s\n", nodePath)
			return nil
		}
	}
	vlog.Errorf("create node error. node: %s, err: %v\n", nodePath, err)
	return err
}

func (z *ZkRegistry) CreatePersistent(path string, createParents bool) {
	if _, err := z.zkConn.Create(path, nil, 0, zk.WorldACL(zk.PermAll)); err == nil {
		vlog.Infof("create Persistent node: %s\n", path)
	} else if err == zk.ErrNoNode {
		if createParents {
			parts := strings.Split(path, "/")
			parentPath := strings.Join(parts[:len(parts)-1], "/")
			z.CreatePersistent(parentPath, createParents)
			z.CreatePersistent(path, createParents)
		}
	} else {
		vlog.Errorf("err create Persistent. path: %s, err: %v\n", path, err)
	}
}

func (z *ZkRegistry) Register(url *motan.URL) {
	z.registerLock.Lock()
	defer z.registerLock.Unlock()
	vlog.Infof("start zk register %s\n", url.GetIdentity())
	if url.Group == "" || url.Path == "" || url.Host == "" {
		vlog.Errorf("register fail.invalid url : %s\n", url.GetIdentity())
	}
	if IsAgent(url) {
		z.CreateNode(url, ZkNodeTypeAgent)
	} else {
		z.registeredServices[url.GetIdentity()] = url
		z.CreateNode(url, ZkNodeTypeUnavailbleServer)
	}
}

func (z *ZkRegistry) UnRegister(url *motan.URL) {
	z.registerLock.Lock()
	defer z.registerLock.Unlock()
	z.RemoveNode(url, ZkNodeTypeServer)
	z.RemoveNode(url, ZkNodeTypeUnavailbleServer)
	delete(z.registeredServices, url.GetIdentity())
}

// @TODO extInfo from java Obj Pase
func buildURL4Nodes(nodes []string, url *motan.URL) []*motan.URL {
	result := make([]*motan.URL, 0, len(nodes))
	for _, node := range nodes {
		nodeinfo := strings.Split(node, ":")
		port, _ := strconv.Atoi(nodeinfo[1])
		refURL := url.Copy()
		refURL.Host = nodeinfo[0]
		refURL.Port = port
		result = append(result, refURL)
	}
	return result
}

func (z *ZkRegistry) Subscribe(url *motan.URL, listener motan.NotifyListener) {
	z.subscribeLock.Lock()
	defer z.subscribeLock.Unlock()
	vlog.Infof("start subscribe service. url:%s\n", url.GetIdentity())
	subKey := GetSubKey(url)
	idt := listener.GetIdentity()
	if listeners, ok := z.subscribeMap[subKey]; ok {
		if _, exist := listeners[idt]; !exist {
			listeners[idt] = listener
		}
	} else {
		lMap := make(map[string]motan.NotifyListener)
		lMap[idt] = listener
		z.subscribeMap[subKey] = lMap
		serverPath := toNodeTypePath(url, ZkNodeTypeServer)
		if _, _, ch, err := z.zkConn.ChildrenW(serverPath); err == nil {
			vlog.Infof("start watch %s\n", subKey)
			url.PutParam(motan.NodeTypeKey, motan.NodeTypeReferer) // all subscribe url must as referer
			if url.Host == "" {
				url.Host = motan.GetLocalIP()
			}
			z.CreateNode(url, ZkNodeTypeClient) // register as rpc client
			go func() {
				for {
					select {
					case evt := <-ch:
						if evt.Type == zk.EventNodeChildrenChanged {
							if nodes, _, chx, err := z.zkConn.ChildrenW(serverPath); err == nil {
								z.buildNodes(nodes, url)
								ch = chx
								if listeners, ok := z.subscribeMap[subKey]; ok {
									for _, l := range listeners {
										l.Notify(z.url, buildURL4Nodes(nodes, url))
										vlog.Infof("EventNodeChildrenChanged %+v\n", nodes)
									}
								}
							}
						}
					}
				}
			}()
		} else {
			vlog.Infof("zk Subscribe err %+v\n", err)
		}
	}
}

func (z *ZkRegistry) buildNodes(nodes []string, url *motan.URL) {
	serviceNode := &ServiceNode{
		Group: url.Group,
		Path:  url.Path,
	}
	nodeInfos := []SnapShotNodeInfo{}
	for _, addr := range nodes {
		info := &SnapShotNodeInfo{Addr: addr}
		nodeInfos = append(nodeInfos, *info)
	}
	serviceNode.Nodes = nodeInfos
	z.nodeRs[getNodeKey(url)] = *serviceNode
}

func (z *ZkRegistry) Unsubscribe(url *motan.URL, listener motan.NotifyListener) {
	z.subscribeLock.Lock()
	defer z.subscribeLock.Unlock()
	subKey := GetSubKey(url)
	idt := listener.GetIdentity()
	if listeners, ok := z.subscribeMap[subKey]; ok {
		delete(listeners, idt)
	}
}

func (z *ZkRegistry) Discover(url *motan.URL) []*motan.URL {
	nodePath := toNodeTypePath(url, ZkNodeTypeServer) // discover server nodes
	nodes, _, err := z.zkConn.Children(nodePath)
	if err == nil {
		z.buildNodes(nodes, url)
		return buildURL4Nodes(nodes, url)
	}
	vlog.Errorf("zookeeper registry discover fail! discover url:%s, err:%s\n", url.GetIdentity(), err.Error())
	return nil
}

func (z *ZkRegistry) SubscribeCommand(url *motan.URL, listener motan.CommandNotifyListener) {
	z.subscribeCmdLock.Lock()
	defer z.subscribeCmdLock.Unlock()
	vlog.Infof("zookeeper subscribe command of %s\n", url.GetIdentity())
	commandPath := toCommandPath(url)
	if IsAgent(url) {
		commandPath = toAgentCommandPath(url)
	}
	if isExist, _, err := z.zkConn.Exists(commandPath); err == nil {
		if !isExist {
			vlog.Infof("command didn't exists, path:%s\n", commandPath)
			return
		}
	} else {
		vlog.Errorf("check command exists error: %+v\n", err)
	}
	if _, _, ch, err := z.zkConn.GetW(commandPath); err == nil {
		tempChan := z.watchSwitcherMap[commandPath]
		if tempChan == nil {
			z.watchSwitcherMap[commandPath] = make(chan bool)
			tempChan = z.watchSwitcherMap[commandPath]
		}
		vlog.Infof("start watch command %s\n", commandPath)
		go func() {
			for {
				select {
				case evt := <-ch:
					if evt.Type == zk.EventNodeDataChanged {
						if data, _, chx, err := z.zkConn.GetW(commandPath); err == nil {
							ch = chx
							cmdInfo := tempFixZK(data)
							listener.NotifyCommand(z.url, cluster.ServiceCmd, cmdInfo)
							vlog.Infof("command changed, path:%s, data:%s\n", commandPath, cmdInfo)
						} else {
							vlog.Infof("command changed, get cmdInfo error, err: %+v\n", err)
						}
					}
				case checkWatch := <-tempChan:
					if !checkWatch {
						close(tempChan)
						return
					}
				}
			}
		}()
	} else {
		vlog.Warningf("zookeeper subscribe command fail. url:%s, err:%s, zk_path:%s, urlx:%+v\n", url.GetIdentity(), err.Error(), commandPath, url)
	}
}

func (z *ZkRegistry) UnSubscribeCommand(url *motan.URL, listener motan.CommandNotifyListener) {
	z.subscribeCmdLock.Lock()
	defer z.subscribeCmdLock.Unlock()
	commandPath := toCommandPath(url)
	if IsAgent(url) {
		commandPath = toAgentCommandPath(url)
	}
	z.watchSwitcherMap[commandPath] <- false
}

func (z *ZkRegistry) DiscoverCommand(url *motan.URL) string {
	vlog.Infof("zookeeper Discover command of %s\n", url.GetIdentity())
	var res string
	commandPath := toCommandPath(url)
	if IsAgent(url) {
		commandPath = toAgentCommandPath(url)
	}
	if isExist, _, err := z.zkConn.Exists(commandPath); err == nil {
		if !isExist {
			vlog.Infof("zookeeper command didn't exist, path:%s\n", commandPath)
			return res
		}
	} else {
		vlog.Infof("zookeeper command check err: %+v\n", err)
		return res
	}
	if data, _, err := z.zkConn.Get(commandPath); err == nil {
		vlog.Infof("zookeeper Discover command %s\n", commandPath)
		res = tempFixZK(data)
	} else {
		vlog.Warningf("zookeeper DiscoverCommand error. url:%s, err:%s\n", url.GetIdentity(), err.Error())
	}
	return res
}

func tempFixZK(data []byte) string {
	if len(data) > 7 && data[0] != '{' && data[7] == '{' {
		return string(data[7:])
	}
	return string(data)
}

func (z *ZkRegistry) Available(url *motan.URL) {
	if url == nil {
		for _, u := range z.registeredServices {
			z.RemoveNode(u, ZkNodeTypeUnavailbleServer)
			z.CreateNode(u, ZkNodeTypeServer)
		}
	} else {
		z.RemoveNode(url, ZkNodeTypeUnavailbleServer)
		z.CreateNode(url, ZkNodeTypeServer)
	}

}

func (z *ZkRegistry) Unavailable(url *motan.URL) {
	if url == nil {
		for _, u := range z.registeredServices {
			z.RemoveNode(u, ZkNodeTypeServer)
			z.CreateNode(u, ZkNodeTypeUnavailbleServer)
		}
	} else {
		z.RemoveNode(url, ZkNodeTypeServer)
		z.CreateNode(url, ZkNodeTypeUnavailbleServer)
	}
}

func (z *ZkRegistry) GetRegisteredServices() []*motan.URL {
	urls := make([]*motan.URL, 0, len(z.registeredServices))
	for _, u := range z.registeredServices {
		urls = append(urls, u)
	}
	return urls
}

func (z *ZkRegistry) GetURL() *motan.URL {
	return z.url
}

func (z *ZkRegistry) SetURL(url *motan.URL) {
	z.url = url
}

func (z *ZkRegistry) GetName() string {
	return "zookeeper"
}

func (z *ZkRegistry) StartSnapshot(conf *motan.SnapshotConf) {
	if _, err := os.Stat(conf.SnapshotDir); os.IsNotExist(err) {
		if err := os.Mkdir(conf.SnapshotDir, 0774); err != nil {
			vlog.Infoln(err)
		}
	}
	go func(z *ZkRegistry) {
		ticker := time.NewTicker(conf.SnapshotInterval)
		for range ticker.C {
			saveSnapshot(conf.SnapshotDir, z.nodeRs)
		}
	}(z)
}
