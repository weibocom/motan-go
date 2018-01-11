package core

import (
	"flag"
	"reflect"

	cfg "github.com/weibocom/motan-go/config"
	"github.com/weibocom/motan-go/log"
	"strings"
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
<<<<<<< HEAD
	Port    = flag.Int("port", 0, "agent listen port")
	Mport   = flag.Int("mport", 0, "agent manage port")
	Pidfile = flag.String("pidfile", "", "agent manage port")
	CfgFile = flag.String("c", "/Users/zengnjin/go/src/git.intra.weibo.com/openapi_rd/weibo-motan-go/main/motan.yaml", "motan run conf")
	LocalIP = flag.String("localIP", "", "local ip for motan register")
=======
	Port         = flag.Int("port", 0, "agent listen port")
	Mport        = flag.Int("mport", 0, "agent manage port")
	Pidfile      = flag.String("pidfile", "", "agent manage port")
	CfgFile      = flag.String("c", "./motan.yaml", "motan run conf")
	LocalIP      = flag.String("localIP", "", "local ip for motan register")
	IDC          = flag.String("idc", "", "the idc info for agent or client.")
	DynamicConfs = flag.String("dynamicConf", "", "dynamic config file for config placeholder")
>>>>>>> 6c19c0cb71b20f4b85f6b18e2cc23fbe06686a55
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
