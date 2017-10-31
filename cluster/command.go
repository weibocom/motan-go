package cluster

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"sync"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	CMD_Traffic_Control = iota
	CMD_Degrade         //service degrade
)

const (
	AGENT_CMD = iota
	SERVICE_CMD
)

const (
	RULE_PROTOCOL = "rule"
)

//warper registry for every cluster
type CommandRegistryWarper struct {
	cluster            *MotanCluster
	registry           motan.Registry
	notifyListener     motan.NotifyListener // e.g. cluster
	serviceCommandInfo string               // current service command
	agentCommandInfo   string               // current agent command
	mux                sync.Mutex
	ownGroupUrls       []*motan.Url
	otherGroupListener map[string]*serviceListener
	tcCommand          *ClientCommand //effective traffic control command
	degradeCommand     *ClientCommand //effective degrade command
}

type ClientCommand struct {
	Index       int      `json:"index"`
	Version     string   `json:"version"`
	CommandType int      `json:"commandType"`
	Dc          string   `json:"dc"`
	Pattern     string   `json:"pattern"`
	MergeGroups []string `json:"mergeGroups"`
	RouteRules  []string `json:"routeRules"`
	Remark      string   `json:"remark"`
}

type Command struct {
	ClientCommandList []ClientCommand `json:"clientCommandList"`
}

type serviceListener struct {
	referUrl *motan.Url
	urls     []*motan.Url
	crw      *CommandRegistryWarper
}

func (s *serviceListener) GetIdentity() string {
	return "serviceListener-" + s.referUrl.GetIdentity()
}

func (s *serviceListener) Notify(registryUrl *motan.Url, urls []*motan.Url) {
	vlog.Infof("serviceListener notify urls size is %d. refer: %v, registry: %v\n", len(urls), s.referUrl, registryUrl)
	s.urls = urls
	s.crw.getResultWithCommand(true)
}

func (s *serviceListener) unSubscribe(registry motan.Registry) {
	registry.Unsubscribe(s.referUrl, s)
	s.crw = nil
	s.referUrl = nil
	s.urls = nil
}

func (s *serviceListener) subscribe(registry motan.Registry) {
	registry.Subscribe(s.referUrl, s)
	s.urls = registry.Discover(s.referUrl)
}

type CmdList []ClientCommand

func (c CmdList) Len() int {
	return len(c)
}
func (c CmdList) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}
func (c CmdList) Less(i, j int) bool {
	return c[i].Index < c[j].Index
}

func (c *ClientCommand) MatchCmdPattern(url *motan.Url) bool {
	if c.Pattern == "*" || strings.HasPrefix(url.Path, c.Pattern) {
		return true
	}
	isRegMatch, err := regexp.MatchString(c.Pattern, url.Path)
	if err != nil {
		vlog.Errorf("check regexp command pattern fail. err :%s\n", err.Error())
	}
	if isRegMatch {
		return true
	}
	return false
}

func ParseCommand(commandInfo string) *Command {
	command := new(Command)
	if err := json.Unmarshal([]byte(commandInfo), command); err != nil {
		vlog.Infof("ParseCommand error, command: %s, err:%s\n", commandInfo, err.Error())
		return nil
	}
	return command
}

func GetCommandRegistryWarper(cluster *MotanCluster, registry motan.Registry) motan.Registry {
	cmdRegistry := &CommandRegistryWarper{cluster: cluster, registry: registry}
	cmdRegistry.ownGroupUrls = make([]*motan.Url, 0)
	cmdRegistry.otherGroupListener = make(map[string]*serviceListener)
	cmdRegistry.cluster = cluster
	return cmdRegistry
}

func (c *CommandRegistryWarper) Register(serverUrl *motan.Url) {
	c.registry.Register(serverUrl)
}

func (c *CommandRegistryWarper) UnRegister(serverUrl *motan.Url) {
	c.registry.UnRegister(serverUrl)
}

func (c *CommandRegistryWarper) Available(serverUrl *motan.Url) {
	c.registry.Available(serverUrl)
}

func (c *CommandRegistryWarper) Unavailable(serverUrl *motan.Url) {
	c.registry.Unavailable(serverUrl)
}

func (c *CommandRegistryWarper) GetRegisteredServices() []*motan.Url {
	return c.registry.GetRegisteredServices()
}

func (c *CommandRegistryWarper) Subscribe(url *motan.Url, listener motan.NotifyListener) {
	c.notifyListener = listener
	c.registry.Subscribe(url, c)
	if cr, ok := c.registry.(motan.DiscoverCommand); ok {
		cr.SubscribeCommand(url, c)
	}
}

func (c *CommandRegistryWarper) Unsubscribe(url *motan.Url, listener motan.NotifyListener) {
	c.registry.Unsubscribe(url, c)
	if cr, ok := c.registry.(motan.DiscoverCommand); ok {
		cr.UnSubscribeCommand(url, c)
	}
	c.clear()
}

func (c *CommandRegistryWarper) Discover(url *motan.Url) []*motan.Url {
	c.ownGroupUrls = c.registry.Discover(url)
	var result []*motan.Url
	if cr, ok := c.registry.(motan.DiscoverCommand); ok {
		serviceCmd := cr.DiscoverCommand(url)
		c.processCommand(SERVICE_CMD, serviceCmd)
		result = c.getResultWithCommand(false)
	} else {
		result = c.ownGroupUrls
	}
	return result
}

func (c *CommandRegistryWarper) StartSnapshot(conf *motan.SnapshotConf) {
	c.registry.StartSnapshot(conf)
}

func (c *CommandRegistryWarper) GetUrl() *motan.Url {
	return c.registry.GetUrl()
}

func (c *CommandRegistryWarper) clear() {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.tcCommand = nil
	c.degradeCommand = nil
	c.agentCommandInfo = ""
	c.serviceCommandInfo = ""
	c.ownGroupUrls = make([]*motan.Url, 0)
	for _, l := range c.otherGroupListener {
		l.unSubscribe(c.registry)
	}
	c.otherGroupListener = make(map[string]*serviceListener)
}

func (c *CommandRegistryWarper) getResultWithCommand(neednotify bool) []*motan.Url {
	c.mux.Lock()
	defer c.mux.Unlock()
	var result []*motan.Url = make([]*motan.Url, 0)
	if c.tcCommand != nil {
		vlog.Infof("%s get result with tc command.%+v\n", c.cluster.GetIdentity(), c.tcCommand)
		var buffer bytes.Buffer
		for _, group := range c.tcCommand.MergeGroups {
			g := strings.Split(group, ":")        //group name should not include ':'
			if c.cluster.GetUrl().Group == g[0] { // own group
				vlog.Infof("%s get result from own group: %s, group result size:%d\n", c.cluster.GetIdentity(), g[0], len(c.ownGroupUrls))
				for _, u := range c.ownGroupUrls {
					result = append(result, u)
				}
			} else if l, ok := c.otherGroupListener[g[0]]; ok {
				vlog.Infof("%s get result merge group: %s, group result size:%d\n", c.cluster.GetIdentity(), g[0], len(l.urls))
				for _, u := range l.urls {
					result = append(result, u)
				}
			} else {
				vlog.Warningf("TCcommand merge group not found. refer:%s, group notfound: %s, TCcommand %v\n", c.cluster.GetUrl(), g[0], c.tcCommand)
				continue
			}
			if buffer.Len() > 0 {
				buffer.WriteString(",")
			}
			buffer.WriteString(group)
		}
		if len(result) > 0 {
			url := buildRuleUrl(buffer.String())
			result = append(result, url) // add command rule url to the end of result.
		}
		result = proceeRoute(result, c.tcCommand.RouteRules)
	} else {
		result = c.ownGroupUrls
	}
	if neednotify {
		c.notifyListener.Notify(c.registry.GetUrl(), result)
	}
	vlog.Infof("%s get result with command. tcCommand: %t, degradeCommand:%t,  result size %d, will notify:%t\n", c.cluster.GetUrl().GetIdentity(), c.tcCommand != nil, c.degradeCommand != nil, len(result), neednotify)
	return result

}

func proceeRoute(urls []*motan.Url, routers []string) []*motan.Url {
	if len(urls) > 0 && len(routers) > 0 {
		lastUrls := urls
		for _, r := range routers {
			rs := strings.Split(r, "to")
			if len(rs) != 2 {
				vlog.Warningf("worng command router:%s is ignored!\n", r)
				continue
			}
			from := strings.TrimSpace(rs[0])
			to := strings.TrimSpace(rs[1])
			if len(from) > 0 && len(to) > 0 && isMatch(from, motan.GetLocalIp()) {
				newUrls := make([]*motan.Url, 0, len(urls))
				for _, u := range lastUrls {
					if u.Protocol == RULE_PROTOCOL || isMatch(to, u.Host) {
						newUrls = append(newUrls, u)
					}
				}
				lastUrls = newUrls
			}
		}
		return lastUrls
	}
	return urls
}

// is matching the router rule
func isMatch(router string, localIp string) bool {
	inverse := strings.HasPrefix(router, "!")
	if inverse {
		router = router[1:]
	}
	match := false
	if router == "*" {
		match = true
	} else if idx := strings.Index(router, "*"); idx > -1 {
		match = strings.HasPrefix(localIp, router[0:idx])
	} else {
		match = localIp == router
	}
	if inverse {
		match = !match
	}
	return match
}

// build a rule url which contains command info like 'weight'...
func buildRuleUrl(weight string) *motan.Url {
	params := make(map[string]string)
	params[motan.WeightKey] = weight
	url := &motan.Url{Protocol: RULE_PROTOCOL, Parameters: params}
	return url
}

func (c *CommandRegistryWarper) processCommand(commandType int, commandInfo string) bool {
	c.mux.Lock()
	defer c.mux.Unlock()
	needNotify := false
	switch commandType {
	case AGENT_CMD:
		if c.agentCommandInfo == commandInfo {
			vlog.Infoln("agent command same with current. ignored.")
			return false
		} else {
			c.agentCommandInfo = commandInfo
		}
	case SERVICE_CMD:
		if c.serviceCommandInfo == commandInfo {
			vlog.Infoln("service command same with current. ignored.")
			return false
		} else {
			c.serviceCommandInfo = commandInfo
		}
	default:
		vlog.Warningf("unknown command type %d\n", commandType)
		return false
	}

	// rebuild clientcommand
	var newTcCommand *ClientCommand
	var newDegradeCommand *ClientCommand
	if c.agentCommandInfo != "" { // agent command first
		newTcCommand, newDegradeCommand = mergeCommand(c.agentCommandInfo, c.cluster.GetUrl())
	}

	if c.serviceCommandInfo != "" {
		tc, dc := mergeCommand(c.serviceCommandInfo, c.cluster.GetUrl())
		if newTcCommand == nil {
			newTcCommand = tc
		}
		if newDegradeCommand == nil {
			newDegradeCommand = dc
		}
	}
	if newTcCommand != nil || (c.tcCommand != nil && newTcCommand == nil) {
		needNotify = true
	}

	//process all kinds commands
	c.tcCommand = newTcCommand
	if c.tcCommand == nil {
		vlog.Infof("%s process command result : no tc command. \n", c.cluster.GetUrl().GetIdentity())
		for _, v := range c.otherGroupListener {
			v.unSubscribe(c.registry)
		}
		c.otherGroupListener = make(map[string]*serviceListener)
	} else {
		vlog.Infof("%s process command result : has tc command. tc command will enable.command : %+v\n", c.cluster.GetUrl().GetIdentity(), newTcCommand)
		newOtherGroupListener := make(map[string]*serviceListener)
		for _, group := range c.tcCommand.MergeGroups {
			g := strings.Split(group, ":")
			if c.cluster.GetUrl().Group == g[0] { // own group already subscribe
				continue
			}
			if listener, ok := c.otherGroupListener[g[0]]; ok { // already exist
				vlog.Infof("commandwarper %s process tc command. reuse group %s\n", c.cluster.GetUrl().GetIdentity(), g[0])
				newOtherGroupListener[g[0]] = listener
				delete(c.otherGroupListener, g[0])
			} else {
				vlog.Infof("commandwarper %s process tc command. subscribe new group %s\n", c.cluster.GetUrl().GetIdentity(), g[0])
				newGroupUrl := c.cluster.GetUrl().Copy()
				newGroupUrl.Group = g[0]
				l := &serviceListener{crw: c, referUrl: newGroupUrl}
				l.subscribe(c.registry)
				newOtherGroupListener[g[0]] = l
			}

		}

		oldOtherGroupListener := c.otherGroupListener
		c.otherGroupListener = newOtherGroupListener
		// sync destroy
		for _, v := range oldOtherGroupListener {
			v.unSubscribe(c.registry)
		}
	}
	c.degradeCommand = newDegradeCommand
	if c.degradeCommand == nil {
		vlog.Infof("%s no degrade command. this cluster is available.\n", c.cluster.GetUrl().GetIdentity())
		c.cluster.available = true
	} else {
		vlog.Infof("%s has degrade command. this cluster will degrade.\n", c.cluster.GetUrl().GetIdentity())
		c.cluster.available = false
	}

	return needNotify
}

func mergeCommand(commandInfo string, url *motan.Url) (tcCommand *ClientCommand, degradeCommand *ClientCommand) {
	//only one command of a type will enable in same service. depends on the index of command
	cmd := ParseCommand(commandInfo)
	if cmd == nil {
		vlog.Warningf("pasre command fail, command is ignored. command info: %s\n", commandInfo)
	} else {
		var cmds CmdList = cmd.ClientCommandList
		sort.Sort(cmds)
		for _, c := range cmds {
			if c.MatchCmdPattern(url) {
				switch c.CommandType {
				case CMD_Traffic_Control:
					if tcCommand == nil {
						temp := c
						tcCommand = &temp
					} else {
						vlog.Warningf("traffic control command will igore by priority. command : %v", c)
					}
				case CMD_Degrade:
					temp := c
					degradeCommand = &temp
				}
			}
		}
	}
	return tcCommand, degradeCommand
}

func (c *CommandRegistryWarper) NotifyCommand(registryUrl *motan.Url, commandType int, commandInfo string) {
	vlog.Infof("%s receive Command notify. type:%d, command:%s\n", c.cluster.GetUrl().GetIdentity(), commandType, commandInfo)
	neednotify := c.processCommand(commandType, commandInfo)
	if neednotify {
		c.getResultWithCommand(neednotify)
	}
}

func (c *CommandRegistryWarper) Notify(registryUrl *motan.Url, urls []*motan.Url) {
	vlog.Infof("CommandRegistryWarper notify urls size is %d. refer: %v, registry: %v\n", len(urls), c.cluster.GetUrl(), registryUrl)
	c.ownGroupUrls = urls
	neednotify := false
	if c.tcCommand != nil {
		for _, group := range c.tcCommand.MergeGroups {
			g := strings.Split(group, ":")
			if g[0] == c.cluster.GetUrl().Group {
				neednotify = true
			}
		}
	} else {
		neednotify = true
	}

	if neednotify {
		c.getResultWithCommand(neednotify)
	}
}

func (c *CommandRegistryWarper) SetUrl(url *motan.Url) {
	c.registry.SetUrl(url)
}

func (c *CommandRegistryWarper) GetName() string {
	return "commandwarp:" + c.registry.GetName()
}

func (c *CommandRegistryWarper) GetIdentity() string {
	return c.notifyListener.GetIdentity()
}
