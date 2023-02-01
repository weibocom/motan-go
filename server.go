package motan

import (
	"errors"
	"flag"
	"fmt"
	"github.com/weibocom/motan-go/config"
	"github.com/weibocom/motan-go/provider"
	"hash/fnv"
	"reflect"
	"strconv"
	"strings"
	"sync"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	mserver "github.com/weibocom/motan-go/server"
)

// MSContext is Motan Server Context
type MSContext struct {
	config       *config.Config
	context      *motan.Context
	extFactory   motan.ExtensionFactory
	portService  map[int]motan.Exporter
	portServer   map[string]motan.Server
	serviceImpls map[string]interface{}
	registries   map[string]motan.Registry // all registries used for services

	csync  sync.Mutex
	inited bool
}

const (
	defaultServerPort = "9982"
	defaultProtocol   = "motan2"
)

var (
	serverContextMap   = make(map[string]*MSContext, 8)
	serverContextMutex sync.Mutex
)

func NewMotanServerContextFromConfig(conf *config.Config) (ms *MSContext) {
	ms = &MSContext{config: conf}
	motan.Initialize(ms)

	section, err := ms.context.Config.GetSection("motan-server")
	if err != nil {
		fmt.Println("get config of \"motan-server\" fail! err " + err.Error())
	}

	logDir := ""
	if section != nil && section["log_dir"] != nil {
		logDir = section["log_dir"].(string)
	}
	if logDir == "" {
		logDir = "."
	}
	initLog(logDir, section)
	registerSwitchers(ms.context)

	return ms
}

// GetMotanServerContext start a motan server context by config
// a motan server context can listen multi ports and provide many services. so a single motan server context is suggested
// default context will be used if confFile is empty
func GetMotanServerContext(confFile string) *MSContext {
	return GetMotanServerContextWithFlagParse(confFile, true)
}

func GetMotanServerContextWithFlagParse(confFile string, parseFlag bool) *MSContext {
	if parseFlag && !flag.Parsed() {
		flag.Parse()
	}
	serverContextMutex.Lock()
	defer serverContextMutex.Unlock()

	conf, err := config.NewConfigFromFile(confFile)
	if err != nil {
		vlog.Errorf("parse config file fail, error: %s, file: %s", err, confFile)
		return nil
	}

	ms := serverContextMap[confFile]
	if ms == nil {
		ms = NewMotanServerContextFromConfig(conf)
		serverContextMap[confFile] = ms
	}
	return ms
}

func (m *MSContext) Start(extfactory motan.ExtensionFactory) {
	m.csync.Lock()
	defer m.csync.Unlock()
	m.extFactory = extfactory
	if m.extFactory == nil {
		m.extFactory = GetDefaultExtFactory()
	}

	for _, url := range m.context.ServiceURLs {
		m.export(url)
	}
}

func (m *MSContext) hashInt(s string) int {
	h := fnv.New32a()
	h.Write([]byte(s))
	return int(h.Sum32())
}
func (m *MSContext) export(url *motan.URL) {
	defer motan.HandlePanic(nil)
	service := m.serviceImpls[url.Parameters[motan.RefKey]]
	if service != nil {
		//TODO multi protocol support. convert to multi url
		export := url.GetParam(motan.ExportKey, "")
		port := defaultServerPort
		protocol := defaultProtocol
		if export != "" {
			s := motan.TrimSplit(export, ":")
			if len(s) == 1 {
				port = s[0]
			} else if len(s) == 2 {
				if s[0] != "" {
					protocol = s[0]
				}
				port = s[1]
			}
		}
		url.Protocol = protocol
		var porti int
		var serverKey string
		var err error
		if v := url.GetParam(provider.ProxyHostKey, ""); strings.HasPrefix(v, motan.UnixSockProtocolFlag) {
			porti = 0
			serverKey = v
		} else if v := url.GetParam(motan.UnixSockKey, ""); v != "" {
			porti = 0
			serverKey = v
		} else {
			porti, err = strconv.Atoi(port)
			if err != nil {
				vlog.Errorf("export port not int. port:%s, url:%+v", port, url)
				return
			}
			serverKey = port
		}
		url.Port = porti
		if url.Host == "" {
			url.Host = motan.GetLocalIP()
		}
		url.ClearCachedInfo()
		provider := GetDefaultExtFactory().GetProvider(url)
		provider.SetService(service)
		motan.Initialize(provider)
		provider = mserver.WrapWithFilter(provider, m.extFactory, m.context)

		exporter := &mserver.DefaultExporter{}
		exporter.SetProvider(provider)

		server := m.portServer[serverKey]

		if server == nil {
			server = m.extFactory.GetServer(url)
			handler := GetDefaultExtFactory().GetMessageHandler("default")
			motan.Initialize(handler)
			handler.AddProvider(provider)
			server.Open(false, false, handler, m.extFactory)
			m.portServer[serverKey] = server
		} else if canShareChannel(*url, *server.GetURL()) {
			server.GetMessageHandler().AddProvider(provider)
		} else {
			vlog.Errorf("service export fail! can not share channel.url:%v, port url:%v", url, server.GetURL())
			return
		}
		err = exporter.Export(server, m.extFactory, m.context)
		if err != nil {
			vlog.Errorf("service export fail! url:%v, err:%v", url, err)
		} else {
			vlog.Infof("service export success. url:%v", url)
			for _, r := range exporter.Registries {
				rid := r.GetURL().GetIdentity()
				if _, ok := m.registries[rid]; !ok {
					m.registries[rid] = r
				}
			}
		}
	}
}

func (m *MSContext) Initialize() {
	m.csync.Lock()
	defer m.csync.Unlock()
	if !m.inited {
		m.context = motan.NewContextFromConfig(m.config, "", "")
		m.portService = make(map[int]motan.Exporter, 32)
		m.portServer = make(map[string]motan.Server, 32)
		m.serviceImpls = make(map[string]interface{}, 32)
		m.registries = make(map[string]motan.Registry)
		m.inited = true
	}
}

// RegisterService register service with serviceId for config ref.
// the type.string will used as serviceId if sid is not set. e.g. 'packageName.structName'
func (m *MSContext) RegisterService(s interface{}, sid string) error {
	if s == nil {
		vlog.Errorln("MSContext register service is nil!")
		return errors.New("register service is nil")
	}
	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Ptr {
		vlog.Errorf("register service must be a pointer of struct. service:%+v", s)
		return errors.New("register service must be a pointer of struct")
	}
	t := v.Elem().Type()
	hasConfig := false
	ref := sid
	if ref == "" {
		ref = t.String()
	}
	// check export config
	for _, url := range m.context.ServiceURLs {
		if url.Parameters != nil && ref == url.Parameters[motan.RefKey] {
			hasConfig = true
			break
		}
	}
	if !hasConfig {
		vlog.Errorf("can not find export config for register service. service:%+v", s)
		return errors.New("can not find export config for register service")
	}
	m.serviceImpls[ref] = s
	return nil
}

// ServicesAvailable will enable all service registed in registries
func (m *MSContext) ServicesAvailable() {
	// TODO: same as agent
	availableService(m.registries)
}

// ServicesUnavailable will enable all service registed in registries
func (m *MSContext) ServicesUnavailable() {
	unavailableService(m.registries)
}

func canShareChannel(u1 motan.URL, u2 motan.URL) bool {
	if u1.Protocol != u2.Protocol {
		return false
	}
	if !motan.IsSame(u1.Parameters, u2.Parameters, motan.SerializationKey, "") {
		return false
	}
	return true
}

func availableService(registries map[string]motan.Registry) {
	defer motan.HandlePanic(nil)
	if registries != nil {
		for _, r := range registries {
			r.Available(nil)
		}
	}
}

func unavailableService(registries map[string]motan.Registry) {
	defer motan.HandlePanic(nil)
	if registries != nil {
		for _, r := range registries {
			r.Unavailable(nil)
		}
	}
}
