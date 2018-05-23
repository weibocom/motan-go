package provider

import (
	"errors"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	"strconv"
	"time"
)

type MotanProvider struct {
	url        *motan.URL
	ep         motan.EndPoint
	gctx       *motan.Context
	available  bool
	extFactory motan.ExtentionFactory
}

const (
	ProtocolConfKey      = "proxy.protocol"
	HostConfKey          = "proxy.host"
	PortConfKey          = "proxy.port"
	SerializationConfKey = "proxy.serialization"
)

func (m *MotanProvider) Initialize() {
	if confId, ok := m.url.Parameters[motan.URLConfKey]; ok {
		if url, ok := m.gctx.ServiceURLs[confId]; ok {
			hostTemp, err := url.Parameters[HostConfKey]
			if !err {
				vlog.Errorf("Can not find %s in service config!\n", HostConfKey)
			}
			portTemp, e := strconv.Atoi(url.Parameters[PortConfKey])
			if e != nil {
				vlog.Errorf("Can not find %s in service config!\n", PortConfKey)
			}
			protocolTemp, err := url.Parameters[ProtocolConfKey]
			if !err {
				vlog.Errorf("Can not find %s in service config!\n", ProtocolConfKey)
			}
			serialTemp, err := url.Parameters[SerializationConfKey]
			if !err {
				vlog.Errorf("Can not find %s in service config!\n", SerializationConfKey)
			}
			proxyUrl := &motan.URL{Host: hostTemp, Port: portTemp, Protocol: protocolTemp, Parameters: map[string]string{motan.URLConfKey: confId, motan.SerializationKey: serialTemp}}
			m.ep = m.extFactory.GetEndPoint(proxyUrl)
			if m.ep != nil {
				m.ep.SetURL(proxyUrl)
			} else {
				vlog.Errorf("Can not find endpoint in ExtentionFactory! url: %+v\n", proxyUrl)
			}
			if serialization := motan.GetSerialization(url, m.extFactory); serialization != nil {
				m.ep.SetSerialization(serialization)
			} else {
				vlog.Errorf("Can not find Serialization in ExtentionFactory! url: %+v\n", url)
			}
		} else {
			vlog.Errorf("Can not find URL in ServiceURLs! ServiceId: %s\n", confId)
		}
	} else {
		vlog.Errorf("Can not find ServiceId! ServiceId: %s\n", confId)
	}
	m.ep.SetProxy(true)
	motan.Initialize(m.ep)
}

func (m *MotanProvider) Call(request motan.Request) motan.Response {
	if m.ep.IsAvailable() {
		return m.ep.Call(request)
	}
	t := time.Now().UnixNano()
	res := &motan.MotanResponse{Attachment: make(map[string]string)}
	fillException(res, t, errors.New("reverse proxy call err: endpoint is unavailable"))
	return res
}

func (m *MotanProvider) GetPath() string {
	return m.url.Path
}

func (m *MotanProvider) SetService(s interface{}) {}

func (m *MotanProvider) SetContext(context *motan.Context) {
	m.gctx = context
}

func (m *MotanProvider) GetName() string {
	return "motanProvider"
}

func (m *MotanProvider) GetURL() *motan.URL {
	return m.url
}

func (m *MotanProvider) SetURL(url *motan.URL) {
	m.url = url
}

func (m *MotanProvider) SetSerialization(s motan.Serialization) {}

func (m *MotanProvider) SetProxy(proxy bool) {}

func (m *MotanProvider) Destroy() {}

func (m *MotanProvider) IsAvailable() bool {
	return m.ep.IsAvailable()
}
