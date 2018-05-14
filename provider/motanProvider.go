package provider

import (
	"errors"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
	"github.com/weibocom/motan-go/log"
	"github.com/weibocom/motan-go/serialize"
	"time"
)

type MotanProvider struct {
	url       *motan.URL              //包含协议、端口和路径
	ep        *endpoint.MotanEndpoint //通信工具，相当于client
	gctx      *motan.Context          //存储配置文件，外部初始化
	available bool                    //是否可用
}

const (
	ReverseProxyServiceConfKey = "reverse-proxy-service"
	HostKey                    = "host"
	PortKey                    = "port"
)

//有多少service就初始化多少provider
func (m *MotanProvider) Initialize() {
	//初始化endpoint
	m.ep = &endpoint.MotanEndpoint{}
	m.ep.SetProxy(true) //endpoint中，message和request之间进行转化需要验证proxy和serialization

	//解析service配置文件
	srvConf, _ := m.gctx.Config.GetSection(ReverseProxyServiceConfKey)
	confId := m.url.Parameters[motan.URLConfKey]
	srvMap := srvConf[confId].(map[interface{}]interface{})
	if serial, ok := srvMap[motan.SerializationKey]; ok {
		//目前只有一种序列化方式
		switch serial {
		case serialize.Simple:
			m.ep.SetSerialization(&serialize.SimpleSerialization{})
		default:
			m.ep.SetSerialization(&serialize.SimpleSerialization{})
		}
	} else {
		vlog.Errorf("reverse proxy serialization is nil: serviceName:%s", confId)
		return
	}
	url := &motan.URL{Protocol: m.url.Protocol}
	if host, ok := srvMap[HostKey]; ok {
		url.Host = host.(string)
	} else {
		vlog.Errorf("reverse proxy host is nil: serviceName:%s", confId)
		//return
	}
	if port, ok := srvMap[PortKey]; ok {
		url.Port = port.(int)
	} else {
		vlog.Errorf("reverse proxy port is nil: serviceName:%s", confId)
		//return
	}
	m.ep.SetURL(url)
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

//实现core.provider接口，获取path
func (m *MotanProvider) GetPath() string {
	return m.url.Path
}

//实现core.provider接口
func (m *MotanProvider) SetService(s interface{}) {}

// 实现core.provider接口，设置gctx字段
func (m *MotanProvider) SetContext(context *motan.Context) {
	m.gctx = context
}

// 实现core.Provider接口，获取provider名称
func (m *MotanProvider) GetName() string {
	return "motanProvider"
}

// 实现core.Provider接口，获取url
func (m *MotanProvider) GetURL() *motan.URL {
	return m.url
}

// 实现core.Provider接口，设置url字段
func (m *MotanProvider) SetURL(url *motan.URL) {
	m.url = url
}

// 实现core.Provider接口
func (m *MotanProvider) SetSerialization(s motan.Serialization) {}

// 实现core.Provider接口
func (m *MotanProvider) SetProxy(proxy bool) {}

// 实现core.Provider接口
func (m *MotanProvider) Destroy() {}

// 实现core.Provider接口，判断endpoint是否可用
func (m *MotanProvider) IsAvailable() bool {
	return m.ep.IsAvailable()
}
