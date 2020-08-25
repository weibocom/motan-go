package endpoint

import (
	motan "github.com/weibocom/motan-go/core"
	vlog "github.com/weibocom/motan-go/log"
)

type LocalEndpoint struct {
	url      *motan.URL
	provider motan.Provider
}

func (l *LocalEndpoint) GetName() string {
	return "localEndpoint"
}

func (l *LocalEndpoint) Initialize() {
	l.provider = motan.GetLocalProvider(l.url.Path)
	if l.provider == nil {
		vlog.Errorf("no local provider found for: " + l.url.GetIdentity())
		return
	}
	l.provider.SetURL(l.url)
	motan.Initialize(l.provider)
}

func (l *LocalEndpoint) GetURL() *motan.URL {
	return l.url
}

func (l *LocalEndpoint) SetURL(url *motan.URL) {
	l.url = url
}

func (l *LocalEndpoint) IsAvailable() bool {
	return true
}

func (l *LocalEndpoint) Call(request motan.Request) motan.Response {
	if l.provider == nil {
		return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{
			ErrCode: 500,
			ErrMsg:  "no local provider for: " + request.GetServiceName(),
			ErrType: motan.ServiceException,
		})
	}
	return l.provider.Call(request)
}

func (l *LocalEndpoint) Destroy() {
}

func (l *LocalEndpoint) SetSerialization(s motan.Serialization) {
}

func (l *LocalEndpoint) SetProxy(proxy bool) {
}
