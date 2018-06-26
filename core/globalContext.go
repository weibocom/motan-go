package core

import (
	"flag"
	"reflect"

	cfg "github.com/weibocom/motan-go/config"
	"strings"
	"fmt"
)

const (
	registrysSection     = "motan-registry"
	basicRefersSection   = "motan-basicRefer"
	refersSection        = "motan-refer"
	basicServicesSection = "motan-basicService"
	servicesSection      = "motan-service"
	agentSection         = "motan-agent"
	clientSection        = "motan-client"
	serverSection        = "motan-server"
	// URLConfKey add confid to url params
	URLConfKey = "conf-id"

	basicReferKey   = "basicRefer"
	basicServiceKey = "basicService"
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
	CfgFile      = flag.String("c", "./motan.yaml", "motan run conf")
	LocalIP      = flag.String("localIP", "", "local ip for motan register")
	IDC          = flag.String("idc", "", "the idc info for agent or client.")
	DynamicConfs = flag.String("dynamicConf", "", "dynamic config file for config placeholder")
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

	cfgRs, _ := cfg.NewConfigFromFile(c.ConfigFile)
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

	c.Config = cfgRs
	c.parseRegistrys()
	c.parseBasicRefers()
	c.parseRefers()
	c.parserBasicServices()
	c.parseServices()
	c.parseHostURL()
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
	} else if section == refersSection {
		basicURLs = c.BasicRefers
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
				fmt.Printf("load %s configuration success. url: %s\n", basicConfName, url.GetIdentity())
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
	c.BasicRefers = c.confToURLs(basicRefersSection)
}

func (c *Context) parseServices() {
	c.ServiceURLs = c.basicConfToURLs(servicesSection)
}

func (c *Context) parserBasicServices() {
	c.BasicServiceURLs = c.confToURLs(basicServicesSection)
}
