package core

import (
	"errors"
	"flag"
	"fmt"
	cfg "github.com/weibocom/motan-go/config"
	"github.com/weibocom/motan-go/log"
	"os"
	"reflect"
	"strings"
)

const (
	// all default sections
	registrysSection          = "motan-registry"
	basicRefersSection        = "motan-basicRefer"
	refersSection             = "motan-refer"
	basicServicesSection      = "motan-basicService"
	servicesSection           = "motan-service"
	agentSection              = "motan-agent"
	clientSection             = "motan-client"
	httpClientSection         = "http-client"
	serverSection             = "motan-server"
	importSection             = "import-refer"
	importHTTPLocationSection = "import-http-location"
	dynamicSection            = "dynamic-param"
	SwitcherSection           = "switcher"

	// URLConfKey is config id
	// config Keys
	URLConfKey      = "conf-id"
	basicReferKey   = "basicRefer"
	basicServiceKey = "basicService"

	// const for application pool
	basicConfig       = "basic.yaml"
	servicePath       = "services/"
	applicationPath   = "applications/"
	poolPath          = "pools/"
	httpServicePath   = "http/service/"
	httpLocationPath  = "http/location/"
	httpPoolPath      = "http/pools/"
	poolNameSeparator = "-"
)

// Context for agent, client, server. context is created depends on  config file
type Context struct {
	ConfigFile       string
	Config           *cfg.Config
	RegistryURLs     map[string]*URL
	RefersURLs       map[string]*URL
	HTTPClientURLs   map[string]*URL
	BasicReferURLs   map[string]*URL
	ServiceURLs      map[string]*URL
	BasicServiceURLs map[string]*URL
	AgentURL         *URL
	ClientURL        *URL
	ServerURL        *URL

	application string
	pool        string
}

var (
	// default config file and path
	defaultConfigFile = "./motan.yaml"
	defaultConfigPath = "./"
	defaultFileSuffix = ".yaml"

	urlFields  = map[string]bool{"protocol": true, "host": true, "port": true, "path": true, "group": true}
	extFilters = make(map[string]bool)
)

// all env flag in motan-go
var (
	Port        = flag.Int("port", 0, "agent listen port")
	Eport       = flag.Int("eport", 0, "agent export service port when as a reverse proxy server")
	Hport       = flag.Int("hport", 0, "http forward proxy server port")
	Mport       = flag.Int("mport", 0, "agent manage port")
	Pidfile     = flag.String("pidfile", "", "agent manage port")
	CfgFile     = flag.String("c", "", "motan run conf")
	LocalIP     = flag.String("localIP", "", "local ip for motan register")
	IDC         = flag.String("idc", "", "the idc info for agent or client.")
	Pool        = flag.String("pool", "", "application pool config. like 'application-idc-level'")
	Application = flag.String("application", "", "assist for application pool config.")
	Recover     = flag.Bool("recover", false, "recover from accidental exit")
)

func AddRelevantFilter(filterStr string) {
	k := strings.TrimSpace(filterStr)
	if k != "" {
		extFilters[k] = true
	}
}

func GetRelevantFilters() map[string]bool {
	return extFilters
}

func (c *Context) confToURLs(section string) map[string]*URL {
	urls := map[string]*URL{}
	sectionConf, _ := c.Config.GetSection(section)
	for key, info := range sectionConf {
		url := confToURL(info.(map[interface{}]interface{}))
		url.Parameters[URLConfKey] = InterfaceToString(key)
		urls[key.(string)] = url
	}
	return urls
}

func confToURL(urlInfo map[interface{}]interface{}) *URL {
	urlParams := make(map[string]string)
	url := &URL{Parameters: urlParams}
	for sk, sinfo := range urlInfo {
		if _, ok := urlFields[sk.(string)]; ok {
			if reflect.TypeOf(sinfo) == nil {
				if sk == "port" {
					sinfo = "0"
				} else {
					sinfo = ""
				}
			}
			switch sk.(string) {
			case "protocol":
				url.Protocol = sinfo.(string)
			case "host":
				url.Host = sinfo.(string)
			case "port":
				url.Port = sinfo.(int)
			case "path":
				url.Path = sinfo.(string)
			case "group":
				url.Group = sinfo.(string)
			}
		} else {
			urlParams[sk.(string)] = InterfaceToString(sinfo)
		}
	}
	url.Parameters = urlParams
	return url
}

// importCfgIgnoreError import the config with specified section
func importCfgIgnoreError(finalConfig *cfg.Config, parsingConfig *cfg.Config, section string, root string, includeDir string, parsedHTTPLocation map[string]bool) {
	is, err := parsingConfig.DIY(section)
	if err != nil || is == nil {
		return
	}
	isHTTPLocation := section == importHTTPLocationSection
	if li, ok := is.([]interface{}); ok {
		for _, r := range li {
			if isHTTPLocation {
				if parsedHTTPLocation[r.(string)] {
					continue
				} else {
					parsedHTTPLocation[r.(string)] = true
				}
			}
			tempCfg, err := cfg.NewConfigFromFile(root + includeDir + r.(string) + defaultFileSuffix)
			if err != nil || tempCfg == nil {
				continue
			}
			importCfgIgnoreError(finalConfig, tempCfg, importHTTPLocationSection, root, httpLocationPath, parsedHTTPLocation)
			finalConfig.Merge(tempCfg)
		}
	}
}

func NewContext(configFile string, application string, pool string) *Context {
	context := Context{
		ConfigFile:  configFile,
		application: application,
		pool:        pool,
	}
	context.Initialize()
	return &context
}

func NewContextFromConfig(conf *cfg.Config, application string, pool string) *Context {
	context := Context{
		Config:      conf,
		application: application,
		pool:        pool,
	}
	context.Initialize()
	return &context
}

func (c *Context) Initialize() {
	if c.pool == "" {
		c.pool = *Pool
	}
	if c.application == "" {
		c.application = *Application
	}

	c.RegistryURLs = make(map[string]*URL)
	c.RefersURLs = make(map[string]*URL)
	c.BasicReferURLs = make(map[string]*URL)
	c.ServiceURLs = make(map[string]*URL)
	c.BasicServiceURLs = make(map[string]*URL)

	if c.Config == nil {
		// init config from config file
		var config *cfg.Config

		if c.ConfigFile == "" {
			c.ConfigFile = *CfgFile
		}
		if c.ConfigFile == "" {
			if c.pool != "" {
				c.ConfigFile = defaultConfigPath
			} else {
				c.ConfigFile = defaultConfigFile
			}
		}

		configFileInfo, err := os.Stat(c.ConfigFile)
		if err != nil {
			vlog.Errorf("get configuration [%s] information failed. err:%s\n", c.ConfigFile, err.Error())
			return
		}
		if configFileInfo.IsDir() {
			// if configuration file is a directory we treat is as pool
			if c.application == "" && c.pool == "" {
				vlog.Errorf("when configuration [%s] is a directory you should specify the application or pool", c.ConfigFile)
				return
			}
			if !strings.HasSuffix(c.ConfigFile, "/") {
				c.ConfigFile = c.ConfigFile + "/"
			}
			config, err = c.parsePoolConfiguration()
		} else {
			config, err = c.parseSingleConfiguration()
		}

		if err != nil {
			vlog.Errorf("parse configs fail. err:%s", err.Error())
			return
		}
		c.Config = config
	}

	c.parseRegistrys()
	c.parseHostURL()
	c.parseBasicRefers()
	c.parseRefers()
	c.parserBasicServices()
	c.parseServices()
	c.parseHTTPClients()
	initSwitcher(c)
}

func (c *Context) parseSingleConfiguration() (*cfg.Config, error) {
	var config *cfg.Config
	config, err := cfg.NewConfigFromFile(c.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("parse configuration [%s] failed. err:%s", c.ConfigFile, err.Error())
	}
	dp, err := config.GetSection(dynamicSection)
	if err == nil && len(dp) > 0 {
		config.ReplacePlaceHolder(c.getDynamicParameters(dp))
	}
	return config, nil
}

// pool config priority ： pool > application > service > http > basic
func (c *Context) parsePoolConfiguration() (*cfg.Config, error) {
	config := cfg.NewConfig()
	path := c.ConfigFile
	pool := c.pool
	// basic
	basicCfg, err := cfg.NewConfigFromFile(c.ConfigFile + basicConfig)
	if err == nil && basicCfg != nil {
		config.Merge(basicCfg)
	}

	parsedHTTPLocation := make(map[string]bool)

	poolPart := strings.Split(pool, poolNameSeparator)
	applicationName := c.application
	if applicationName == "" && len(poolPart) > 0 { // the first part be the application name
		applicationName = poolPart[0]
	}
	// http service
	httpService := path + httpServicePath + applicationName + defaultFileSuffix
	httpServiceCfg, err := cfg.NewConfigFromFile(httpService)
	if err == nil && httpServiceCfg != nil {
		importCfgIgnoreError(config, httpServiceCfg, importHTTPLocationSection, path, httpLocationPath, parsedHTTPLocation)
		config.Merge(httpServiceCfg)
	}

	// http pool
	httpPool := path + httpPoolPath + pool + defaultFileSuffix
	httpPoolCfg, err := cfg.NewConfigFromFile(httpPool)
	if err == nil && httpPoolCfg != nil {
		importCfgIgnoreError(config, httpPoolCfg, importHTTPLocationSection, path, httpLocationPath, parsedHTTPLocation)
		config.Merge(httpPoolCfg)
	}

	// application
	application := path + applicationPath + applicationName + defaultFileSuffix
	if application != "" {
		appConfig, err := cfg.NewConfigFromFile(application)
		if err == nil && appConfig != nil {
			importCfgIgnoreError(config, appConfig, importSection, path, servicePath, parsedHTTPLocation)
			config.Merge(appConfig) // application config > service default config
		}
	}

	// pool
	if len(poolPart) > 0 { // step loading
		base := ""
		for _, v := range poolPart {
			base = base + v
			poolCfg, err := cfg.NewConfigFromFile(path + poolPath + base + defaultFileSuffix)
			if err == nil && poolCfg != nil {
				config.Merge(poolCfg)
			}
			base = base + poolNameSeparator
		}
	}

	// replace dynamic param
	dp, err := config.GetSection(dynamicSection)
	if err == nil && len(dp) > 0 {
		config.ReplacePlaceHolder(c.getDynamicParameters(dp))
	}

	if len(config.GetOriginMap()) == 0 {
		return nil, errors.New("parse " + pool + " pool fail.")
	}
	return config, nil
}

func (c *Context) getDynamicParameters(dp map[interface{}]interface{}) map[string]interface{} {
	ph := make(map[string]interface{}, len(dp))
	for k, v := range dp {
		if sk, ok := k.(string); ok {
			switch v.(type) {
			case map[interface{}]interface{}:
				mv := v.(map[interface{}]interface{})
				ph[sk] = mv[c.pool]
				if ph[sk] == nil {
					ph[sk] = mv["default"]
				}
			default:
				ph[sk] = v
			}
		}
	}
	return ph
}

// parse host url including agenturl, clienturl, serverurl
func (c *Context) parseHostURL() {
	agentInfo, _ := c.Config.GetSection(agentSection)
	c.AgentURL = confToURL(agentInfo)
	clientInfo, _ := c.Config.GetSection(clientSection)
	c.ClientURL = confToURL(clientInfo)
	serverInfo, _ := c.Config.GetSection(serverSection)
	c.ServerURL = confToURL(serverInfo)
}

func (c *Context) parseRegistrys() {
	c.RegistryURLs = c.confToURLs(registrysSection)
}

func (c *Context) basicConfToURLs(section string) map[string]*URL {
	newURLs := map[string]*URL{}
	urls := c.confToURLs(section)
	var basicURLs map[string]*URL
	var basicKey string
	if section == servicesSection {
		basicURLs = c.BasicServiceURLs
		basicKey = basicServiceKey
	} else if section == refersSection || section == httpClientSection {
		basicURLs = c.BasicReferURLs
		basicKey = basicReferKey
	}
	for key, url := range urls {
		var newURL *URL
		if basicConfName := url.GetParam(basicKey, ""); basicConfName != "" {
			if basicURL, ok := basicURLs[basicConfName]; ok {
				newURL = basicURL.Copy()
				if url.Protocol != "" {
					newURL.Protocol = url.Protocol
				}
				if url.Host != "" {
					newURL.Host = url.Host
				}
				if url.Port != 0 {
					newURL.Port = url.Port
				}
				if url.Group != "" {
					newURL.Group = url.Group
				}
				if url.Path != "" {
					newURL.Path = url.Path
				}
				newURL.MergeParams(url.Parameters)
				vlog.Infof("load %s configuration success. url: %s\n", basicConfName, newURL.GetIdentity())
			} else {
				newURL = url
				vlog.Infof("can not found %s: %s. url: %s\n", basicKey, basicConfName, url.GetIdentity())
			}
		} else {
			newURL = url
		}

		//final filters: defaultFilter + globalFilter + filters + envFilter + relevantFilters
		finalFilters := c.MergeFilterSet(
			c.GetDefaultFilterSet(newURL),
			c.GetGlobalFilterSet(newURL),
			c.GetEnvGlobalFilterSet(),
			GetRelevantFilters(),
			c.GetFilterSet(newURL.GetStringParamsWithDefault(FilterKey, ""), ""),
		)
		if len(finalFilters) > 0 {
			newURL.PutParam(FilterKey, c.FilterSetToStr(finalFilters))
		}
		newURLs[key] = newURL
	}
	return newURLs
}

func (c *Context) FilterSetToStr(f map[string]bool) string {
	var dst []string
	for k := range f {
		dst = append(dst, k)
	}
	return strings.Join(dst, ",")
}

func (c *Context) GetFilterSet(filterStr, disableFilterStr string) (dst map[string]bool) {
	if filterStr == "" {
		return
	}
	dst = map[string]bool{}

	for _, k := range strings.Split(filterStr, ",") {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		dst[k] = true
	}

	for _, k := range strings.Split(disableFilterStr, ",") {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		delete(dst, k)
	}
	return
}

func (c *Context) MergeFilterSet(sets ...map[string]bool) (dst map[string]bool) {
	dst = map[string]bool{}
	for _, set := range sets {
		for k := range set {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			dst[k] = true
		}
	}
	return
}

func (c *Context) GetDefaultFilterSet(newURL *URL) map[string]bool {
	if c.AgentURL == nil {
		return nil
	}
	return c.GetFilterSet(c.AgentURL.GetStringParamsWithDefault(DefaultFilter, ""),
		newURL.GetStringParamsWithDefault(DisableDefaultFilter, ""))
}

func (c *Context) GetGlobalFilterSet(newURL *URL) map[string]bool {
	if c.AgentURL == nil {
		return nil
	}
	return c.GetFilterSet(c.AgentURL.GetStringParamsWithDefault(GlobalFilter, ""),
		newURL.GetStringParamsWithDefault(DisableGlobalFilter, ""))
}

func (c *Context) GetEnvGlobalFilterSet() map[string]bool {
	res := make(map[string]bool)
	if filters := os.Getenv(FilterEnvironmentName); filters != "" {
		for _, k := range strings.Split(filters, ",") {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			res[k] = true
		}
	}
	return res
}

// parseMultipleServiceGroup  add motan-service group support of multiple comma split group name
func (c *Context) parseMultipleServiceGroup(motanServiceMap map[string]*URL) {
	addMotanServiceMap := map[string]*URL{}
	for k, serviceURL := range motanServiceMap {
		//add additional service group from environment
		if v := os.Getenv(GroupEnvironmentName); v != "" {
			if serviceURL.Group == "" {
				serviceURL.Group = v
			} else {
				serviceURL.Group += "," + v
			}
		}
		if !strings.Contains(serviceURL.Group, GroupNameSeparator) {
			continue
		}
		groups := SlicesUnique(TrimSplit(serviceURL.Group, GroupNameSeparator))
		serviceURL.Group = groups[0]
		for idx, g := range groups[1:] {
			key := fmt.Sprintf("%v-%v", k, idx)
			newService := serviceURL.Copy()
			newService.Group = g
			addMotanServiceMap[key] = newService
		}
	}
	for k, v := range addMotanServiceMap {
		motanServiceMap[k] = v
	}
}

func (c *Context) parseRegGroupSuffix(urlMap map[string]*URL) {
	regGroupSuffix := os.Getenv(RegGroupSuffix)
	if regGroupSuffix == "" {
		return
	}
	filterMap := make(map[string]struct{}, len(urlMap))
	for _, url := range urlMap {
		filterMap[url.GetIdentityWithRegistry()] = struct{}{}
	}
	for k, url := range urlMap {
		if strings.HasSuffix(url.Group, regGroupSuffix) {
			continue
		}
		newUrl := url.Copy()
		newUrl.Group += regGroupSuffix
		if _, ok := filterMap[newUrl.GetIdentityWithRegistry()]; ok {
			continue
		}
		filterMap[newUrl.GetIdentityWithRegistry()] = struct{}{}
		urlMap[k] = newUrl
	}
}

func (c *Context) parseSubGroupSuffix(urlMap map[string]*URL) {
	subGroupSuffix := os.Getenv(SubGroupSuffix)
	if subGroupSuffix == "" || c.AgentURL == nil {
		return
	}
	filterMap := make(map[string]struct{}, len(urlMap)*2)
	for _, url := range urlMap {
		filterMap[url.GetIdentity()] = struct{}{}
	}
	for k, url := range urlMap {
		if strings.HasSuffix(url.Group, subGroupSuffix) {
			continue
		}
		groupWithSuffix := url.Group + subGroupSuffix
		newUrl := url.Copy()
		newUrl.Group += subGroupSuffix
		if _, ok := filterMap[newUrl.GetIdentity()]; ok {
			continue
		}
		filterMap[newUrl.GetIdentity()] = struct{}{}
		urlMap["auto_"+k+groupWithSuffix] = newUrl
	}
}

func (c *Context) parseRefers() {
	referUrls := c.basicConfToURLs(refersSection)
	c.parseSubGroupSuffix(referUrls)
	c.RefersURLs = referUrls
}

func (c *Context) parseBasicRefers() {
	c.BasicReferURLs = c.confToURLs(basicRefersSection)
}

func (c *Context) parseServices() {
	urlsMap := c.basicConfToURLs(servicesSection)
	c.parseMultipleServiceGroup(urlsMap)
	c.parseRegGroupSuffix(urlsMap)
	c.ServiceURLs = urlsMap
}

func (c *Context) parserBasicServices() {
	c.BasicServiceURLs = c.confToURLs(basicServicesSection)
}

func (c *Context) parseHTTPClients() {
	c.HTTPClientURLs = c.basicConfToURLs(httpClientSection)
}
