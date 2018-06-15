package core

import (
	"flag"
	"reflect"

	cfg "github.com/weibocom/motan-go/config"
	"github.com/weibocom/motan-go/log"
	"strings"
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
	serverSection        = "motan-server"
	importSection        = "import-refer"
	dynamicSection       = "dynamic-param"

	// URLConfKey add confid to url params
	URLConfKey = "conf-id"

	// default config file and path
	configFile = "./motan.yaml"
	configPath = "./"
	fileSuffix = ".yaml"

	// path in application pool
	basicConfig     = "basic.yaml"
	servicePath     = "services/"
	applicationPath = "applications/"
	poolPath        = "pools/"

	poolNameSaperator = "-"
)

// Context for agent, client, server. context is created depends on  config file
type Context struct {
	ConfigFile       string
	Config           *cfg.Config
	RegistryURLs     map[string]*URL
	RefersURLs       map[string]*URL
	BasicRefers      map[string]*URL
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
	Mport        = flag.Int("mport", 0, "agent manage port")
	Pidfile      = flag.String("pidfile", "", "agent manage port")
	CfgFile      = flag.String("c", "", "motan run conf")
	LocalIP      = flag.String("localIP", "", "local ip for motan register")
	IDC          = flag.String("idc", "", "the idc info for agent or client.")
	DynamicConfs = flag.String("dynamicConf", "", "dynamic config file for config placeholder")
	Pool         = flag.String("pool", "", "application pool config. like 'application-idc-level'")
	Application  = flag.String("application", "", "assist for application pool config.")
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
	c.BasicRefers = make(map[string]*URL)
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
		cfgRs, err = parsePool(c.ConfigFile, *Pool)
		if err != nil {
			vlog.Errorf("parse config fail. err:%s\n", err.Error())
			return
		}
	} else { // parse single config file and dynamic file
		if c.ConfigFile == "" {
			c.ConfigFile = configFile
		}
		cfgRs, err = cfg.NewConfigFromFile(c.ConfigFile)
		if err != nil {
			vlog.Errorf("parse config fail. err:%s\n", err.Error())
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
			dc, _ := cfg.NewConfigFromFile(dynamicFile)
			dconfs := make(map[string]interface{})
			for k, v := range dc.GetOriginMap() {
				if _, ok := v.(map[interface{}]interface{}); ok { // v must be a single value
					continue
				}
				dconfs[k] = v
			}
			cfgRs.ReplacePlaceHolder(dconfs)
		}
	}

	c.Config = cfgRs
	c.parseRegistrys()
	c.parseBasicRefers()
	c.parseRefers()
	c.parserBasicServices()
	c.parseServices()
	c.parseHostURL()
}

// pool config priority ： pool > application > service > basic
func parsePool(path string, pool string) (*cfg.Config, error) {
	c := cfg.NewConfig()
	var tempcfg *cfg.Config
	var err error

	// basic config
	tempcfg, err = cfg.NewConfigFromFile(path + basicConfig)
	if err == nil && tempcfg != nil {
		c.Merge(tempcfg)
	}

	// application
	poolPart := strings.Split(pool, poolNameSaperator)
	var appconfig *cfg.Config
	application := *Application
	if application == "" && len(poolPart) > 0 { // the first part be the application name
		application = poolPart[0]
	}
	application = path + applicationPath + application + fileSuffix
	if application != "" {
		appconfig, err = cfg.NewConfigFromFile(application)
		if err == nil && appconfig != nil {
			// import-refer
			is, err := appconfig.DIY(importSection)
			if err == nil && is != nil {
				if li, ok := is.([]interface{}); ok {
					for _, r := range li {
						tempcfg, err = cfg.NewConfigFromFile(path + servicePath + r.(string) + fileSuffix)
						if err == nil && tempcfg != nil {
							c.Merge(tempcfg)
						}
					}

				}
			}
			c.Merge(appconfig) // application config > service default config
		}
	}

	// pool
	if len(poolPart) > 0 { // step loading
		base := ""
		for _, v := range poolPart {
			base = base + v
			tempcfg, err = cfg.NewConfigFromFile(path + poolPath + base + fileSuffix)
			if err == nil && tempcfg != nil {
				c.Merge(tempcfg)
			}
			base = base + poolNameSaperator
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

func (c *Context) basicConfToURLs(basicsection, section string) map[string]*URL {
	urls := map[string]*URL{}
	Refs := c.confToURLs(section)
	var BasicInfo map[string]*URL
	if section == servicesSection {
		BasicInfo = c.BasicServiceURLs
	} else if section == refersSection {
		BasicInfo = c.BasicRefers
	}
	for key, ref := range Refs {
		var newURL *URL
		if basicConfKey, ok := ref.Parameters["basicRefer"]; ok {
			if basicRef, ok := BasicInfo[basicConfKey]; ok {
				newURL = basicRef.Copy()
				if ref.Protocol != "" {
					newURL.Protocol = ref.Protocol
				}
				if ref.Host != "" {
					newURL.Host = ref.Host
				}
				if ref.Port != 0 {
					newURL.Port = ref.Port
				}
				if ref.Group != "" {
					newURL.Group = ref.Group
				}
				if ref.Path != "" {
					newURL.Path = ref.Path
				}
				newURL.MergeParams(ref.Parameters)
			} else {
				newURL = ref
				vlog.Warningf("can not found basicRefer: %s. ref %s\n", basicConfKey, ref.GetIdentity())
			}

		} else {
			newURL = ref
		}
		urls[key] = newURL
	}
	return urls
}

func (c *Context) parseRefers() {
	c.RefersURLs = c.basicConfToURLs(basicRefersSection, refersSection)
}

func (c *Context) parseBasicRefers() {
	c.BasicRefers = c.confToURLs(basicRefersSection)
}

func (c *Context) parseServices() {
	c.ServiceURLs = c.basicConfToURLs(basicServicesSection, servicesSection)
}

func (c *Context) parserBasicServices() {
	c.BasicServiceURLs = c.confToURLs(basicServicesSection)
}
