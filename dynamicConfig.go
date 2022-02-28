package motan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	URL "net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	vlog "github.com/weibocom/motan-go/log"

	"github.com/weibocom/motan-go/core"
)

const dynamicConfigRegistrySnapshot = "registry.snap"
const dynamicConfigFilterPrefix = "af_"

type DynamicConfigurer struct {
	runtimePath      string
	registrySnapshot string
	registerNodes    map[string]*core.URL
	subscribeNodes   map[string]*core.URL
	regLock          sync.Mutex
	subLock          sync.Mutex
	saveLock         sync.Mutex
	agent            *Agent
}

type registrySnapInfoStorage struct {
	RegisterNodes  []*core.URL `json:"register_nodes"`
	SubscribeNodes []*core.URL `json:"subscribe_nodes"`
}

func NewDynamicConfigurer(agent *Agent) *DynamicConfigurer {
	configurer := &DynamicConfigurer{
		registrySnapshot: agent.runtimedir + string(filepath.Separator) + dynamicConfigRegistrySnapshot,
		agent:            agent,
		registerNodes:    make(map[string]*core.URL),
		subscribeNodes:   make(map[string]*core.URL),
	}
	configurer.initialize()
	return configurer
}

func (c *DynamicConfigurer) initialize() {
	if c.agent.recover {
		c.doRecover()
	}
}

func (c *DynamicConfigurer) doRecover() error {
	bytes, err := ioutil.ReadFile(c.registrySnapshot)
	if err != nil {
		vlog.Warningln("Read configuration snapshot file error: " + err.Error())
		return err
	}
	registerSnapInfo := new(registrySnapInfoStorage)
	err = json.Unmarshal(bytes, registerSnapInfo)
	if err != nil {
		vlog.Errorln("Parse snapshot string error: " + err.Error())
		return err
	}
	// recover just redo register and subscribe
	for _, node := range registerSnapInfo.RegisterNodes {
		vlog.Infof("Recover register node: %v", node)
		c.doRegister(node)
	}

	for _, node := range registerSnapInfo.SubscribeNodes {
		vlog.Infof("Recover subscribe node: %v", node)
		c.doSubscribe(node)
	}
	return nil
}

func (c *DynamicConfigurer) Register(url *core.URL) error {
	err := c.doRegister(url)
	if err != nil {
		return err
	}
	c.saveSnapshot()
	return nil
}

func (c *DynamicConfigurer) doRegister(url *core.URL) error {
	c.regLock.Lock()
	defer c.regLock.Unlock()
	if _, ok := c.registerNodes[url.GetIdentity()]; ok {
		return nil
	}
	c.agent.ExportService(url)
	c.registerNodes[url.GetIdentity()] = url
	return nil
}

func (c *DynamicConfigurer) Unregister(url *core.URL) error {
	err := c.doUnregister(url)
	if err != nil {
		return err
	}
	c.saveSnapshot()
	return nil
}

func (c *DynamicConfigurer) doUnregister(url *core.URL) error {
	c.regLock.Lock()
	defer c.regLock.Unlock()

	if _, ok := c.registerNodes[url.GetIdentity()]; ok {
		c.agent.UnexportService(url)
		delete(c.registerNodes, url.GetIdentity())
	}
	return nil
}

func (c *DynamicConfigurer) Subscribe(url *core.URL) error {
	err := c.doSubscribe(url)
	if err != nil {
		return err
	}
	c.saveSnapshot()
	return nil
}

func (c *DynamicConfigurer) doSubscribe(url *core.URL) error {
	c.subLock.Lock()
	defer c.subLock.Unlock()
	if _, ok := c.subscribeNodes[url.GetIdentity()]; ok {
		return nil
	}
	c.subscribeNodes[url.GetIdentity()] = url
	c.agent.SubscribeService(url)
	return nil
}

func (c *DynamicConfigurer) saveSnapshot() {
	c.saveLock.Lock()
	defer c.saveLock.Unlock()

	bytes, err := json.Marshal(c.getRegistryInfo())
	if err != nil {
		vlog.Errorln("Convert registry information to json error: " + err.Error())
		return
	}
	err = ioutil.WriteFile(c.registrySnapshot, bytes, 0644)
	if err != nil {
		vlog.Errorln("Write registry snapshot file error: " + err.Error())
		return
	}
}

func (c *DynamicConfigurer) getRegistryInfo() *registrySnapInfoStorage {
	registrySnapInfo := registrySnapInfoStorage{}

	c.regLock.Lock()
	defer c.regLock.Unlock()
	registerNodes := make([]*core.URL, 0, len(c.registerNodes))
	for _, node := range c.registerNodes {
		registerNodes = append(registerNodes, node.Copy())
	}

	c.subLock.Lock()
	defer c.subLock.Unlock()
	subscribeNodes := make([]*core.URL, 0, len(c.subscribeNodes))
	for _, node := range c.subscribeNodes {
		subscribeNodes = append(subscribeNodes, node.Copy())
	}

	registrySnapInfo.RegisterNodes = registerNodes
	registrySnapInfo.SubscribeNodes = subscribeNodes
	return &registrySnapInfo
}

type DynamicConfigurerHandler struct {
	agent *Agent
}

func (h *DynamicConfigurerHandler) SetAgent(agent *Agent) {
	h.agent = agent
}

func (h *DynamicConfigurerHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "application/json;charset=utf-8")
	switch req.RequestURI {
	case "/registry/register":
		h.register(res, req)
	case "/registry/unregister":
		h.unregister(res, req)
	case "/registry/subscribe":
		h.subscribe(res, req)
	case "/registry/list":
		h.list(res, req)
	case "/registry/info":
		h.info(res, req)
	default:
		res.WriteHeader(http.StatusNotFound)
	}
}

func (h *DynamicConfigurerHandler) getURL(req *http.Request) (*core.URL, error) {
	bytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	url := new(core.URL)
	err = json.Unmarshal(bytes, url)
	if err != nil {
		return nil, err
	}
	if url.Group == "" {
		url.Group = url.GetParam(core.GroupKey, "")
		delete(url.Parameters, core.GroupKey)
	}
	registryID := ""
	// such as 'direct://localhost:9981'
	proxyRegistry := url.GetParam(core.ProxyRegistryKey, "")
	if proxyRegistry != "" {
		for id, url := range h.agent.Context.RegistryURLs {
			if fmt.Sprintf("%s://%s:%d", url.Protocol, url.Host, url.Port) == proxyRegistry {
				registryID = id
				vlog.Infof("find proxy registry by ProxyRegistryKey, id:%s", registryID)
				break
			}
		}
	}
	if registryID == "" {
		// find proper registry
		proxyRegistryUrlString := url.GetParam(core.ProxyRegistryUrlString, "")
		if proxyRegistryUrlString != "" {
			unescapeString, err := URL.QueryUnescape(proxyRegistryUrlString)
			if err == nil {
				ProxyRegistryUrl := core.FromExtInfo(unescapeString)
				if ProxyRegistryUrl != nil {
					for id, url := range h.agent.Context.RegistryURLs {
						if registryEquals(ProxyRegistryUrl, url) {
							registryID = id
							vlog.Infof("find proxy registry by registryEquals, id:%s", registryID)
							break
						}
					}
					// TODO 动态添加 registry url， 需要考虑并发 问题
				}
			}
		}
	}
	if registryID == "" {
		return nil, errors.New("registry not found")
	}
	url.PutParam(core.RegistryKey, registryID)

	filters := ""
	agentFilter := make([]string, 0, 8)
	for _, f := range core.TrimSplit(url.GetParam(core.FilterKey, ""), ",") {
		if !strings.HasPrefix(f, dynamicConfigFilterPrefix) {
			continue
		}
		if f == dynamicConfigFilterPrefix {
			continue
		}
		agentFilter = append(agentFilter, strings.TrimSpace(f[len(dynamicConfigFilterPrefix):]))
	}
	if len(agentFilter) > 0 {
		filters = strings.Join(agentFilter, ",")
	}
	if filters == "" {
		filters = h.agent.Context.AgentURL.GetParam(core.FilterKey, "")
	}
	if filters != "" {
		url.PutParam(core.FilterKey, filters)
	}
	return url, nil
}

func (h *DynamicConfigurerHandler) register(res http.ResponseWriter, req *http.Request) {
	url, err := h.getURL(req)
	if err != nil {
		writeHandlerResponse(res, http.StatusBadRequest, err.Error(), nil)
		return
	}
	url.PutParam(core.ProxyKey, url.Protocol+":"+url.GetPortStr())
	url.PutParam(core.ExportKey, url.Protocol+":"+strconv.Itoa(h.agent.eport))
	h.agent.initProxyServiceURL(url)
	err = h.agent.configurer.Register(url)
	if err != nil {
		writeHandlerResponse(res, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeHandlerResponse(res, http.StatusOK, "ok", nil)
}

func (h *DynamicConfigurerHandler) unregister(res http.ResponseWriter, req *http.Request) {
	url, err := h.getURL(req)
	if err != nil {
		writeHandlerResponse(res, http.StatusBadRequest, err.Error(), nil)
		return
	}
	url.PutParam(core.ProxyKey, url.Protocol+":"+url.GetPortStr())
	url.PutParam(core.ExportKey, url.Protocol+":"+strconv.Itoa(h.agent.eport))
	h.agent.initProxyServiceURL(url)
	err = h.agent.configurer.Unregister(url)
	if err != nil {
		writeHandlerResponse(res, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeHandlerResponse(res, http.StatusOK, "ok", nil)
}

func (h *DynamicConfigurerHandler) subscribe(res http.ResponseWriter, req *http.Request) {
	url, err := h.getURL(req)
	if err != nil {
		writeHandlerResponse(res, http.StatusBadRequest, err.Error(), nil)
		return
	}
	url.Host = ""
	url.Port = 0
	err = h.agent.configurer.Subscribe(url)
	if err != nil {
		writeHandlerResponse(res, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeHandlerResponse(res, http.StatusOK, "ok", nil)
}

func (h *DynamicConfigurerHandler) list(res http.ResponseWriter, req *http.Request) {
	writeHandlerResponse(res, http.StatusOK, "ok", h.agent.configurer.getRegistryInfo())
}

func (h *DynamicConfigurerHandler) info(res http.ResponseWriter, req *http.Request) {
	writeHandlerResponse(res, http.StatusOK, "ok", struct {
		MeshPort int `json:"mesh_port"`
	}{MeshPort: h.agent.port})
}

func writeHandlerResponse(res http.ResponseWriter, code int, message string, body interface{}) {
	res.WriteHeader(code)
	m := make(map[string]interface{})
	m["code"] = code
	if message != "" {
		m["message"] = message
	}
	if body != nil {
		m["body"] = body
	}
	bytes, _ := json.Marshal(m)
	res.Write(bytes)
}

func registryEquals(reg1 *core.URL, reg2 *core.URL) bool {
	if reg1 != nil && reg2 != nil {
		return reg1.Protocol == reg2.Protocol && reg1.Host == reg2.Host && reg1.Port == reg2.Port
	}
	return false
}
