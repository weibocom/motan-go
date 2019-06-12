package core

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"

	cfg "github.com/weibocom/motan-go/config"
	"github.com/weibocom/motan-go/log"
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

	urlFields = map[string]bool{"protocol": true, "host": true, "port": true, "path": true, "group": true}
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

func (c *Context) Initialize() {
	if c.pool == "" {
		c.pool = *Pool
	}
	if c.application == "" {
		c.application = *Application
	}
	if c.ConfigFile == "" {
		c.ConfigFile = *CfgFile
	}

	c.RegistryURLs = make(map[string]*URL)
	c.RefersURLs = make(map[string]*URL)
	c.BasicReferURLs = make(map[string]*URL)
	c.ServiceURLs = make(map[string]*URL)
	c.BasicServiceURLs = make(map[string]*URL)
	var config *cfg.Config
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
	c.parseRegistrys()
	c.parseBasicRefers()
	c.parseRefers()
	c.parserBasicServices()
	c.parseServices()
	c.parseHostURL()
	c.parseHTTPClients()
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
		newURLs[key] = newURL
	}
	return newURLs
}

func (c *Context) parseRefers() {
	c.RefersURLs = c.basicConfToURLs(refersSection)
}

func (c *Context) parseBasicRefers() {
	c.BasicReferURLs = c.confToURLs(basicRefersSection)
}

func (c *Context) parseServices() {
	c.ServiceURLs = c.basicConfToURLs(servicesSection)
}

func (c *Context) parserBasicServices() {
	c.BasicServiceURLs = c.confToURLs(basicServicesSection)
}

func (c *Context) parseHTTPClients() {
	c.HTTPClientURLs = c.basicConfToURLs(httpClientSection)
}
