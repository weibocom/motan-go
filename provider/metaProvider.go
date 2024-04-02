package provider

import (
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/meta"
)

type MetaProvider struct{}

func (m *MetaProvider) Initialize() {}

func (m *MetaProvider) Call(request motan.Request) motan.Response {
	resp := &motan.MotanResponse{
		RequestID: request.GetRequestID(),
		Value:     meta.GetDynamicMeta(),
	}
	return resp
}

func (m *MetaProvider) GetPath() string {
	return "com.weibo.api.motan.runtime.meta.MetaService"
}

func (m *MetaProvider) SetService(s interface{}) {}

func (m *MetaProvider) SetContext(context *motan.Context) {}

func (m *MetaProvider) GetName() string {
	return "metaProvider"
}

func (m *MetaProvider) GetURL() *motan.URL {
	return nil
}

func (m *MetaProvider) SetURL(url *motan.URL) {}

func (m *MetaProvider) SetSerialization(s motan.Serialization) {}

func (m *MetaProvider) SetProxy(proxy bool) {}

func (m *MetaProvider) Destroy() {}

func (m *MetaProvider) IsAvailable() bool {
	return true
}
