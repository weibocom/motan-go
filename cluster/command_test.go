package cluster

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/assert"
	"strconv"
	"strings"
	"testing"

	motan "github.com/weibocom/motan-go/core"
)

func TestCommandParse(t *testing.T) {
	cmds := make([]string, 0, 10)
	cmds = append(cmds, buildCmd(1, CMDTrafficControl, "*", "\"openapi-tc-test-rpc:1\",\"openapi-yf-test-rpc:1\"", ""))
	cmds = append(cmds, buildCmd(2, CMDTrafficControl, "*", "\"openapi-tc-test-rpc:1\",\"openapi-yf-test-rpc:1\"", "  \"10.73.1.* to 10.75.1.*\""))
	cl := buildCmdList(cmds)
	cmd := ParseCommand(cl)
	if cmd == nil {
		t.Errorf("parse command fail. cmd %s\n", cl)
	}
}

func TestProcessRouter(t *testing.T) {
	urls := buildURLs("group0")
	router := newRouter(" 10.73.1.* to 10.75.1.* ")

	//not match
	*motan.LocalIP = "10.75.0.8"
	result := processRoute(urls, router)
	checkSize(len(result), len(urls), t)

	// prefix match
	*motan.LocalIP = "10.73.1.8"
	result = processRoute(urls, router)
	checkSize(len(result), 2, t)
	checkHost(result, func(host string) bool {
		return !strings.HasPrefix(host, "10.75.1")
	}, t)

	// exact match
	router = newRouter("10.75.0.8 to 10.73.1.*")
	*motan.LocalIP = "10.75.0.8"
	result = processRoute(urls, router)
	checkSize(len(result), 2, t)
	checkHost(result, func(host string) bool {
		return !strings.HasPrefix(host, "10.73.1")
	}, t)

	// * match
	router = newRouter(" * to 10.75.*")
	*motan.LocalIP = "10.108.0.8"
	result = processRoute(urls, router)
	checkSize(len(result), 4, t)
	checkHost(result, func(host string) bool {
		return !strings.HasPrefix(host, "10.75")
	}, t)

	// multi rules
	router = newRouter(" * to 10.75.*", "10.108.* to 10.77.1.* ")
	*motan.LocalIP = "10.108.0.8"
	result = processRoute(urls, router)
	checkSize(len(result), 0, t)

	router = newRouter(" * to 10.75.*", "10.108.* to 10.75.1.* ")
	result = processRoute(urls, router)
	checkSize(len(result), 2, t)
	checkHost(result, func(host string) bool {
		return !(strings.HasPrefix(host, "10.77.1") || strings.HasPrefix(host, "10.75"))
	}, t)

	router = newRouter(" * to 10.*", "10.108.* to !10.73.1.* ")
	result = processRoute(urls, router)
	checkSize(len(result), 6, t)
	checkHost(result, func(host string) bool {
		return !(strings.HasPrefix(host, "10.77.1") || strings.HasPrefix(host, "10.75"))
	}, t)

	router = newRouter(" 10.79.* to !10.75.1.*", "10.108.* to 10.73.1.* ", "10.108.* to !10.73.1.5")
	*motan.LocalIP = "10.79.0.8"
	result = processRoute(urls, router)
	checkSize(len(result), 6, t)
	checkHost(result, func(host string) bool {
		return strings.HasPrefix(host, "10.75.1")
	}, t)

	*motan.LocalIP = "10.108.0.8"
	result = processRoute(urls, router)
	checkSize(len(result), 1, t)
	checkHost(result, func(host string) bool {
		return host != "10.73.1.3"
	}, t)
}

func TestGetResultWithCommand(t *testing.T) {
	crw := getDefaultCommandWrapper(false)
	cmds := make([]string, 0, 10)
	cmds = append(cmds, buildCmd(1, CMDTrafficControl, "*", "\"group0:3\",\"group1:5\"", "\" 10.79.* to !10.75.1.*\", \"10.108.* to 10.73.1.* \""))
	cmds = append(cmds, buildCmd(1, CMDDegrade, "com.weibo.test.TestService", "", ""))
	*motan.LocalIP = "10.108.0.8"
	cl := buildCmdList(cmds)
	listener := &MockListener{}
	crw.notifyListener = listener
	crw.processCommand(ServiceCmd, cl)
	crw.otherGroupListener["group0"].Notify(crw.registry.GetURL(), buildURLs("group0"))
	crw.otherGroupListener["group1"].Notify(crw.registry.GetURL(), buildURLs("group1"))

	// not notify
	listener.registryURL = nil
	listener.urls = nil
	urls := crw.getResultWithCommand(false)
	if listener.registryURL != nil || listener.urls != nil {
		t.Errorf("notify not correct! listener:%+v\n", listener)
	}
	// notify
	urls = crw.getResultWithCommand(true)
	if listener.registryURL != crw.registry.GetURL() || len(listener.urls) != len(urls) {
		t.Errorf("notify not correct! listener:%+v\n", listener)
	}
	fmt.Printf("srw:%+v, urls: %v\n", crw.notifyListener, urls)

	if len(urls) != 5 {
		t.Errorf("notify urls size not correct! listener:%+v\n", listener)
	}

	// check urls
	hasRule := false
	for _, u := range urls {
		if u.Protocol == RuleProtocol {
			hasRule = true
			continue
		}
		if !strings.HasPrefix(u.Host, "10.73.1") {
			t.Errorf("notify urls host correct! url:%+v\n", u)
		}
	}
	if !hasRule {
		t.Errorf("notify urls not has rule url! urls:%+v\n", urls)
	}
	// check runtime info
	checkDefaultCommandWrapperRuntimeInfo(crw, t)

	// has static command
	crw = getDefaultCommandWrapper(true)
	urls = crw.getResultWithCommand(false)
	crw.notifyListener = listener
	listener.urls = nil
	validateResult(crw, []string{"test-group1", "test-group2", "test-group3"}, t)

	cmds = make([]string, 0, 10)
	cmds = append(cmds, buildCmd(1, CMDTrafficControl, "*", "\"group0:3\",\"group1:5\"", ""))
	crw.processCommand(ServiceCmd, buildCmdList(cmds))
	validateResult(crw, []string{"group0", "group1"}, t)

	crw.processCommand(ServiceCmd, "")
	validateResult(crw, []string{"test-group1", "test-group2", "test-group3"}, t)

	// check runtime info
	checkDefaultCommandWrapperRuntimeInfo(crw, t)
}

func validateResult(crw *CommandRegistryWrapper, notifyGroups []string, t *testing.T) {
	if len(crw.otherGroupListener) != len(notifyGroups) {
		t.Errorf("other group listener with wrong size. otherGroupListener: %d, comand merge groups:%d, crw:%+v\n", len(crw.otherGroupListener), len(notifyGroups), crw)
	}
	size := 0
	for _, group := range notifyGroups {
		crw.otherGroupListener[group].Notify(crw.registry.GetURL(), buildURLs(group))
		size += len(crw.otherGroupListener[group].urls)
	}
	urls := crw.getResultWithCommand(false)
	if len(urls) != size+1 { // with weight rule
		t.Errorf("result size not correct. expect size:%d, actual size:%d", size+1, len(urls))
	}
}

func TestProcessCommand(t *testing.T) {
	crw := getDefaultCommandWrapper(false)
	cmds := make([]string, 0, 10)
	cmds = append(cmds, buildCmd(1, CMDTrafficControl, "*", "\"group0:3\",\"group1:5\"", ""))
	cmds = append(cmds, buildCmd(1, CMDDegrade, "com.weibo.test.TestService", "", ""))
	cl := buildCmdList(cmds)
	// process service cmd
	processServiceCmd(crw, cl, t)

	//process agent cmd, agent cmd will over service cmd
	// & process with router
	cmds = make([]string, 0, 10)
	cmds = append(cmds, buildCmd(1, CMDTrafficControl, "*", "\"group0:3\",\"group1:5\"", "\" * to 10.75.*\", \"10.108.* to 10.75.1.* \""))
	cmds = append(cmds, buildCmd(1, CMDDegrade, "com.weibo.test.TestService", "", ""))
	cl = buildCmdList(cmds)
	notify := crw.processCommand(AgentCmd, cl)
	if crw.agentCommandInfo != cl {
		t.Errorf("agentCommandInfo not correct! real:%s, expect:%s\n", crw.agentCommandInfo, cl)
	}
	if len(crw.tcCommand.RouteRules) != 2 {
		t.Errorf("tc command is not correct! tc command:%+v\n", crw.tcCommand)
	}

	//repeat command
	notify = crw.processCommand(AgentCmd, cl)
	if notify {
		t.Errorf("should not notify with same command! crw:%+v\n", crw)
	}

	//process degrade command
	crw.cluster.GetURL().Path = "com.weibo.test.TestService"
	crw.agentCommandInfo = ""
	notify = crw.processCommand(AgentCmd, cl)
	if crw.degradeCommand == nil || crw.cluster.IsAvailable() {
		t.Errorf("degrade command not enable! crw:%+v\n", crw)
	}

	// process abnormal command
	crw.serviceCommandInfo = ""
	notify = crw.processCommand(AgentCmd, "kljsdfoie")
	if !notify || crw.tcCommand != nil || crw.degradeCommand != nil {
		t.Errorf("abnormal command process not correct! notify: %t, crw:%+v\n", notify, crw)
	}
	fmt.Printf("notify:%t, crw:%+v\n", notify, crw)

	// check runtime info
	checkDefaultCommandWrapperRuntimeInfo(crw, t)

	// has static command
	crw = getDefaultCommandWrapper(true)
	processServiceCmd(crw, cl, t)
	notify = crw.processCommand(ServiceCmd, "")
	if !notify {
		t.Errorf("test empty service command fail! notify: %t, crw:%+v\n", notify, crw)
	}
	if len(crw.otherGroupListener) != len(crw.staticTcCommand.MergeGroups)-1 {
		t.Errorf("other group listener with wrong size. otherGroupListener: %d, static comand mergeGroups:%d, crw:%+v\n", len(crw.otherGroupListener), len(crw.staticTcCommand.MergeGroups), crw)
	}
	for _, group := range crw.staticTcCommand.MergeGroups {
		if group == crw.cluster.GetURL().Group {
			continue
		}
		if _, ok := crw.otherGroupListener[group]; !ok {
			t.Errorf("not have static group listener :%s, crw:%+v\n", group, crw)
		}
	}

	// check runtime info
	checkDefaultCommandWrapperRuntimeInfo(crw, t)
}

func processServiceCmd(crw *CommandRegistryWrapper, cl string, t *testing.T) {
	notify := crw.processCommand(ServiceCmd, cl)
	if crw.serviceCommandInfo != cl {
		t.Errorf("serviceCommandInfo not correct! real:%s, expect:%s\n", crw.serviceCommandInfo, cl)
	}
	if crw.tcCommand == nil {
		t.Errorf("tc command is nil! crw:%+v\n", crw)
	}
	if crw.degradeCommand != nil {
		t.Errorf("degrade command should be nil! crw:%+v\n", crw)
	}
	if notify != true {
		t.Errorf("process command notify not true! crw:%+v\n", crw)
	}
	if len(crw.tcCommand.MergeGroups) != 2 {
		t.Errorf("tc command is not correct! tc command:%+v\n", crw.tcCommand)
	}
	if len(crw.otherGroupListener) != 2 || crw.otherGroupListener["group0"] == nil || crw.otherGroupListener["group1"] == nil {
		t.Errorf("serviceCommandInfo not correct! real:%s, expect:%s\n", crw.serviceCommandInfo, cl)
	}
}

func TestMatchPattern(t *testing.T) {
	// *
	c := &ClientCommand{Pattern: "*"}
	url := &motan.URL{Path: "com.weibo.test.TestService"}
	m := c.MatchCmdPattern(url)
	if !m {
		t.Errorf("test match pattern fail! match:%t, command:%+v\n", m, c)
	}

	// prefix
	c.Pattern = "com.weibo"
	m = c.MatchCmdPattern(url)
	if !m {
		t.Errorf("test match pattern fail! match:%t, command:%+v\n", m, c)
	}

	//regexp
	c.Pattern = "[a-zA-Z0-9.]+"
	m = c.MatchCmdPattern(url)
	if !m {
		t.Errorf("test match pattern fail! match:%t, command:%+v\n", m, c)
	}

	// not match
	c.Pattern = "com.weibo.ttt"
	m = c.MatchCmdPattern(url)
	if m {
		t.Errorf("test match pattern fail! match:%t, command:%+v\n", m, c)
	}
}

func TestParseStaticCommand(t *testing.T) {
	ownGroup := "ownGroup"
	for _, unit := range []struct {
		mixGroup         string
		shouldHasCommand bool
		groups           []string
	}{
		{"test-group1", true, []string{ownGroup, "test-group1"}},
		{"test-group1 , test-group2", true, []string{ownGroup, "test-group1", "test-group2"}},
		{"test-group1 , test-group2,test-group3", true, []string{ownGroup, "test-group1", "test-group2", "test-group3"}},
		{"test-group1 , " + ownGroup + ",test-group2,test-group3", true, []string{ownGroup, "test-group1", "test-group2", "test-group3"}},
		{ownGroup, false, nil},
		{ownGroup + ",   ", false, nil},
		{"   ", false, nil},
		{"  , ", false, nil},
	} {
		cluster := initCluster()
		cluster.GetURL().Group = ownGroup
		cluster.url.Parameters[motan.MixGroups] = unit.mixGroup
		registry := cluster.extFactory.GetRegistry(RegistryURL)
		commandRegistry := GetCommandRegistryWrapper(cluster, registry).(*CommandRegistryWrapper)
		command := commandRegistry.staticTcCommand
		if unit.shouldHasCommand {
			if command == nil {
				t.Errorf("static command is nil. mix groups: %s", unit.mixGroup)
			}
			if len(unit.groups) != len(command.MergeGroups) {
				t.Errorf("merge group size not correct. mix groups: %s, expect size: %d, actual size:%d", unit.mixGroup, len(unit.groups), len(command.MergeGroups))
			}
			for i, group := range unit.groups {
				if group != command.MergeGroups[i] {
					t.Errorf("merge groups not same. mix groups: %s, index:%d, expect:%s, actual:%s", unit.mixGroup, i, group, command.MergeGroups[i])
				}
			}
		} else if command != nil {
			t.Errorf("command should nil. mix groups: %s", unit.mixGroup)
		}
	}
}

func newRouter(rules ...string) []string {
	router := make([]string, 0, 20)
	for _, r := range rules {
		router = append(router, r)
	}
	return router
}

func buildURLs(group string) []*motan.URL {
	urls := make([]*motan.URL, 0, 20)
	urls = append(urls, &motan.URL{Host: "10.75.1.3", Group: group})
	urls = append(urls, &motan.URL{Host: "10.75.1.5", Group: group})
	urls = append(urls, &motan.URL{Host: "10.75.2.3", Group: group})
	urls = append(urls, &motan.URL{Host: "10.75.3.5", Group: group})
	urls = append(urls, &motan.URL{Host: "10.73.1.3", Group: group})
	urls = append(urls, &motan.URL{Host: "10.73.1.5", Group: group})
	urls = append(urls, &motan.URL{Host: "10.77.1.3", Group: group})
	urls = append(urls, &motan.URL{Host: "10.77.1.5", Group: group})

	return urls
}

func checkSize(realSize int, expectSize int, t *testing.T) {
	if realSize != expectSize {
		t.Errorf("test router check size fail. real:%d, exp:%d\n", realSize, expectSize)
	}
}

func checkHost(urls []*motan.URL, f func(host string) bool, t *testing.T) {
	for _, u := range urls {
		if f(u.Host) {
			t.Errorf("test fail in prefix match pattern. url:%+v\n", u)
		}
	}
}

func checkDefaultCommandWrapperRuntimeInfo(crw *CommandRegistryWrapper, t *testing.T) {
	info := crw.GetRuntimeInfo()
	assert.NotNil(t, info)

	name, ok := info[motan.RuntimeNameKey]
	assert.True(t, ok)
	assert.Equal(t, crw.GetName(), name)

	weightsInfo, ok := info[motan.RuntimeWeightKey]
	assert.True(t, ok)
	assert.NotNil(t, weightsInfo)

	staticCommand, ok := info[motan.RuntimeStaticCommandKey]
	if crw.staticTcCommand == nil {
		assert.False(t, ok)
		assert.Nil(t, staticCommand)
	} else {
		assert.True(t, ok)
		assert.NotNil(t, staticCommand)
	}

	agentCommandInfo, ok := info[motan.RuntimeAgentCommandKey]
	assert.True(t, ok)
	assert.NotNil(t, agentCommandInfo)

	serviceCommandInfo, ok := info[motan.RuntimeServiceCommandKey]
	assert.True(t, ok)
	assert.NotNil(t, serviceCommandInfo)

	commandHistory, ok := info[motan.RuntimeCommandHistoryKey]
	assert.True(t, ok)
	assert.NotNil(t, commandHistory)

	notifyHistory, ok := info[motan.RuntimeNotifyHistoryKey]
	assert.True(t, ok)
	assert.NotNil(t, notifyHistory)
}

func getDefaultCommandWrapper(withStaticCommand bool) *CommandRegistryWrapper {
	cluster := initCluster()
	if withStaticCommand {
		cluster.url.Parameters[motan.MixGroups] = "test-group1,test-group2,test-group3"
	}
	registry := cluster.extFactory.GetRegistry(RegistryURL)
	return GetCommandRegistryWrapper(cluster, registry).(*CommandRegistryWrapper)
}

func buildCmd(index int, cmdType int, pattern string, mergeGroup string, routers string) string {
	return "{\"commandType\":" + strconv.Itoa(cmdType) + ", " +
		"\"index\": " + strconv.Itoa(index) + "," +
		"\"version\": \"1.0\"," +
		" \"dc\": \"yf\"," +
		" \"pattern\": \"" + pattern + "\"," +
		"\"mergeGroups\": [" + mergeGroup + "]," +
		"\"routeRules\": [" + routers + "]," +
		"\"remark\": \"any remark\"" +
		"}"
}

func buildCmdList(cmds []string) string {
	var buffer bytes.Buffer
	buffer.WriteString("{\"clientCommandList\" : [")
	for i, c := range cmds {
		if i != 0 {
			buffer.WriteString(",")
		}
		buffer.WriteString(c)
	}
	buffer.WriteString("]}")
	return buffer.String()
}

type MockListener struct {
	registryURL *motan.URL
	urls        []*motan.URL
}

func (m *MockListener) Notify(registryURL *motan.URL, urls []*motan.URL) {
	m.registryURL = registryURL
	m.urls = urls
}

func (m *MockListener) GetIdentity() string {
	return "mockListener"
}
