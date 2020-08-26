package provider

import (
	"errors"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

type MotanProvider struct {
	url        *motan.URL
	ep         motan.EndPoint
	available  bool
	extFactory motan.ExtensionFactory
}

const (
	ProxyHostKey = "proxy.host"
	DefaultHost  = "127.0.0.1"
)

func (m *MotanProvider) Initialize() {
	protocol, port, err := motan.ParseExportInfo(m.url.GetParam(motan.ProxyKey, ""))
	if err != nil {
		vlog.Errorf("reverse proxy service config in %s error!", motan.ProxyKey)
		return
	} else if port <= 0 {
		vlog.Errorln("reverse proxy service port config error!")
		return
	}
	host := m.url.GetParam(ProxyHostKey, DefaultHost)
	endpointURL := m.url.Copy()
	endpointURL.Protocol = protocol
	endpointURL.Host = host
	endpointURL.Port = port
	// when as reverse proxy for a motan service, we can retry connect in less time
	if endpointURL.GetParam(motan.ConnectRetryIntervalKey, "") == "" {
		// 5000ms
		endpointURL.PutParam(motan.ConnectRetryIntervalKey, "5000")
	}
	// no need disable endpoint when error occurs
	if endpointURL.GetParam(motan.ErrorCountThresholdKey, "") == "" {
		endpointURL.PutParam(motan.ErrorCountThresholdKey, "0")
	}
	m.ep = m.extFactory.GetEndPoint(endpointURL)
	if m.ep == nil {
		vlog.Errorf("Can not find %s endpoint in ExtensionFactory!", protocol)
		return
	}
	m.ep.SetProxy(true)
	motan.Initialize(m.ep)
	m.available = true
}

func (m *MotanProvider) Call(request motan.Request) motan.Response {
	if m.IsAvailable() {
		return m.ep.Call(request)
	}
	t := time.Now().UnixNano()
	res := &motan.MotanResponse{Attachment: motan.NewStringMap(motan.DefaultAttachmentSize)}
	fillException(res, t, errors.New("reverse proxy call err: motanProvider is unavailable"))
	return res
}

func (m *MotanProvider) GetPath() string {
	return m.url.Path
}

func (m *MotanProvider) SetService(s interface{}) {}

func (m *MotanProvider) SetContext(context *motan.Context) {}

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

func (m *MotanProvider) Destroy() {
	m.ep.Destroy()
}

func (m *MotanProvider) IsAvailable() bool {
	return m.available && m.ep.IsAvailable()
}
