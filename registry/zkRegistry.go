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
	"encoding/binary"
)

const (
	StreamMagicTag              = 0xaced
	ShortStringTag              = 0x74
	ZkRegistryNamespace         = "/motan"
	ZkRegistryCommand           = "/command"
	ZkRegistryNode              = "/node"
	ZkPathSeparator             = "/"
	ZkNodeTypeServer            = "server"
	ZkNodeTypeUnavailableServer = "unavailableServer"
	ZkNodeTypeClient            = "client"
	ZkNodeTypeAgent             = "agent"
)

type ZkRegistry struct {
	available            bool
	zkConn               *zk.Conn
	url                  *motan.URL
	sessionTimeout       time.Duration
	serverLock           sync.Mutex
	clientLock           sync.Mutex
	snapshotNodeMap      map[string]ServiceNode
	watchSwitcherMap     map[string]chan bool
	registeredServiceMap map[string]*motan.URL
	availableServiceMap  map[string]*motan.URL
	subscribedServiceMap map[string]map[*motan.URL]motan.NotifyListener
	subscribedCommandMap map[string]map[*motan.URL]motan.CommandNotifyListener
}

func (z *ZkRegistry) Initialize() {
	z.sessionTimeout = time.Duration(
		z.url.GetPositiveIntValue(motan.SessionTimeOutKey, DefaultHeartbeatInterval)) * time.Millisecond
	z.subscribedServiceMap = make(map[string]map[*motan.URL]motan.NotifyListener)
	z.subscribedCommandMap = make(map[string]map[*motan.URL]motan.CommandNotifyListener)
	z.snapshotNodeMap = make(map[string]ServiceNode)
	z.StartSnapshot(GetSanpshotConf())
	z.watchSwitcherMap = make(map[string]chan bool)
	z.registeredServiceMap = make(map[string]*motan.URL)
	z.availableServiceMap = make(map[string]*motan.URL)
	//todo：需不需要GetAddressList方法，还可以tirm空格，逗号间可以加空格，与filter一致
	addrs := strings.Split(z.url.GetAddressStr(), ",")
	c, ch, err := zk.Connect(addrs, z.sessionTimeout)
	if err != nil {
		vlog.Errorf("[ZkRegistry] connect server error. err:%v\n", err)
		return
	}
	z.zkConn = c
	go z.reconnect(addrs, ch)
	z.setAvailable(true)
}

func (z *ZkRegistry) reconnect(addrs []string, ch <-chan zk.Event) {
	defer motan.HandlePanic(nil)
	for {
		ev := <-ch
		if ev.State == zk.StateDisconnected {
			z.setAvailable(false)
		} else if ev.State == zk.StateHasSession && !z.IsAvailable() {
			z.setAvailable(true)
			vlog.Infoln("[ZkRegistry] get new session notify")
			z.reconnectServer()
			z.reconnectClient()
		}
	}
}

func (z *ZkRegistry) reconnectServer() {
	z.serverLock.Lock()
	defer z.serverLock.Unlock()
	if len(z.registeredServiceMap) > 0 {
		for _, url := range z.registeredServiceMap {
			z.doRegister(url)
		}
		vlog.Infoln("[ZkRegistry] reconnect: register services success", z.registeredServiceMap)
	}
	if len(z.availableServiceMap) > 0 {
		for _, url := range z.availableServiceMap {
			z.doAvailable(url)
		}
		vlog.Infoln("[ZkRegistry] reconnect: available services success", z.availableServiceMap)
	}
}

func (z *ZkRegistry) reconnectClient() {
	z.clientLock.Lock()
	defer z.clientLock.Unlock()
	if len(z.subscribedServiceMap) > 0 {
		for _, serviceMap := range z.subscribedServiceMap {
			for url, listener := range serviceMap {
				z.doSubscribe(url, listener)
			}
		}
		vlog.Infoln("[ZkRegistry] reconnect: subscribe services success")
	}
	if len(z.subscribedCommandMap) > 0 {
		for _, commandMap := range z.subscribedCommandMap {
			for url, listener := range commandMap {
				z.doSubscribeCommand(url, listener)
			}
		}
		vlog.Infoln("[ZkRegistry] reconnect: subscribe commands success")
	}
}

//todo：锁跟行为拆分，是否比多把锁更优？
func (z *ZkRegistry) Register(url *motan.URL) {
	z.serverLock.Lock()
	defer z.serverLock.Unlock()
	vlog.Infof("[ZkRegistry] register service. url:%s\n", url.GetIdentity())
	z.doRegister(url)
}

func (z *ZkRegistry) doRegister(url *motan.URL) {
	if url.Group == "" || url.Path == "" || url.Host == "" {
		vlog.Errorf("[ZkRegistry] register service fail. invalid url:%s\n", url.GetIdentity())
	}
	if IsAgent(url) {
		z.createNode(url, ZkNodeTypeAgent)
	} else {
		z.removeNode(url, ZkNodeTypeServer)
		z.createNode(url, ZkNodeTypeUnavailableServer)
		z.registeredServiceMap[url.GetIdentity()] = url
	}
}

func (z *ZkRegistry) UnRegister(url *motan.URL) {
	z.serverLock.Lock()
	defer z.serverLock.Unlock()
	vlog.Infof("[ZkRegistry] unregister service. url:%s\n", url.GetIdentity())
	z.doUnRegister(url)
}

func (z *ZkRegistry) doUnRegister(url *motan.URL) {
	z.removeNode(url, ZkNodeTypeServer)
	z.removeNode(url, ZkNodeTypeUnavailableServer)
	delete(z.registeredServiceMap, url.GetIdentity())
}

func (z *ZkRegistry) Subscribe(url *motan.URL, listener motan.NotifyListener) {
	if !z.IsAvailable() {
		return
	}
	z.clientLock.Lock()
	defer z.clientLock.Unlock()
	vlog.Infof("[ZkRegistry] subscribe service. url:%s\n", url.GetIdentity())
	z.doSubscribe(url, listener)
}

//todo: 是否可以和doSubscribeCommand合并，CommandNotifyListener，NotifyListene如何兼容
func (z *ZkRegistry) doSubscribe(url *motan.URL, listener motan.NotifyListener) {
	subKey := GetSubKey(url)
	//todo: 此map如何设计，有没有可能同一个service同一个url使用不同的listener？
	if listeners, ok := z.subscribedServiceMap[subKey]; ok {
		if _, exist := listeners[url]; !exist {
			listeners[url] = listener
		}
	}
	lMap := make(map[*motan.URL]motan.NotifyListener)
	lMap[url] = listener
	z.subscribedServiceMap[subKey] = lMap
	serverPath := toNodeTypePath(url, ZkNodeTypeServer)
	if _, _, ch, err := z.zkConn.ChildrenW(serverPath); err == nil {
		unsubscribeChan, ok := z.watchSwitcherMap[serverPath]
		if !ok {
			z.watchSwitcherMap[serverPath] = make(chan bool)
			unsubscribeChan = z.watchSwitcherMap[serverPath]
		}
		vlog.Infof("[ZkRegistry] start watch children. path:%s\n", serverPath)
		url.PutParam(motan.NodeTypeKey, motan.NodeTypeReferer) // all subscribe url must as referer
		if url.Host == "" {
			url.Host = motan.GetLocalIP()
		}
		z.createNode(url, ZkNodeTypeClient) // register as rpc client
		go func() {
			defer motan.HandlePanic(nil)
			for {
				select {
				case evt := <-ch:
					if evt.Type == zk.EventNodeChildrenChanged {
						if nodes, _, chx, err := z.zkConn.ChildrenW(serverPath); err == nil {
							z.buildSnapShotNodes(nodes, url)
							ch = chx
							if listeners, ok := z.subscribedServiceMap[subKey]; ok && len(nodes) > 0 {
								for _, l := range listeners {
									l.Notify(z.url, z.nodeChildsToURLs(url, serverPath, nodes))
									vlog.Infof("[ZkRegistry] notify nodes:%+v\n", nodes)
								}
							}
						} else {
							vlog.Errorln("[ZkRegistry] watch children error. err:", err)
						}
					} else if evt.Type == zk.EventNotWatching {
						vlog.Infoln("[ZkRegistry] not watch children. path:", serverPath)
						return
					}
				case checkWatch := <-unsubscribeChan:
					if !checkWatch {
						close(unsubscribeChan)
						delete(z.watchSwitcherMap, serverPath)
						return
					}
				}
			}
		}()
	} else {
		vlog.Errorf("[ZkRegistry] subscribe service error. err:%v\n", err)
	}
}

//todo：s大不大写？
func (z *ZkRegistry) Unsubscribe(url *motan.URL, listener motan.NotifyListener) {
	z.clientLock.Lock()
	defer z.clientLock.Unlock()
	vlog.Infof("[ZkRegistry] unsubscribe service. url:%s\n", url.GetIdentity())
	z.doUnsubscribe(url, listener)
}

//todo: 是否可以和doUnSubscribeCommand合并，CommandNotifyListener，NotifyListene如何兼容
func (z *ZkRegistry) doUnsubscribe(url *motan.URL, listener motan.NotifyListener) {
	serverPath := toNodeTypePath(url, ZkNodeTypeServer)
	if unsubscribeChan, ok := z.watchSwitcherMap[serverPath]; ok {
		unsubscribeChan <- false
	}
	if listeners, ok := z.subscribedServiceMap[ GetSubKey(url)]; ok {
		delete(listeners, url)
	}
}

func (z *ZkRegistry) Discover(url *motan.URL) []*motan.URL {
	if !z.IsAvailable() {
		return nil
	}
	nodePath := toNodeTypePath(url, ZkNodeTypeServer) // discover server nodes
	nodes, _, err := z.zkConn.Children(nodePath)
	if err == nil {
		z.buildSnapShotNodes(nodes, url)
		return z.nodeChildsToURLs(url, nodePath, nodes)
	}
	vlog.Errorf("[ZkRegistry] discover service error! url:%s, err:%v\n", url.GetIdentity(), err)
	return nil
}

func (z *ZkRegistry) SubscribeCommand(url *motan.URL, listener motan.CommandNotifyListener) {
	if !z.IsAvailable() {
		return
	}
	z.clientLock.Lock()
	defer z.clientLock.Unlock()
	vlog.Infof("[ZkRegistry] subscribe command. url:%s\n", url.GetIdentity())
	z.doSubscribeCommand(url, listener)
}

func (z *ZkRegistry) doSubscribeCommand(url *motan.URL, listener motan.CommandNotifyListener) {
	var commandPath string
	if IsAgent(url) {
		commandPath = toAgentCommandPath(url)
	} else {
		commandPath = toCommandPath(url)
	}
	//todo: 此map如何设计，有没有可能同一个command同一个url使用不同的listener？
	if listeners, ok := z.subscribedCommandMap[GetSubKey(url)]; ok {
		if _, ok := listeners[url]; !ok {
			listeners[url] = listener
		}
	}
	if isExist, _, err := z.zkConn.Exists(commandPath); err == nil {
		if !isExist {
			vlog.Warningf("[ZkRegistry] command didn't exists, path:%s\n", commandPath)
			return
		}
	} else {
		vlog.Errorf("[ZkRegistry] check command exists error:%v\n", err)
	}
	if _, _, ch, err := z.zkConn.GetW(commandPath); err == nil {
		unsubscribeChan, ok := z.watchSwitcherMap[commandPath]
		if !ok {
			z.watchSwitcherMap[commandPath] = make(chan bool)
			unsubscribeChan = z.watchSwitcherMap[commandPath]
		}
		vlog.Infof("[ZkRegistry] start watch command %s\n", commandPath)
		go func() {
			defer motan.HandlePanic(nil)
			for {
				select {
				case evt := <-ch:
					if evt.Type == zk.EventNodeDataChanged {
						if data, _, chx, err := z.zkConn.GetW(commandPath); err == nil {
							ch = chx
							cmdInfo := deserializeExtInfo(data)
							listener.NotifyCommand(z.url, cluster.ServiceCmd, cmdInfo)
							vlog.Infof("[ZkRegistry] command changed, path:%s, cmdInfo:%s\n", commandPath, cmdInfo)
						} else {
							vlog.Errorf("[ZkRegistry] command changed, get cmdInfo error, err:%v\n", err)
						}
					} else if evt.Type == zk.EventNotWatching {
						vlog.Infoln("[ZkRegistry] not watching commandPath:", commandPath)
						return
					}
				case checkWatch := <-unsubscribeChan:
					if !checkWatch {
						close(unsubscribeChan)
						delete(z.watchSwitcherMap, commandPath)
						return
					}
				}
			}
		}()
	} else {
		vlog.Errorf("[ZkRegistry] subscribe command error. err:%v, commandPath:%s, url:%v\n", err, commandPath, url)
	}
}

func (z *ZkRegistry) UnSubscribeCommand(url *motan.URL, listener motan.CommandNotifyListener) {
	z.clientLock.Lock()
	defer z.clientLock.Unlock()
	vlog.Infof("[ZkRegistry] unsubscribe command. url:%s\n", url.GetIdentity())
	z.doUnSubscribeCommand(url, listener)
}

func (z *ZkRegistry) doUnSubscribeCommand(url *motan.URL, listener motan.CommandNotifyListener) {
	var commandPath string
	if IsAgent(url) {
		commandPath = toAgentCommandPath(url)
	} else {
		commandPath = toCommandPath(url)
	}
	if unsubscribeChan, ok := z.watchSwitcherMap[commandPath]; ok {
		unsubscribeChan <- false
	}
	if listeners, ok := z.subscribedCommandMap[ GetSubKey(url)]; ok {
		delete(listeners, url)
	}
}

func (z *ZkRegistry) DiscoverCommand(url *motan.URL) string {
	if !z.IsAvailable() {
		return ""
	}
	var res string
	var commandPath string
	if IsAgent(url) {
		commandPath = toAgentCommandPath(url)
	} else {
		commandPath = toCommandPath(url)
	}
	if isExist, _, err := z.zkConn.Exists(commandPath); err == nil {
		if !isExist {
			vlog.Warningf("[ZkRegistry] command didn't exist, path:%s\n", commandPath)
			return res
		}
	} else {
		vlog.Errorf("[ZkRegistry] command check error:%v\n", err)
		return res
	}
	if data, _, err := z.zkConn.Get(commandPath); err == nil {
		vlog.Infof("[ZkRegistry] discover command. path:%s\n", commandPath)
		res = deserializeExtInfo(data)
	} else {
		vlog.Errorf("[ZkRegistry] discover command error. url:%s, err:%s\n", url.GetIdentity(), err.Error())
	}
	return res
}

func (z *ZkRegistry) Available(url *motan.URL) {
	z.serverLock.Lock()
	z.serverLock.Unlock()
	z.doAvailable(url)
}

func (z *ZkRegistry) doAvailable(url *motan.URL) {
	if url == nil {
		for _, u := range z.registeredServiceMap {
			z.removeNode(u, ZkNodeTypeUnavailableServer)
			z.createNode(u, ZkNodeTypeServer)
			z.availableServiceMap[u.GetIdentity()] = url
		}
	} else {
		z.removeNode(url, ZkNodeTypeUnavailableServer)
		z.createNode(url, ZkNodeTypeServer)
		z.availableServiceMap[url.GetIdentity()] = url
	}
}

func (z *ZkRegistry) Unavailable(url *motan.URL) {
	z.serverLock.Lock()
	z.serverLock.Unlock()
	z.doUnavailable(url)
}

func (z *ZkRegistry) doUnavailable(url *motan.URL) {
	if url == nil {
		for _, u := range z.registeredServiceMap {
			z.removeNode(u, ZkNodeTypeServer)
			z.createNode(u, ZkNodeTypeUnavailableServer)
			delete(z.availableServiceMap, u.GetIdentity())
		}
	} else {
		z.removeNode(url, ZkNodeTypeServer)
		z.createNode(url, ZkNodeTypeUnavailableServer)
		delete(z.availableServiceMap, url.GetIdentity())
	}
}

func (z *ZkRegistry) GetRegisteredServices() []*motan.URL {
	z.serverLock.Lock()
	defer z.serverLock.Unlock()
	return z.doGetRegisteredServices()
}

func (z *ZkRegistry) doGetRegisteredServices() []*motan.URL {
	urls := make([]*motan.URL, 0, len(z.registeredServiceMap))
	for _, u := range z.registeredServiceMap {
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

func (z *ZkRegistry) IsAvailable() bool {
	return z.available
}

func (z *ZkRegistry) setAvailable(available bool) {
	z.available = available
}

func (z *ZkRegistry) StartSnapshot(conf *motan.SnapshotConf) {
	if _, err := os.Stat(conf.SnapshotDir); os.IsNotExist(err) {
		if err := os.Mkdir(conf.SnapshotDir, 0774); err != nil {
			vlog.Errorln("[ZkRegistry] make directory error. err:" + err.Error())
		}
	}
	go func(z *ZkRegistry) {
		defer motan.HandlePanic(nil)
		ticker := time.NewTicker(conf.SnapshotInterval)
		for range ticker.C {
			saveSnapshot(conf.SnapshotDir, z.snapshotNodeMap)
		}
	}(z)
}

func (z *ZkRegistry) buildSnapShotNodes(nodes []string, url *motan.URL) {
	nodeRsSnapshotLock.Lock()
	defer nodeRsSnapshotLock.Unlock()
	serviceNode := ServiceNode{
		Group: url.Group,
		Path:  url.Path,
	}
	nodeInfos := make([]SnapShotNodeInfo, 0, len(nodes))
	for _, addr := range nodes {
		nodeInfos = append(nodeInfos, SnapShotNodeInfo{Addr: addr})
	}
	serviceNode.Nodes = nodeInfos
	z.snapshotNodeMap[getNodeKey(url)] = serviceNode
}

func (z *ZkRegistry) removeNode(url *motan.URL, nodeType string) {
	if !z.IsAvailable() {
		return
	}
	var nodePath string
	if nodeType == ZkNodeTypeAgent {
		nodePath = toAgentNodePath(url)
	} else {
		nodePath = toNodePath(url, nodeType)
	}
	isExist, stats, err := z.zkConn.Exists(nodePath)
	if err == nil && isExist {
		if err = z.zkConn.Delete(nodePath, stats.Version); err == nil {
			return
		}
	}
	if err != nil {
		vlog.Errorf("[ZkRegistry] remove node error. err:%v, isExist:%v\n", err, isExist)
	}
}

func (z *ZkRegistry) createNode(url *motan.URL, nodeType string) {
	if !z.IsAvailable() {
		return
	}
	var typePath string
	var nodePath string
	if nodeType == ZkNodeTypeAgent {
		typePath = toAgentNodeTypePath(url)
		nodePath = toAgentNodePath(url)
	} else {
		typePath = toNodeTypePath(url, nodeType)
		nodePath = toNodePath(url, nodeType)
	}
	z.removeNode(url, nodeType)
	if isExist, _, err := z.zkConn.Exists(typePath); err != nil {
		vlog.Errorf("[ZkRegistry] create node error. path:%s, err:%v\n", nodePath, err)
		return
	} else if !isExist {
		z.createPersistent(typePath, true)
	}
	if _, err := z.zkConn.Create(nodePath, serializeExtInfo(url.ToExtInfo()), zk.FlagEphemeral, zk.WorldACL(zk.PermAll)); err != nil {
		vlog.Errorf("[ZkRegistry] create node error. path:%s, err:%v\n", nodePath, err)
		return
	}
}

func (z *ZkRegistry) createPersistent(path string, createParents bool) {
	if _, err := z.zkConn.Create(path, nil, 0, zk.WorldACL(zk.PermAll)); err != nil {
		if err == zk.ErrNoNode && createParents {
			parts := strings.Split(path, "/")
			parentPath := strings.Join(parts[:len(parts)-1], "/")
			z.createPersistent(parentPath, createParents)
			z.createPersistent(path, createParents)
			return
		}
		vlog.Errorf("[ZkRegistry] create persistent error. path:%s, err:%v\n", path, err)
	}
}

func serializeExtInfo(str string) []byte {
	return []byte(str)
}

func deserializeExtInfo(data []byte) string {
	if len(data) > 7 && binary.BigEndian.Uint16(data[:2]) == StreamMagicTag {
		if data[4] == ShortStringTag {
			return string(data[7:])
		} else if len(data) > 13 {
			return string(data[13:])
		}
	}
	return string(data)
}

func (z *ZkRegistry) nodeChildsToURLs(url *motan.URL, parentPath string, currentChilds []string) []*motan.URL {
	urls := make([]*motan.URL, 0, len(currentChilds))
	if currentChilds != nil {
		for _, node := range currentChilds {
			nodePath := parentPath + ZkPathSeparator + node
			data, _, err := z.zkConn.Get(nodePath)
			if err != nil {
				vlog.Errorln("[ZkRegistry] get node data error. err:" + err.Error())
			}
			newURL := &motan.URL{}
			nodeInfo := deserializeExtInfo(data)
			if nodeInfo != "" {
				newURL = motan.FromExtInfo(nodeInfo)
			} else {
				newURL = url.Copy()
				var host string
				port := 80
				if strings.Index(node, ":") > -1 {
					hp := strings.Split(node, ":")
					if len(hp) > 1 {
						host = hp[0]
						port, _ = strconv.Atoi(hp[1])
					}
				} else {
					host = node
				}
				newURL.Host = host
				newURL.Port = port
			}
			urls = append(urls, newURL)
		}
	}
	return urls
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
	return ZkRegistryNamespace + ZkPathSeparator + ZkNodeTypeAgent + ZkPathSeparator + url.GetParam(motan.ApplicationKey, "")
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
