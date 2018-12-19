package core

import (
	"errors"
	"flag"
	"fmt"
	"reflect"
	"strings"

	cfg "github.com/weibocom/motan-go/config"
	"github.com/weibocom/motan-go/log"
)

const (
	// all default sections
	registrysSection     = "motan-registry"
	basicRefersSection   = "motan-basicRefer"
	refersSection        = "motan-refer"
	basicServicesSection = "motan-basicService"
	servicesSection      = "motan-service"
	agentSection         = "motan-agent"
	clientSection        = "motan-client"
	httpClientSection    = "http-client"
	serverSection        = "motan-server"
	importSection        = "import-refer"
	importHTTPLocation   = "import-http-location"
	dynamicSection       = "dynamic-param"
	SwitcherSection      = "switcher"

	// URLConfKey is config id
	// config Keys
	URLConfKey      = "conf-id"
	basicReferKey   = "basicRefer"
	basicServiceKey = "basicService"

	// default config file and path
	configFile = "./motan.yaml"
	configPath = "./"
	fileSuffix = ".yaml"

	// const for application pool
	basicConfig       = "basic.yaml"
	servicePath       = "services/"
	applicationPath   = "applications/"
	poolPath          = "pools/"
	httpServicePath   = "http/service/"
	httpLocationPath  = "http/location/"
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
}

var (
	urlFields = map[string]bool{"protocol": true, "host": true, "port": true, "path": true, "group": true}
)

// all env flag in motan-go
var (
	Port         = flag.Int("port", 0, "agent listen port")
	Eport        = flag.Int("eport", 0, "agent export service port when as a reverse proxy server")
	Hport        = flag.Int("hport", 0, "http forward proxy server port")
	Mport        = flag.Int("mport", 0, "agent manage port")
	Pidfile      = flag.String("pidfile", "", "agent manage port")
	CfgFile      = flag.String("c", "", "motan run conf")
	LocalIP      = flag.String("localIP", "", "local ip for motan register")
	IDC          = flag.String("idc", "", "the idc info for agent or client.")
	DynamicConfs = flag.String("dynamicConf", "", "dynamic config file for config placeholder")
	Pool         = flag.String("pool", "", "application pool config. like 'application-idc-level'")
	Application  = flag.String("application", "", "assist for application pool config.")
	Recover      = flag.Bool("recover", false, "recover from accidental exit")
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

func (c *Context) Initialize() {
	c.RegistryURLs = make(map[string]*URL)
	c.RefersURLs = make(map[string]*URL)
	c.BasicReferURLs = make(map[string]*URL)
	c.ServiceURLs = make(map[string]*URL)
	c.BasicServiceURLs = make(map[string]*URL)
	if c.ConfigFile == "" { // use flag as default config file name
		c.ConfigFile = *CfgFile
	}
	var cfgRs *cfg.Config
	var err error
	if *Pool != "" { // parse application pool configs
		if c.ConfigFile == "" {
			c.ConfigFile = configPath
		}
		if !strings.HasSuffix(c.ConfigFile, "/") {
			c.ConfigFile = c.ConfigFile + "/"
		}
		if cfgRs, err = parsePool(c.ConfigFile, *Pool); err != nil {
			fmt.Printf("parse configs fail. err:%s\n", err.Error())
			return
		}
	} else { // parse single config file and dynamic file
		if c.ConfigFile == "" {
			c.ConfigFile = configFile
		}
		if cfgRs, err = cfg.NewConfigFromFile(c.ConfigFile); err != nil {
			fmt.Printf("parse config fail. err:%s\n", err.Error())
			return
		}
		var dynamicFile string
		if *DynamicConfs != "" {
			dynamicFile = *DynamicConfs
		}
		if dynamicFile == "" && *IDC != "" {
			suffix := *IDC + ".yaml"
			idx := strings.LastIndex(c.ConfigFile, "/")
			if idx > -1 {
				dynamicFile = c.ConfigFile[0:idx+1] + suffix
			} else {
				dynamicFile = suffix
			}
		}
		if dynamicFile != "" {
			if dc, err := cfg.NewConfigFromFile(dynamicFile); err != nil {
				vlog.Warningf("load dynamic config file failed: %s", err.Error())
			} else {
				dconfs := make(map[string]interface{})
				for k, v := range dc.GetOriginMap() {
					if _, ok := v.(map[interface{}]interface{}); ok { // v must be a single value
						continue
					}
					if ks, ok := k.(string); ok {
						dconfs[ks] = v
					}
				}
				cfgRs.ReplacePlaceHolder(dconfs)
			}
		}
	}

	c.Config = cfgRs
	c.parseRegistrys()
	c.parseBasicRefers()
	c.parseRefers()
	c.parserBasicServices()
	c.parseServices()
	c.parseHostURL()
	c.parseHTTPClients()
}

func importCfgIgnoreError(finalConfig *cfg.Config, parsingConfig *cfg.Config, section string, root string, includeDir string, parsedHttpLocation map[string]bool) {
	is, err := parsingConfig.DIY(section)
	if err != nil || is == nil {
		return
	}
	isHttpLocation := section == importHTTPLocation
	if li, ok := is.([]interface{}); ok {
		for _, r := range li {
			tempCfg, err := cfg.NewConfigFromFile(root + includeDir + r.(string) + fileSuffix)
			if isHttpLocation {
				if parsedHttpLocation[r.(string)] {
					continue
				} else {
					parsedHttpLocation[r.(string)] = true
				}
			}
			if err != nil || tempCfg == nil {
				continue
			}
			importCfgIgnoreError(finalConfig, tempCfg, importHTTPLocation, root, httpLocationPath, parsedHttpLocation)
			finalConfig.Merge(tempCfg)
		}
	}
}

// pool config priority ： pool > application > service > http > basic
func parsePool(path string, pool string) (*cfg.Config, error) {
	c := cfg.NewConfig()
	var tempCfg *cfg.Config
	var err error

	// basic config
	tempCfg, err = cfg.NewConfigFromFile(path + basicConfig)
	if err == nil && tempCfg != nil {
		c.Merge(tempCfg)
	}

	parsedHttpLocation := make(map[string]bool)

	// application
	poolPart := strings.Split(pool, poolNameSeparator)
	var appConfig *cfg.Config
	applicationName := *Application
	if applicationName == "" && len(poolPart) > 0 { // the first part be the application name
		applicationName = poolPart[0]
	}
	// http service
	httpService := path + httpServicePath + applicationName + fileSuffix
	httpConfig, err := cfg.NewConfigFromFile(httpService)
	if err == nil && httpConfig != nil {
		importCfgIgnoreError(c, httpConfig, importHTTPLocation, path, httpLocationPath, parsedHttpLocation)
		c.Merge(httpConfig)
	}

	application := path + applicationPath + applicationName + fileSuffix
	if application != "" {
		appConfig, err = cfg.NewConfigFromFile(application)
		if err == nil && appConfig != nil {
			importCfgIgnoreError(c, appConfig, importSection, path, servicePath, parsedHttpLocation)
			c.Merge(appConfig) // application config > service default config
		}
	}

	// pool
	if len(poolPart) > 0 { // step loading
		base := ""
		for _, v := range poolPart {
			base = base + v
			tempCfg, err = cfg.NewConfigFromFile(path + poolPath + base + fileSuffix)
			if err == nil && tempCfg != nil {
				c.Merge(tempCfg)
			}
			base = base + poolNameSeparator
		}
	}

	// replace dynamic param
	dp, err := c.GetSection(dynamicSection)
	if err == nil && len(dp) > 0 {
		ph := make(map[string]interface{}, len(dp))
		for k, v := range dp {
			if sk, ok := k.(string); ok {
				ph[sk] = v
			}
		}
		c.ReplacePlaceHolder(ph)
	}

	if len(c.GetOriginMap()) == 0 {
		return nil, errors.New("parse " + pool + " pool fail.")
	}
	return c, nil

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
				fmt.Printf("load %s configuration success. url: %s\n", basicConfName, newURL.GetIdentity())
			} else {
				newURL = url
				fmt.Printf("can not found %s: %s. url: %s\n", basicKey, basicConfName, url.GetIdentity())
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
