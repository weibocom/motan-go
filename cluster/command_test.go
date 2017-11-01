package cluster

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"

	motan "github.com/weibocom/motan-go/core"
)

func TestCommandParse(t *testing.T) {
	cmds := make([]string, 0, 10)
	cmds = append(cmds, buildCmd(1, CMD_Traffic_Control, "*", "\"openapi-tc-test-rpc:1\",\"openapi-yf-test-rpc:1\"", ""))
	cmds = append(cmds, buildCmd(2, CMD_Traffic_Control, "*", "\"openapi-tc-test-rpc:1\",\"openapi-yf-test-rpc:1\"", "  \"10.73.1.* to 10.75.1.*\""))
	cl := buildCmdList(cmds)
	cmd := ParseCommand(cl)
	if cmd == nil {
		t.Errorf("parse command fail. cmd %s\n", cl)
	}
}

func TestProcessRouter(t *testing.T) {
	urls := buildUrls("group0")
	router := newRouter(" 10.73.1.* to 10.75.1.* ")

	//not match
	*motan.LocalIp = "10.75.0.8"
	result := proceeRoute(urls, router)
	checksize(len(result), len(urls), t)

	// prefix match
	*motan.LocalIp = "10.73.1.8"
	result = proceeRoute(urls, router)
	checksize(len(result), 2, t)
	checkHost(result, func(host string) bool {
		return !strings.HasPrefix(host, "10.75.1")
	}, t)

	// exact match
	router = newRouter("10.75.0.8 to 10.73.1.*")
	*motan.LocalIp = "10.75.0.8"
	result = proceeRoute(urls, router)
	checksize(len(result), 2, t)
	checkHost(result, func(host string) bool {
		return !strings.HasPrefix(host, "10.73.1")
	}, t)

	// * match
	router = newRouter(" * to 10.75.*")
	*motan.LocalIp = "10.108.0.8"
	result = proceeRoute(urls, router)
	checksize(len(result), 4, t)
	checkHost(result, func(host string) bool {
		return !strings.HasPrefix(host, "10.75")
	}, t)

	// multi rules
	router = newRouter(" * to 10.75.*", "10.108.* to 10.77.1.* ")
	*motan.LocalIp = "10.108.0.8"
	result = proceeRoute(urls, router)
	checksize(len(result), 0, t)

	router = newRouter(" * to 10.75.*", "10.108.* to 10.75.1.* ")
	result = proceeRoute(urls, router)
	checksize(len(result), 2, t)
	checkHost(result, func(host string) bool {
		return !(strings.HasPrefix(host, "10.77.1") || strings.HasPrefix(host, "10.75"))
	}, t)

	router = newRouter(" * to 10.*", "10.108.* to !10.73.1.* ")
	result = proceeRoute(urls, router)
	checksize(len(result), 6, t)
	checkHost(result, func(host string) bool {
		return !(strings.HasPrefix(host, "10.77.1") || strings.HasPrefix(host, "10.75"))
	}, t)

	router = newRouter(" 10.79.* to !10.75.1.*", "10.108.* to 10.73.1.* ", "10.108.* to !10.73.1.5")
	*motan.LocalIp = "10.79.0.8"
	result = proceeRoute(urls, router)
	checksize(len(result), 6, t)
	checkHost(result, func(host string) bool {
		return strings.HasPrefix(host, "10.75.1")
	}, t)

	*motan.LocalIp = "10.108.0.8"
	result = proceeRoute(urls, router)
	checksize(len(result), 1, t)
	checkHost(result, func(host string) bool {
		return host != "10.73.1.3"
	}, t)
}

func TestGetResultWithCommand(t *testing.T) {
	crw := getDefalultCommandWarper()
	cmds := make([]string, 0, 10)
	cmds = append(cmds, buildCmd(1, CMD_Traffic_Control, "*", "\"group0:3\",\"group1:5\"", "\" 10.79.* to !10.75.1.*\", \"10.108.* to 10.73.1.* \""))
	cmds = append(cmds, buildCmd(1, CMD_Degrade, "com.weibo.test.TestService", "", ""))
	*motan.LocalIp = "10.108.0.8"
	cl := buildCmdList(cmds)
	listener := &MockListener{}
	crw.notifyListener = listener
	crw.processCommand(SERVICE_CMD, cl)
	crw.otherGroupListener["group0"].Notify(crw.registry.GetUrl(), buildUrls("group0"))
	crw.otherGroupListener["group1"].Notify(crw.registry.GetUrl(), buildUrls("group1"))

	// not notify
	listener.registryUrl = nil
	listener.urls = nil
	urls := crw.getResultWithCommand(false)
	if listener.registryUrl != nil || listener.urls != nil {
		t.Errorf("notify not correct! listener:%+v\n", listener)
	}
	// notify
	urls = crw.getResultWithCommand(true)
	if listener.registryUrl != crw.registry.GetUrl() || len(listener.urls) != len(urls) {
		t.Errorf("notify not correct! listener:%+v\n", listener)
	}
	fmt.Printf("srw:%+v, urls: %v\n", crw.notifyListener, urls)

	if len(urls) != 5 {
		t.Errorf("notify urls size not correct! listener:%+v\n", listener)
	}

	// check urls
	hasrule := false
	for _, u := range urls {
		if u.Protocol == RULE_PROTOCOL {
			hasrule = true
			continue
		}
		if !strings.HasPrefix(u.Host, "10.73.1") {
			t.Errorf("notify urls host correct! url:%+v\n", u)
		}
	}
	if !hasrule {
		t.Errorf("notify urls not has rule url! urls:%+v\n", urls)
	}

}

func TestProcessCommand(t *testing.T) {
	crw := getDefalultCommandWarper()
	cmds := make([]string, 0, 10)
	cmds = append(cmds, buildCmd(1, CMD_Traffic_Control, "*", "\"group0:3\",\"group1:5\"", ""))
	cmds = append(cmds, buildCmd(1, CMD_Degrade, "com.weibo.test.TestService", "", ""))
	cl := buildCmdList(cmds)
	// process service cmd
	processServiceCmd(crw, cl, t)

	//process agent cmd, agent cmd will over service cmd
	// & process with router
	cmds = make([]string, 0, 10)
	cmds = append(cmds, buildCmd(1, CMD_Traffic_Control, "*", "\"group0:3\",\"group1:5\"", "\" * to 10.75.*\", \"10.108.* to 10.75.1.* \""))
	cmds = append(cmds, buildCmd(1, CMD_Degrade, "com.weibo.test.TestService", "", ""))
	cl = buildCmdList(cmds)
	notify := crw.processCommand(AGENT_CMD, cl)
	if crw.agentCommandInfo != cl {
		t.Errorf("agentCommandInfo not correct! real:%s, expect:%s\n", crw.agentCommandInfo, cl)
	}
	if len(crw.tcCommand.RouteRules) != 2 {
		t.Errorf("tc command is not correct! tc command:%+v\n", crw.tcCommand)
	}

	//repeat command
	notify = crw.processCommand(AGENT_CMD, cl)
	if notify {
		t.Errorf("should not notify with same command! crw:%+v\n", crw)
	}

	//process degrade command
	crw.cluster.GetUrl().Path = "com.weibo.test.TestService"
	crw.agentCommandInfo = ""
	notify = crw.processCommand(AGENT_CMD, cl)
	if crw.degradeCommand == nil || crw.cluster.IsAvaiable() {
		t.Errorf("degrade command not enable! crw:%+v\n", crw)
	}

	// process abnormal command
	crw.serviceCommandInfo = ""
	notify = crw.processCommand(AGENT_CMD, "kljsdfoie")
	if !notify || crw.tcCommand != nil || crw.degradeCommand != nil {
		t.Errorf("abnormal command process not correct! notify: %t, crw:%+v\n", notify, crw)
	}
	fmt.Printf("notify:%t, crw:%+v\n", notify, crw)
}

func processServiceCmd(crw *CommandRegistryWarper, cl string, t *testing.T) {
	notify := crw.processCommand(SERVICE_CMD, cl)
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
	url := &motan.Url{Path: "com.weibo.test.TestService"}
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

func newRouter(rules ...string) []string {
	router := make([]string, 0, 20)
	for _, r := range rules {
		router = append(router, r)
	}
	return router
}

func buildUrls(group string) []*motan.Url {
	urls := make([]*motan.Url, 0, 20)
	urls = append(urls, &motan.Url{Host: "10.75.1.3", Group: group})
	urls = append(urls, &motan.Url{Host: "10.75.1.5", Group: group})
	urls = append(urls, &motan.Url{Host: "10.75.2.3", Group: group})
	urls = append(urls, &motan.Url{Host: "10.75.3.5", Group: group})
	urls = append(urls, &motan.Url{Host: "10.73.1.3", Group: group})
	urls = append(urls, &motan.Url{Host: "10.73.1.5", Group: group})
	urls = append(urls, &motan.Url{Host: "10.77.1.3", Group: group})
	urls = append(urls, &motan.Url{Host: "10.77.1.5", Group: group})

	return urls
}

func checksize(realsize int, expectsize int, t *testing.T) {
	if realsize != expectsize {
		t.Errorf("test router check size fail. real:%d, exp:%d\n", realsize, expectsize)
	}
}

func checkHost(urls []*motan.Url, f func(host string) bool, t *testing.T) {
	for _, u := range urls {
		if f(u.Host) {
			t.Errorf("test fail in prefix match pattern. url:%+v\n", u)
		}
	}
}

func getDefalultCommandWarper() *CommandRegistryWarper {
	cluster := initCluster()
	cluster.InitCluster()
	registry := cluster.extFactory.GetRegistry(RegistryUrl)
	return GetCommandRegistryWarper(cluster, registry).(*CommandRegistryWarper)
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
	registryUrl *motan.Url
	urls        []*motan.Url
}

func (m *MockListener) Notify(registryUrl *motan.Url, urls []*motan.Url) {
	m.registryUrl = registryUrl
	m.urls = urls
}

func (m *MockListener) GetIdentity() string {
	return "mocklistener"
}
