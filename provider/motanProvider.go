package provider

import (
	"errors"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	"time"
)

type MotanProvider struct {
	url        *motan.URL
	ep         motan.EndPoint
	available  bool
	extFactory motan.ExtentionFactory
}

const (
	ProxyConfKey = "proxy"
)

func (m *MotanProvider) Initialize() {
	protocol, port, err := motan.ParseExportInfo(m.url.GetParam(ProxyConfKey, ""))
	if err != nil {
		vlog.Errorf("reverse proxy service config in %s error!\n", ProxyConfKey)
		return
	} else if port <= 0 || port == 9982 {
		vlog.Errorln("reverse proxy service port config error!")
		return
	}
	m.ep = m.extFactory.GetEndPoint(&motan.URL{Protocol: protocol, Port: port})
	if m.ep == nil {
		vlog.Errorf("Can not find %s endpoint in ExtentionFactory!\n", protocol)
		return
	}
	m.ep.SetProxy(true)
	motan.Initialize(m.ep)
	m.available = m.ep.IsAvailable()
}

func (m *MotanProvider) Call(request motan.Request) motan.Response {
	if m.IsAvailable() {
		return m.ep.Call(request)
	}
	t := time.Now().UnixNano()
	res := &motan.MotanResponse{Attachment: make(map[string]string)}
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

func (m *MotanProvider) Destroy() {}

func (m *MotanProvider) IsAvailable() bool {
	return m.available
}
