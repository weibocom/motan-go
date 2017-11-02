package motan

import (
	"errors"
	"flag"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	mserver "github.com/weibocom/motan-go/server"
)

type MSContext struct {
	confFile     string
	context      *motan.Context
	extFactory   motan.ExtentionFactory
	portService  map[int]motan.Exporter
	portServer   map[int]motan.Server
	serviceImpls map[string]interface{}

	csync  sync.Mutex
	inited bool
}

const (
	defaultServerPort = "9982"
	defaultProtocal   = "motan2"
)

var (
	serverContextMap   = make(map[string]*MSContext, 8)
	serverContextMutex sync.Mutex
)

// GetMotanServerContext start a motan server context by config
// a motan server context can listen multi ports and provide many services. so a single motan server context is suggested
// default context will be used if confFile is empty
func GetMotanServerContext(confFile string) *MSContext {
	if !flag.Parsed() {
		flag.Parse()
	}
	serverContextMutex.Lock()
	defer serverContextMutex.Unlock()
	ms := serverContextMap[confFile]
	if ms == nil {

		ms = &MSContext{confFile: confFile}
		serverContextMap[confFile] = ms
		motan.Initialize(ms)
		section, err := ms.context.Config.GetSection("motan-server")
		if err != nil {
			fmt.Println("get config of \"motan-server\" fail! err " + err.Error())
		}

		logdir := ""
		if section != nil && section["log_dir"] != nil {
			logdir = section["log_dir"].(string)
		}
		if logdir == "" {
			logdir = "."
		}
		initLog(logdir)
	}
	return ms
}

func (m *MSContext) Start(extfactory motan.ExtentionFactory) {
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

func (m *MSContext) export(url *motan.URL) {
	defer func() {
		if err := recover(); err != nil {
			vlog.Errorf("MSContext export fail! url: %v, err:%v\n", url, err)
		}
	}()
	service := m.serviceImpls[url.Parameters[motan.RefKey]]
	if service != nil {
		//TODO multi protocol support. convert to multi url
		export := url.GetParam(motan.ExportKey, "")
		port := defaultServerPort
		protocol := defaultProtocal
		if export != "" {
			s := strings.Split(export, ":")
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
		porti, err := strconv.Atoi(port)
		if err != nil {
			vlog.Errorf("export port not int. port:%s, url:%+v\n", port, url)
			return
		}
		url.Port = porti
		if url.Host == "" {
			url.Host = motan.GetLocalIP()
		}
		provider := GetDefaultExtFactory().GetProvider(url)
		provider.SetService(service)
		motan.Initialize(provider)
		provider = mserver.WarperWithFilter(provider, m.extFactory)

		exporter := &mserver.DefaultExporter{}
		exporter.SetProvider(provider)

		server := m.portServer[url.Port]

		if server == nil {
			server = m.extFactory.GetServer(url)
			handler := GetDefaultExtFactory().GetMessageHandler("default")
			motan.Initialize(handler)
			handler.AddProvider(provider)
			server.Open(false, false, handler, m.extFactory)
			m.portServer[url.Port] = server
		} else if canShareChannel(*url, *server.GetURL()) {
			server.GetMessageHandler().AddProvider(provider)
		} else {
			vlog.Errorf("service export fail! can not share channel.url:%v, port url:%v\n", url, server.GetURL())
			return
		}
		err = exporter.Export(server, m.extFactory, m.context)
		if err != nil {
			vlog.Errorf("service export fail! url:%v, err:%v\n", url, err)
		} else {
			vlog.Infof("service export success. url:%v\n", url)
		}
	}
}

func (m *MSContext) Initialize() {
	m.csync.Lock()
	defer m.csync.Unlock()
	if !m.inited {
		m.context = &motan.Context{ConfigFile: m.confFile}
		m.context.Initialize()

		m.portService = make(map[int]motan.Exporter, 32)
		m.portServer = make(map[int]motan.Server, 32)
		m.serviceImpls = make(map[string]interface{}, 32)
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
		vlog.Errorf("register service must be a pointer of struct. service:%+v\n", s)
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
		vlog.Errorf("can not find export config for register service. service:%+v\n", s)
		return errors.New("can not find export config for register service")
	}
	m.serviceImpls[ref] = s
	return nil
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
