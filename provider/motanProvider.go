package provider

import (
	"errors"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
	"github.com/weibocom/motan-go/log"
	"time"
)

type MotanProvider struct {
	url        *motan.URL
	ep         *endpoint.MotanEndpoint
	gctx       *motan.Context
	available  bool
	extFactory motan.ExtentionFactory
}

func (m *MotanProvider) Initialize() {
	m.ep = &endpoint.MotanEndpoint{}
	m.ep.SetProxy(true)
	if confId, ok := m.url.Parameters[motan.URLConfKey]; ok {
		if url, ok := m.gctx.ReverseProxyURLs[confId]; ok {
			m.ep.SetURL(url)
			if serialization := motan.GetSerialization(url, m.extFactory); serialization != nil {
				m.ep.SetSerialization(serialization)
			} else {
				vlog.Errorf("MotanProvider can not find Serialization in DefaultExtentionFactory! url:%+v\n", url)
			}
		} else {
			vlog.Errorf("MotanProvider can not find URL in ReverseProxyService! ServiceId:%s\n", confId)
		}
	} else {
		vlog.Errorf("MotanProvider can not find ServiceId in ReverseProxyService! ServiceId:%s\n", confId)
	}
	m.ep.Initialize()
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
