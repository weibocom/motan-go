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
	CMDTrafficControl = iota
	CMDDegrade        //service degrade
)

const (
	AgentCmd = iota
	ServiceCmd
)

const (
	RuleProtocol = "rule"
)

// CommandRegistryWarper warper registry for every cluster
type CommandRegistryWarper struct {
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
	crw      *CommandRegistryWarper
}

func (s *serviceListener) GetIdentity() string {
	return "serviceListener-" + s.referURL.GetIdentity()
}

func (s *serviceListener) Notify(registryURL *motan.URL, urls []*motan.URL) {
	vlog.Infof("serviceListener notify urls size is %d. refer: %v, registry: %v\n", len(urls), s.referURL, registryURL)
	if s.crw == nil {
		vlog.Infof("serviceListener maybe unSubscribed. notify will ignore. s:%+v\n", s)
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
	cmdRegistry.ownGroupURLs = make([]*motan.URL, 0)
	cmdRegistry.otherGroupListener = make(map[string]*serviceListener)
	cmdRegistry.cluster = cluster
	return cmdRegistry
}

func (c *CommandRegistryWarper) Register(serverURL *motan.URL) {
	c.registry.Register(serverURL)
}

func (c *CommandRegistryWarper) UnRegister(serverURL *motan.URL) {
	c.registry.UnRegister(serverURL)
}

func (c *CommandRegistryWarper) Available(serverURL *motan.URL) {
	c.registry.Available(serverURL)
}

func (c *CommandRegistryWarper) Unavailable(serverURL *motan.URL) {
	c.registry.Unavailable(serverURL)
}

func (c *CommandRegistryWarper) GetRegisteredServices() []*motan.URL {
	return c.registry.GetRegisteredServices()
}

func (c *CommandRegistryWarper) Subscribe(url *motan.URL, listener motan.NotifyListener) {
	c.notifyListener = listener
	c.registry.Subscribe(url, c)
	if cr, ok := c.registry.(motan.DiscoverCommand); ok {
		cr.SubscribeCommand(url, c)
	}
}

func (c *CommandRegistryWarper) Unsubscribe(url *motan.URL, listener motan.NotifyListener) {
	c.registry.Unsubscribe(url, c)
	if cr, ok := c.registry.(motan.DiscoverCommand); ok {
		cr.UnSubscribeCommand(url, c)
	}
	c.clear()
}

func (c *CommandRegistryWarper) Discover(url *motan.URL) []*motan.URL {
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

func (c *CommandRegistryWarper) StartSnapshot(conf *motan.SnapshotConf) {
	c.registry.StartSnapshot(conf)
}

func (c *CommandRegistryWarper) GetURL() *motan.URL {
	return c.registry.GetURL()
}

func (c *CommandRegistryWarper) clear() {
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

func (c *CommandRegistryWarper) getResultWithCommand(neednotify bool) []*motan.URL {
	c.mux.Lock()
	defer c.mux.Unlock()
	var result []*motan.URL = make([]*motan.URL, 0)
	if c.tcCommand != nil {
		vlog.Infof("%s get result with tc command.%+v\n", c.cluster.GetIdentity(), c.tcCommand)
		var buffer bytes.Buffer
		for _, group := range c.tcCommand.MergeGroups {
			g := strings.Split(group, ":")        //group name should not include ':'
			if c.cluster.GetURL().Group == g[0] { // own group
				vlog.Infof("%s get result from own group: %s, group result size:%d\n", c.cluster.GetIdentity(), g[0], len(c.ownGroupURLs))
				for _, u := range c.ownGroupURLs {
					result = append(result, u)
				}
			} else if l, ok := c.otherGroupListener[g[0]]; ok {
				vlog.Infof("%s get result merge group: %s, group result size:%d\n", c.cluster.GetIdentity(), g[0], len(l.urls))
				for _, u := range l.urls {
					result = append(result, u)
				}
			} else {
				vlog.Warningf("TCcommand merge group not found. refer:%s, group notfound: %s, TCcommand %v\n", c.cluster.GetURL(), g[0], c.tcCommand)
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
		result = proceeRoute(result, c.tcCommand.RouteRules)
	} else {
		result = c.ownGroupURLs
	}
	if neednotify {
		c.notifyListener.Notify(c.registry.GetURL(), result)
	}
	vlog.Infof("%s get result with command. tcCommand: %t, degradeCommand:%t,  result size %d, will notify:%t\n", c.cluster.GetURL().GetIdentity(), c.tcCommand != nil, c.degradeCommand != nil, len(result), neednotify)
	return result

}

func proceeRoute(urls []*motan.URL, routers []string) []*motan.URL {
	if len(urls) > 0 && len(routers) > 0 {
		lastURLs := urls
		for _, r := range routers {
			rs := strings.Split(r, "to")
			if len(rs) != 2 {
				vlog.Warningf("worng command router:%s is ignored!\n", r)
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

func (c *CommandRegistryWarper) processCommand(commandType int, commandInfo string) bool {
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
		vlog.Warningf("unknown command type %d\n", commandType)
		return false
	}

	// rebuild clientcommand
	var newTcCommand *ClientCommand
	var newDegradeCommand *ClientCommand
	if c.agentCommandInfo != "" { // agent command first
		newTcCommand, newDegradeCommand = mergeCommand(c.agentCommandInfo, c.cluster.GetURL())
	}

	if c.serviceCommandInfo != "" {
		tc, dc := mergeCommand(c.serviceCommandInfo, c.cluster.GetURL())
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
		vlog.Infof("%s process command result : no tc command. \n", c.cluster.GetURL().GetIdentity())
		for _, v := range c.otherGroupListener {
			v.unSubscribe(c.registry)
		}
		c.otherGroupListener = make(map[string]*serviceListener)
	} else {
		vlog.Infof("%s process command result : has tc command. tc command will enable.command : %+v\n", c.cluster.GetURL().GetIdentity(), newTcCommand)
		newOtherGroupListener := make(map[string]*serviceListener)
		for _, group := range c.tcCommand.MergeGroups {
			g := strings.Split(group, ":")
			if c.cluster.GetURL().Group == g[0] { // own group already subscribe
				continue
			}
			if listener, ok := c.otherGroupListener[g[0]]; ok { // already exist
				vlog.Infof("commandwarper %s process tc command. reuse group %s\n", c.cluster.GetURL().GetIdentity(), g[0])
				newOtherGroupListener[g[0]] = listener
				delete(c.otherGroupListener, g[0])
			} else {
				vlog.Infof("commandwarper %s process tc command. subscribe new group %s\n", c.cluster.GetURL().GetIdentity(), g[0])
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
		vlog.Infof("%s no degrade command. this cluster is available.\n", c.cluster.GetURL().GetIdentity())
		c.cluster.available = true
	} else {
		vlog.Infof("%s has degrade command. this cluster will degrade.\n", c.cluster.GetURL().GetIdentity())
		c.cluster.available = false
	}

	return needNotify
}

func mergeCommand(commandInfo string, url *motan.URL) (tcCommand *ClientCommand, degradeCommand *ClientCommand) {
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
				case CMDTrafficControl:
					if tcCommand == nil {
						temp := c
						tcCommand = &temp
					} else {
						vlog.Warningf("traffic control command will igore by priority. command : %v", c)
					}
				case CMDDegrade:
					temp := c
					degradeCommand = &temp
				}
			}
		}
	}
	return tcCommand, degradeCommand
}

func (c *CommandRegistryWarper) NotifyCommand(registryURL *motan.URL, commandType int, commandInfo string) {
	vlog.Infof("%s receive Command notify. type:%d, command:%s\n", c.cluster.GetURL().GetIdentity(), commandType, commandInfo)
	neednotify := c.processCommand(commandType, commandInfo)
	if neednotify {
		c.getResultWithCommand(neednotify)
	}
}

func (c *CommandRegistryWarper) Notify(registryURL *motan.URL, urls []*motan.URL) {
	vlog.Infof("CommandRegistryWarper notify urls size is %d. refer: %v, registry: %v\n", len(urls), c.cluster.GetURL(), registryURL)
	c.ownGroupURLs = urls
	neednotify := false
	if c.tcCommand != nil {
		for _, group := range c.tcCommand.MergeGroups {
			g := strings.Split(group, ":")
			if g[0] == c.cluster.GetURL().Group {
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

func (c *CommandRegistryWarper) SetURL(url *motan.URL) {
	c.registry.SetURL(url)
}

func (c *CommandRegistryWarper) GetName() string {
	return "commandwarp:" + c.registry.GetName()
}

func (c *CommandRegistryWarper) GetIdentity() string {
	return c.notifyListener.GetIdentity()
}
