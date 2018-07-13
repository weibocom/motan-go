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
	snapShotNodeMap      map[string]ServiceNode
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
	z.snapShotNodeMap = make(map[string]ServiceNode)
	z.StartSnapshot(GetSanpshotConf())
	z.watchSwitcherMap = make(map[string]chan bool)
	z.registeredServiceMap = make(map[string]*motan.URL)
	z.availableServiceMap = make(map[string]*motan.URL)
	addrs := strings.Split(z.url.GetAddressStr(), ",")
	c, ch, err := zk.Connect(addrs, z.sessionTimeout)
	if err != nil {
		vlog.Errorf("zk connect error:%v\n", err)
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
			z.reconnectServer()
			z.reconnectClient()
			vlog.Infoln("zookeeper reconnect success")
		}
	}
}

func (z *ZkRegistry) reconnectServer() {
	z.serverLock.Lock()
	defer z.serverLock.Unlock()
	if allRegisteredServices := z.doGetRegisteredServices(); len(allRegisteredServices) > 0 {
		for _, url := range allRegisteredServices {
			z.doRegister(url)
		}
		vlog.Infoln("reconnect zookeeper server: register services ", allRegisteredServices)
		for _, url := range z.availableServiceMap {
			z.doAvailable(url)
		}
		vlog.Infoln("reconnect zookeeper server: available services ", z.availableServiceMap)
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
		vlog.Infoln("reconnect zookeeper server: subscribe services")
	}
	if len(z.subscribedCommandMap) > 0 {
		for _, commandMap := range z.subscribedCommandMap {
			for url, listener := range commandMap {
				z.doSubscribeCommand(url, listener)
			}
		}
		vlog.Infoln("reconnect zookeeper server: subscribe commands")
	}
}

func (z *ZkRegistry) Register(url *motan.URL) {
	z.serverLock.Lock()
	defer z.serverLock.Unlock()
	vlog.Infof("start zk register %s\n", url.GetIdentity())
	z.doRegister(url)
}

func (z *ZkRegistry) doRegister(url *motan.URL) {
	if url.Group == "" || url.Path == "" || url.Host == "" {
		vlog.Errorf("register fail. invalid url:%s\n", url.GetIdentity())
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
	vlog.Infof("start subscribe service. url:%s\n", url.GetIdentity())
	z.doSubscribe(url, listener)
}

func (z *ZkRegistry) doSubscribe(url *motan.URL, listener motan.NotifyListener) {
	subKey := GetSubKey(url)
	if listeners, ok := z.subscribedServiceMap[subKey]; ok {
		if _, exist := listeners[url]; !exist {
			listeners[url] = listener //todo: 有没有可能同一个service同一个url使用不同的listener？
		}
	}
	lMap := make(map[*motan.URL]motan.NotifyListener)
	lMap[url] = listener
	z.subscribedServiceMap[subKey] = lMap
	serverPath := toNodeTypePath(url, ZkNodeTypeServer)
	if _, _, ch, err := z.zkConn.ChildrenW(serverPath); err == nil {
		vlog.Infof("start watch %s\n", subKey)
		url.PutParam(motan.NodeTypeKey, motan.NodeTypeReferer) // all subscribe url must as referer
		if url.Host == "" {
			url.Host = motan.GetLocalIP()
		}
		z.createNode(url, ZkNodeTypeClient) // register as rpc client
		go func() {
			defer motan.HandlePanic(nil)
			for {
				if evt := <-ch; evt.Type == zk.EventNodeChildrenChanged {
					if nodes, _, chx, err := z.zkConn.ChildrenW(serverPath); err == nil {
						z.buildSnapShotNodes(nodes, url)
						ch = chx
						if listeners, ok := z.subscribedServiceMap[subKey]; ok {
							for _, l := range listeners {
								l.Notify(z.url, z.nodeChildsToURLs(url, serverPath, nodes))
								vlog.Infof("EventNodeChildrenChanged %+v\n", nodes)
							}
						}
					} else {
						vlog.Errorln("watch children error:", err)
					}
				} else if evt.Type == zk.EventNotWatching {
					vlog.Infoln("EventNotWatching serverPath:", serverPath)
					return
				}
			}
		}()
	} else {
		vlog.Errorf("zk Subscribe error %v\n", err)
	}
}

func (z *ZkRegistry) Unsubscribe(url *motan.URL, listener motan.NotifyListener) {
	z.clientLock.Lock()
	defer z.clientLock.Unlock()
	z.doUnsubscribe(url, listener)
}

func (z *ZkRegistry) doUnsubscribe(url *motan.URL, listener motan.NotifyListener) {
	subKey := GetSubKey(url)
	if listeners, ok := z.subscribedServiceMap[subKey]; ok {
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
	vlog.Errorf("zookeeper registry discover fail! discover url:%s, err:%v\n", url.GetIdentity(), err)
	return nil
}

func (z *ZkRegistry) SubscribeCommand(url *motan.URL, listener motan.CommandNotifyListener) {
	if !z.IsAvailable() {
		return
	}
	z.clientLock.Lock()
	defer z.clientLock.Unlock()
	vlog.Infof("zookeeper subscribe command of %s\n", url.GetIdentity())
	z.doSubscribeCommand(url, listener)
}

func (z *ZkRegistry) doSubscribeCommand(url *motan.URL, listener motan.CommandNotifyListener) {
	var commandPath string
	if IsAgent(url) {
		commandPath = toAgentCommandPath(url)
	} else {
		commandPath = toCommandPath(url)
	}
	if listeners, ok := z.subscribedCommandMap[commandPath]; ok {
		if _, exist := listeners[url]; !exist {
			listeners[url] = listener //todo: 有没有可能同一个command同一个url使用不同的listener？
		}
	}
	if isExist, _, err := z.zkConn.Exists(commandPath); err == nil {
		if !isExist {
			vlog.Warningf("command didn't exists, path:%s\n", commandPath)
			return
		}
	} else {
		vlog.Errorf("check command exists error:%v\n", err)
	}
	if _, _, ch, err := z.zkConn.GetW(commandPath); err == nil {
		tempChan := z.watchSwitcherMap[commandPath]
		if tempChan == nil {
			z.watchSwitcherMap[commandPath] = make(chan bool)
			tempChan = z.watchSwitcherMap[commandPath]
		}
		vlog.Infof("start watch command %s\n", commandPath)
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
							vlog.Infof("command changed, path:%s, data:%s\n", commandPath, cmdInfo)
						} else {
							vlog.Errorf("command changed, get cmdInfo error, err:%v\n", err)
						}
					} else if evt.Type == zk.EventNotWatching {
						vlog.Infoln("EventNotWatching commandPath:", commandPath)
						return
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
		vlog.Errorf("zookeeper subscribe command error. err:%v, commandPath:%s, url:%v\n", err, commandPath, url)
	}
}

func (z *ZkRegistry) UnSubscribeCommand(url *motan.URL, listener motan.CommandNotifyListener) {
	z.clientLock.Lock()
	defer z.clientLock.Unlock()
	z.doUnSubscribeCommand(url, listener)
}

func (z *ZkRegistry) doUnSubscribeCommand(url *motan.URL, listener motan.CommandNotifyListener) {
	var commandPath string
	if IsAgent(url) {
		commandPath = toAgentCommandPath(url)
	} else {
		commandPath = toCommandPath(url)
	}
	z.watchSwitcherMap[commandPath] <- false
}

func (z *ZkRegistry) DiscoverCommand(url *motan.URL) string {
	if !z.IsAvailable() {
		return ""
	}
	vlog.Infof("zookeeper Discover command of %s\n", url.GetIdentity())
	var res string
	var commandPath string
	if IsAgent(url) {
		commandPath = toAgentCommandPath(url)
	} else {
		commandPath = toCommandPath(url)
	}
	if isExist, _, err := z.zkConn.Exists(commandPath); err == nil {
		if !isExist {
			vlog.Warningf("zookeeper command didn't exist, path:%s\n", commandPath)
			return res
		}
	} else {
		vlog.Errorf("zookeeper command check error:%v\n", err)
		return res
	}
	if data, _, err := z.zkConn.Get(commandPath); err == nil {
		vlog.Infof("zookeeper Discover command %s\n", commandPath)
		res = deserializeExtInfo(data)
	} else {
		vlog.Errorf("zookeeper DiscoverCommand error. url:%s, err:%s\n", url.GetIdentity(), err.Error())
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
			vlog.Errorln("make directory failed. err:" + err.Error())
		}
	}
	go func(z *ZkRegistry) {
		defer motan.HandlePanic(nil)
		ticker := time.NewTicker(conf.SnapshotInterval)
		for range ticker.C {
			saveSnapshot(conf.SnapshotDir, z.snapShotNodeMap)
		}
	}(z)
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
			vlog.Infof("remove node success. path:%s\n", nodePath)
			return
		}
	}
	if err != nil {
		vlog.Errorf("remove node error. err:%v, isExist:%v\n", err, isExist)
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
		vlog.Errorf("create node error. path:%s, err:%v\n", nodePath, err)
		return
	} else if !isExist {
		z.createPersistent(typePath, true)
	}
	if _, err := z.zkConn.Create(nodePath, serializeExtInfo(url.ToExtInfo()), zk.FlagEphemeral, zk.WorldACL(zk.PermAll)); err != nil {
		vlog.Errorf("create node error. path:%s, err:%v\n", nodePath, err)
		return
	}
	vlog.Infof("create node success. path:%s\n", nodePath)
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
		vlog.Errorf("create persistent error. path:%s, err:%v\n", path, err)
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

func (z *ZkRegistry) buildSnapShotNodes(nodes []string, url *motan.URL) {
	serviceNode := ServiceNode{
		Group: url.Group,
		Path:  url.Path,
	}
	nodeInfos := make([]SnapShotNodeInfo, 0, len(nodes))
	for _, addr := range nodes {
		nodeInfos = append(nodeInfos, SnapShotNodeInfo{Addr: addr})
	}
	serviceNode.Nodes = nodeInfos
	z.snapShotNodeMap[getNodeKey(url)] = serviceNode
}

func (z *ZkRegistry) nodeChildsToURLs(url *motan.URL, parentPath string, currentChilds []string) []*motan.URL {
	urls := make([]*motan.URL, 0, len(currentChilds))
	if currentChilds != nil {
		for _, node := range currentChilds {
			nodePath := parentPath + ZkPathSeparator + node
			data, _, err := z.zkConn.Get(nodePath)
			if err != nil {
				vlog.Errorln("get zk data fail!" + err.Error())
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
