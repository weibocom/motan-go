package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	CMDTrafficControl = iota
	CMDDegrade        //service degrade
	CMDSwitcher
)

const (
	AgentCmd = iota
	ServiceCmd
)

const (
	RuleProtocol = "rule"
)

var oldSwitcherMap = make(map[string]bool) //Save the default value before the switcher last called

// CommandRegistryWrapper wrapper registry for every cluster
type CommandRegistryWrapper struct {
	cluster            *MotanCluster
	registry           motan.Registry
	notifyListener     motan.NotifyListener // e.g. cluster
	serviceCommandInfo string               // current service command
	agentCommandInfo   string               // current agent command
	mux                sync.Mutex
	ownGroupURLs       []*motan.URL
	otherGroupListener map[string]*serviceListener
	tcCommand          *ClientCommand //effective traffic control command
	degradeCommand     *ClientCommand //effective degrade command
	switcherCommand    *ClientCommand
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
	referURL *motan.URL
	urls     []*motan.URL
	crw      *CommandRegistryWrapper

	// cached identity
	identity motan.AtomicString
}

func (s *serviceListener) GetIdentity() string {
	id := s.identity.Load()
	if id == "" {
		id = fmt.Sprintf("serviceListener-%p-%s", s, s.referURL.GetIdentity())
		s.identity.Store(id)
	}
	return id
}

func (s *serviceListener) Notify(registryURL *motan.URL, urls []*motan.URL) {
	vlog.Infof("serviceListener notify urls size is %d. refer: %v, registry: %v", len(urls), s.referURL, registryURL)
	if s.crw == nil {
		vlog.Infof("serviceListener maybe unSubscribed. notify will ignore. s:%+v", s)
		return
	}
	s.urls = urls
	s.crw.getResultWithCommand(true)
}

func (s *serviceListener) unSubscribe(registry motan.Registry) {
	registry.Unsubscribe(s.referURL, s)
	s.crw = nil
	s.referURL = nil
	s.urls = nil
}

func (s *serviceListener) subscribe(registry motan.Registry) {
	// this listener should not reuse, so let it crash when this listener is resubscribe after unsubscribe.
	registry.Subscribe(s.referURL, s)
	s.urls = registry.Discover(s.referURL)
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

func (c *ClientCommand) MatchCmdPattern(url *motan.URL) bool {
	if c.CommandType == CMDSwitcher {
		return true
	}
	if c.Pattern == "*" || strings.HasPrefix(url.Path, c.Pattern) {
		return true
	}
	isRegMatch, err := regexp.MatchString(c.Pattern, url.Path)
	if err != nil {
		vlog.Errorf("check regexp command pattern fail. err :%s", err.Error())
	}
	if isRegMatch {
		return true
	}
	return false
}

func ParseCommand(commandInfo string) *Command {
	command := new(Command)
	if err := json.Unmarshal([]byte(commandInfo), command); err != nil {
		vlog.Infof("ParseCommand error, command: %s, err:%s", commandInfo, err.Error())
		return nil
	}
	return command
}

func GetCommandRegistryWrapper(cluster *MotanCluster, registry motan.Registry) motan.Registry {
	cmdRegistry := &CommandRegistryWrapper{cluster: cluster, registry: registry}
	cmdRegistry.ownGroupURLs = make([]*motan.URL, 0)
	cmdRegistry.otherGroupListener = make(map[string]*serviceListener)
	cmdRegistry.cluster = cluster
	return cmdRegistry
}

func (c *CommandRegistryWrapper) Register(serverURL *motan.URL) {
	c.registry.Register(serverURL)
}

func (c *CommandRegistryWrapper) UnRegister(serverURL *motan.URL) {
	c.registry.UnRegister(serverURL)
}

func (c *CommandRegistryWrapper) Available(serverURL *motan.URL) {
	c.registry.Available(serverURL)
}

func (c *CommandRegistryWrapper) Unavailable(serverURL *motan.URL) {
	c.registry.Unavailable(serverURL)
}

func (c *CommandRegistryWrapper) GetRegisteredServices() []*motan.URL {
	return c.registry.GetRegisteredServices()
}

func (c *CommandRegistryWrapper) Subscribe(url *motan.URL, listener motan.NotifyListener) {
	c.notifyListener = listener
	c.registry.Subscribe(url, c)
	if cr, ok := c.registry.(motan.DiscoverCommand); ok {
		cr.SubscribeCommand(url, c)
	}
}

func (c *CommandRegistryWrapper) Unsubscribe(url *motan.URL, listener motan.NotifyListener) {
	c.registry.Unsubscribe(url, c)
	if cr, ok := c.registry.(motan.DiscoverCommand); ok {
		cr.UnSubscribeCommand(url, c)
	}
	c.clear()
}

func (c *CommandRegistryWrapper) Discover(url *motan.URL) []*motan.URL {
	c.ownGroupURLs = c.registry.Discover(url)
	var result []*motan.URL
	if cr, ok := c.registry.(motan.DiscoverCommand); ok {
		serviceCmd := cr.DiscoverCommand(url)
		c.processCommand(ServiceCmd, serviceCmd)
		result = c.getResultWithCommand(false)
	} else {
		result = c.ownGroupURLs
	}
	return result
}

func (c *CommandRegistryWrapper) StartSnapshot(conf *motan.SnapshotConf) {
	c.registry.StartSnapshot(conf)
}

func (c *CommandRegistryWrapper) GetURL() *motan.URL {
	return c.registry.GetURL()
}

func (c *CommandRegistryWrapper) clear() {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.tcCommand = nil
	c.degradeCommand = nil
	c.agentCommandInfo = ""
	c.serviceCommandInfo = ""
	c.ownGroupURLs = make([]*motan.URL, 0)
	for _, l := range c.otherGroupListener {
		l.unSubscribe(c.registry)
	}
	c.otherGroupListener = make(map[string]*serviceListener)
}

func (c *CommandRegistryWrapper) getResultWithCommand(needNotify bool) []*motan.URL {
	c.mux.Lock()
	defer c.mux.Unlock()
	result := make([]*motan.URL, 0)
	if c.tcCommand != nil {
		vlog.Infof("%s get result with tc command.%+v", c.cluster.GetIdentity(), c.tcCommand)
		var buffer bytes.Buffer
		for _, group := range c.tcCommand.MergeGroups {
			g := strings.Split(group, ":")        //group name should not include ':'
			if c.cluster.GetURL().Group == g[0] { // own group
				vlog.Infof("%s get result from own group: %s, group result size:%d", c.cluster.GetIdentity(), g[0], len(c.ownGroupURLs))
				for _, u := range c.ownGroupURLs {
					result = append(result, u)
				}
			} else if l, ok := c.otherGroupListener[g[0]]; ok {
				vlog.Infof("%s get result merge group: %s, group result size:%d", c.cluster.GetIdentity(), g[0], len(l.urls))
				for _, u := range l.urls {
					result = append(result, u)
				}
			} else {
				vlog.Warningf("TC command merge group not found. refer:%s, group not found: %s, TC command %v", c.cluster.GetURL().GetIdentity(), g[0], c.tcCommand)
				continue
			}
			if buffer.Len() > 0 {
				buffer.WriteString(",")
			}
			buffer.WriteString(group)
		}
		if len(result) > 0 {
			url := buildRuleURL(buffer.String())
			result = append(result, url) // add command rule url to the end of result.
		}
		result = processRoute(result, c.tcCommand.RouteRules)
		if len(result) == 0 {
			result = c.ownGroupURLs
			vlog.Warningf("TC command process failed, use default group. refer:%s, MergeGroups: %v, RouteRules %v", c.cluster.GetURL().GetIdentity(), c.tcCommand.MergeGroups, c.tcCommand.RouteRules)
		}
	} else {
		result = c.ownGroupURLs
	}
	if needNotify {
		c.notifyListener.Notify(c.registry.GetURL(), result)
	}
	vlog.Infof("%s get result with command. tcCommand: %t, degradeCommand:%t,  result size %d, will notify:%t", c.cluster.GetURL().GetIdentity(), c.tcCommand != nil, c.degradeCommand != nil, len(result), needNotify)
	return result
}

func processRoute(urls []*motan.URL, routers []string) []*motan.URL {
	if len(urls) > 0 && len(routers) > 0 {
		lastURLs := urls
		for _, r := range routers {
			rs := strings.Split(r, "to")
			if len(rs) != 2 {
				vlog.Warningf("wrong command router:%s is ignored!", r)
				continue
			}
			from := strings.TrimSpace(rs[0])
			to := strings.TrimSpace(rs[1])
			if len(from) > 0 && len(to) > 0 && isMatch(from, motan.GetLocalIP()) {
				newURLs := make([]*motan.URL, 0, len(urls))
				for _, u := range lastURLs {
					if u.Protocol == RuleProtocol || isMatch(to, u.Host) {
						newURLs = append(newURLs, u)
					}
				}
				lastURLs = newURLs
			}
		}
		return lastURLs
	}
	return urls
}

// is matching the router rule
func isMatch(router string, localIP string) bool {
	inverse := strings.HasPrefix(router, "!")
	if inverse {
		router = router[1:]
	}
	match := false
	if router == "*" {
		match = true
	} else if idx := strings.Index(router, "*"); idx > -1 {
		match = strings.HasPrefix(localIP, router[0:idx])
	} else {
		match = localIP == router
	}
	if inverse {
		match = !match
	}
	return match
}

// build a rule url which contains command info like 'weight'...
func buildRuleURL(weight string) *motan.URL {
	params := make(map[string]string)
	params[motan.WeightKey] = weight
	url := &motan.URL{Protocol: RuleProtocol, Parameters: params}
	return url
}

func (c *CommandRegistryWrapper) processCommand(commandType int, commandInfo string) bool {
	c.mux.Lock()
	defer c.mux.Unlock()
	needNotify := false
	switch commandType {
	case AgentCmd:
		if c.agentCommandInfo == commandInfo {
			vlog.Infoln("agent command same with current. ignored.")
			return false
		}
		c.agentCommandInfo = commandInfo
	case ServiceCmd:
		if c.serviceCommandInfo == commandInfo {
			vlog.Infoln("service command same with current. ignored.")
			return false
		}
		c.serviceCommandInfo = commandInfo
	default:
		vlog.Warningf("unknown command type %d", commandType)
		return false
	}

	// rebuild clientCommand
	var newTcCommand *ClientCommand
	var newDegradeCommand *ClientCommand
	var newSwitcherCommand *ClientCommand
	if c.agentCommandInfo != "" { // agent command first
		newTcCommand, newDegradeCommand, newSwitcherCommand = mergeCommand(c.agentCommandInfo, c.cluster.GetURL())
	}

	if c.serviceCommandInfo != "" {
		tc, dc, sc := mergeCommand(c.serviceCommandInfo, c.cluster.GetURL())
		if newTcCommand == nil {
			newTcCommand = tc
		}
		if newDegradeCommand == nil {
			newDegradeCommand = dc
		}
		if newSwitcherCommand == nil {
			newSwitcherCommand = sc
		}
	}
	if newTcCommand != nil || (c.tcCommand != nil && newTcCommand == nil) {
		needNotify = true
	}

	//process all kinds commands
	c.tcCommand = newTcCommand
	if c.tcCommand == nil {
		vlog.Infof("%s process command result : no tc command. ", c.cluster.GetURL().GetIdentity())
		for _, v := range c.otherGroupListener {
			v.unSubscribe(c.registry)
		}
		c.otherGroupListener = make(map[string]*serviceListener)
	} else {
		vlog.Infof("%s process command result : has tc command. tc command will enable.command : %+v", c.cluster.GetURL().GetIdentity(), newTcCommand)
		newOtherGroupListener := make(map[string]*serviceListener)
		for _, group := range c.tcCommand.MergeGroups {
			g := strings.Split(group, ":")
			if c.cluster.GetURL().Group == g[0] { // own group already subscribe
				continue
			}
			if listener, ok := c.otherGroupListener[g[0]]; ok { // already exist
				vlog.Infof("commandWrapper %s process tc command. reuse group %s", c.cluster.GetURL().GetIdentity(), g[0])
				newOtherGroupListener[g[0]] = listener
				delete(c.otherGroupListener, g[0])
			} else {
				vlog.Infof("commandWrapper %s process tc command. subscribe new group %s", c.cluster.GetURL().GetIdentity(), g[0])
				newGroupURL := c.cluster.GetURL().Copy()
				newGroupURL.Group = g[0]
				l := &serviceListener{crw: c, referURL: newGroupURL}
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
		vlog.Infof("%s no degrade command. this cluster is available.", c.cluster.GetURL().GetIdentity())
		c.cluster.available = true
	} else {
		vlog.Infof("%s has degrade command. this cluster will degrade.", c.cluster.GetURL().GetIdentity())
		c.cluster.available = false
	}

	//process switcher command
	switcherManger := motan.GetSwitcherManager()
	newSwitcherMap := make(map[string]bool)
	if newSwitcherCommand != nil {
		switchers := strings.Split(newSwitcherCommand.Pattern, ",")
		for _, switcherStr := range switchers {
			v := strings.Split(switcherStr, ":")
			if len(v) > 1 {
				if value, err := strconv.ParseBool(v[1]); err == nil {
					if switcher := switcherManger.GetSwitcher(v[0]); switcher != nil {
						if _, ok := oldSwitcherMap[v[0]]; !ok {
							oldSwitcherMap[v[0]] = switcher.IsOpen()
						}
						switcher.SetValue(value)
						newSwitcherMap[v[0]] = true //record current switcher names
					}
				}
			}
		}
	}
	for name, value := range oldSwitcherMap {
		if _, ok := newSwitcherMap[name]; !ok {
			switcherManger.GetSwitcher(name).SetValue(value) //restore default value
		} else {
			newSwitcherMap[name] = value //save default switcher
		}
	}
	oldSwitcherMap = newSwitcherMap
	c.switcherCommand = newSwitcherCommand

	return needNotify
}

func mergeCommand(commandInfo string, url *motan.URL) (tcCommand *ClientCommand, degradeCommand *ClientCommand, switcherCommand *ClientCommand) {
	//only one command of a type will enable in same service. depends on the index of command
	cmd := ParseCommand(commandInfo)
	if cmd == nil {
		vlog.Warningf("parse command fail, command is ignored. command info: %s", commandInfo)
	} else {
		var cmdList CmdList = cmd.ClientCommandList
		sort.Sort(cmdList)
		for _, c := range cmdList {
			if c.MatchCmdPattern(url) {
				switch c.CommandType {
				case CMDTrafficControl:
					if tcCommand == nil {
						temp := c
						tcCommand = &temp
					} else {
						vlog.Warningf("traffic control command will ignore by priority. command : %v", c)
					}
				case CMDDegrade:
					temp := c
					degradeCommand = &temp
				case CMDSwitcher:
					temp := c
					switcherCommand = &temp
				}
			}
		}
	}
	return tcCommand, degradeCommand, switcherCommand
}

func (c *CommandRegistryWrapper) NotifyCommand(registryURL *motan.URL, commandType int, commandInfo string) {
	vlog.Infof("%s receive Command notify. type:%d, command:%s", c.cluster.GetURL().GetIdentity(), commandType, commandInfo)
	needNotify := c.processCommand(commandType, commandInfo)
	if needNotify {
		c.getResultWithCommand(needNotify)
	}
}

func (c *CommandRegistryWrapper) Notify(registryURL *motan.URL, urls []*motan.URL) {
	vlog.Infof("CommandRegistryWrapper notify urls size is %d. refer: %v, registry: %v", len(urls), c.cluster.GetURL(), registryURL)
	c.ownGroupURLs = urls
	needNotify := false
	if c.tcCommand != nil {
		for _, group := range c.tcCommand.MergeGroups {
			g := strings.Split(group, ":")
			if g[0] == c.cluster.GetURL().Group {
				needNotify = true
			}
		}
	} else {
		needNotify = true
	}

	if needNotify {
		c.getResultWithCommand(needNotify)
	}
}

func (c *CommandRegistryWrapper) SetURL(url *motan.URL) {
	c.registry.SetURL(url)
}

func (c *CommandRegistryWrapper) GetName() string {
	return "commandWrapper:" + c.registry.GetName()
}

func (c *CommandRegistryWrapper) GetIdentity() string {
	return c.notifyListener.GetIdentity()
}
