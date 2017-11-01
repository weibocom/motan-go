package core

import (
	"flag"
	"reflect"

	cfg "github.com/weibocom/motan-go/config"
	"github.com/weibocom/motan-go/log"
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
)

// Context for agent, client, server. context is created depends on  config file
type Context struct {
	ConfigFile       string
	Config           *cfg.Config
	RegistryUrls     map[string]*Url
	RefersUrls       map[string]*Url
	BasicRefers      map[string]*Url
	ServiceUrls      map[string]*Url
	BasicServiceUrls map[string]*Url
	AgentUrl         *Url
	ClientUrl        *Url
	ServerUrl        *Url
}

var (
	urlFields = map[string]bool{"protocol": true, "host": true, "port": true, "path": true, "group": true}
)

// all env flag in motan-go
var (
	Port    = flag.Int("port", 0, "agent listen port")
	Mport   = flag.Int("mport", 0, "agent manage port")
	Pidfile = flag.String("pidfile", "", "agent manage port")
	CfgFile = flag.String("c", "./motan.yaml", "motan run conf")
	LocalIp = flag.String("localIp", "", "local ip for motan register")
)

func (c *Context) confToUrls(section string) map[string]*Url {
	urls := map[string]*Url{}
	sectionConf, _ := c.Config.GetSection(section)
	for key, info := range sectionConf {
		url := confToUrl(info.(map[interface{}]interface{}))
		urls[key.(string)] = url
	}
	return urls
}

func confToUrl(urlInfo map[interface{}]interface{}) *Url {
	urlParams := make(map[string]string)
	url := &Url{Parameters: urlParams}
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
	c.RegistryUrls = make(map[string]*Url)
	c.RefersUrls = make(map[string]*Url)
	c.BasicRefers = make(map[string]*Url)
	c.ServiceUrls = make(map[string]*Url)
	c.BasicServiceUrls = make(map[string]*Url)
	if c.ConfigFile == "" { // use flag as default config file name
		c.ConfigFile = *CfgFile
	}

	cfgRs, _ := cfg.NewConfigFromFile(c.ConfigFile)
	c.Config = cfgRs
	c.parseRegistrys()
	c.parseBasicRefers()
	c.parseRefers()
	c.parserBasicServices()
	c.parseServices()
	c.parseHostUrl()
}

// parse host url including agenturl, clienturl, serverurl
func (c *Context) parseHostUrl() {
	agentInfo, _ := c.Config.GetSection(agentSection)
	c.AgentUrl = confToUrl(agentInfo)
	clientInfo, _ := c.Config.GetSection(clientSection)
	c.ClientUrl = confToUrl(clientInfo)
	serverInfo, _ := c.Config.GetSection(serverSection)
	c.ServerUrl = confToUrl(serverInfo)
}

func (c *Context) parseRegistrys() {
	c.RegistryUrls = c.confToUrls(registrysSection)
}

func (c *Context) basicConfToUrls(basicsection, section string) map[string]*Url {
	urls := map[string]*Url{}
	Refs := c.confToUrls(section)
	var BasicInfo map[string]*Url
	if section == servicesSection {
		BasicInfo = c.BasicServiceUrls
	} else if section == refersSection {
		BasicInfo = c.BasicRefers
	}
	for key, ref := range Refs {
		var newUrl *Url
		if basicConfKey, ok := ref.Parameters["basicRefer"]; ok {
			if basicRef, ok := BasicInfo[basicConfKey]; ok {
				newUrl = basicRef.Copy()
				if ref.Protocol != "" {
					newUrl.Protocol = ref.Protocol
				}
				if ref.Host != "" {
					newUrl.Host = ref.Host
				}
				if ref.Port != 0 {
					newUrl.Port = ref.Port
				}
				if ref.Group != "" {
					newUrl.Group = ref.Group
				}
				if ref.Path != "" {
					newUrl.Path = ref.Path
				}
				newUrl.MergeParams(ref.Parameters)
			} else {
				newUrl = ref
				vlog.Warningf("can not found basicRefer: %s. ref %s\n", basicConfKey, ref.GetIdentity())
			}

		} else {
			newUrl = ref
		}
		urls[key] = newUrl
	}
	return urls
}

func (c *Context) parseRefers() {
	c.RefersUrls = c.basicConfToUrls(basicRefersSection, refersSection)
}

func (c *Context) parseBasicRefers() {
	c.BasicRefers = c.confToUrls(basicRefersSection)
}

func (c *Context) parseServices() {
	c.ServiceUrls = c.basicConfToUrls(basicServicesSection, servicesSection)
}

func (c *Context) parserBasicServices() {
	c.BasicServiceUrls = c.confToUrls(basicServicesSection)
}
