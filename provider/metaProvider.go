package provider

import (
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/meta"
	"github.com/weibocom/motan-go/serialize"
)

type MetaProvider struct{}

func (m *MetaProvider) Initialize() {}

func (m *MetaProvider) Call(request motan.Request) motan.Response {
	// only support motan2 protocol and breeze serialization
	if request.GetRPCContext(true).IsMotanV1 {
		return &motan.MotanResponse{
			Exception: &motan.Exception{ErrCode: 500, ErrMsg: "meta provider not support motan1 protocol"},
		}
	}
	serialization := &serialize.BreezeSerialization{}
	b, err := serialization.Serialize(meta.GetDynamicMeta())
	if err != nil {
		return &motan.MotanResponse{
			Exception: &motan.Exception{ErrCode: 500, ErrMsg: "meta provider serialize fail"},
		}
	}
	resp := &motan.MotanResponse{
		RequestID: request.GetRequestID(),
		Value:     b,
		RPCContext: &motan.RPCContext{
			Serialized:   true,
			SerializeNum: serialize.BreezeNumber,
		},
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

func (m *MetaProvider) GetRuntimeInfo() map[string]interface{} {
	info := map[string]interface{}{
		motan.RuntimeNameKey: m.GetName(),
	}
	return info
}
