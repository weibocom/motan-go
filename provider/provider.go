package provider

import (
	"fmt"
	"reflect"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

// ext name
const (
	CGI     = "cgi"
	HTTP    = "http"
	MOTAN2  = "motan2"
	Mock    = "mockProvider"
	Default = "default"
)

func RegistDefaultProvider(extFactory motan.ExtentionFactory) {

	extFactory.RegistExtProvider(CGI, func(url *motan.URL) motan.Provider {
		return &CgiProvider{url: url}
	})

	extFactory.RegistExtProvider(HTTP, func(url *motan.URL) motan.Provider {
		return &HTTPProvider{url: url}
	})

	extFactory.RegistExtProvider(MOTAN2, func(url *motan.URL) motan.Provider {
		return &MotanProvider{url: url, extFactory: extFactory}
	})

	extFactory.RegistExtProvider(Mock, func(url *motan.URL) motan.Provider {
		return &MockProvider{URL: url}
	})

	extFactory.RegistExtProvider(Default, func(url *motan.URL) motan.Provider {
		return &DefaultProvider{url: url}
	})
}

type DefaultProvider struct {
	service interface{}
	methods map[string]reflect.Value
	url     *motan.URL
}

func (d *DefaultProvider) Initialize() {
	d.methods = make(map[string]reflect.Value, 32)
	if d.service != nil && d.url != nil {
		v := reflect.ValueOf(d.service)
		if v.Kind() != reflect.Ptr {
			vlog.Errorf("can not init provider. service is not a pointer. service :%v, url:%v\n", d.service, d.url)
			return
		}
		for i := 0; i < v.NumMethod(); i++ {
			name := v.Type().Method(i).Name
			vm := v.MethodByName(name)
			d.methods[name] = vm
		}

	} else {
		vlog.Errorf("can not init provider. service :%v, url:%v\n", d.service, d.url)
	}
}

func (d *DefaultProvider) SetService(s interface{}) {
	d.service = s
}

func (d *DefaultProvider) GetURL() *motan.URL {
	return d.url
}

func (d *DefaultProvider) SetURL(url *motan.URL) {
	d.url = url
}

func (d *DefaultProvider) GetPath() string {
	return d.url.Path
}

func (d *DefaultProvider) IsAvailable() bool {
	return true
}

func (d *DefaultProvider) Destroy() {}

func (d *DefaultProvider) Call(request motan.Request) (res motan.Response) {
	m, exit := d.methods[motan.FirstUpper(request.GetMethod())]
	if !exit {
		vlog.Errorf("mehtod not found in provider. %s\n", motan.GetReqInfo(request))
		return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "mehtod " + request.GetMethod() + " is not found in provider.", ErrType: motan.ServiceException})
	}
	defer func() {
		if err := recover(); err != nil {
			vlog.Errorf("provider call fail! e: %v, %s\n", err, motan.GetReqInfo(request))
			res = motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: fmt.Sprintf("request process fail in provider. e:%v", err), ErrType: motan.ServiceException})
		}
	}()

	inNum := m.Type().NumIn()
	if inNum > 0 {
		values := make([]interface{}, 0, inNum)
		for i := 0; i < inNum; i++ {
			// TODO how to reflect value pointer???
			values = append(values, reflect.New(m.Type().In(i)).Type())
		}
		err := request.ProcessDeserializable(values)
		if err != nil {
			return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "deserialize arguments fail." + err.Error(), ErrType: motan.ServiceException})
		}
	}

	vs := make([]reflect.Value, 0, len(request.GetArguments()))
	for _, arg := range request.GetArguments() {
		vs = append(vs, reflect.ValueOf(arg))
	}
	ret := m.Call(vs)
	mres := &motan.MotanResponse{RequestID: request.GetRequestID()}
	if len(ret) > 0 { // only use first return value.
		mres.Value = ret[0]
		res = mres
	}
	return res
}

type MockProvider struct {
	URL          *motan.URL
	MockResponse motan.Response
	service      interface{}
}

func (m *MockProvider) GetName() string {
	return "mockProvider"
}

func (m *MockProvider) GetURL() *motan.URL {
	return m.URL
}

func (m *MockProvider) SetURL(url *motan.URL) {
	m.URL = url
}

func (m *MockProvider) IsAvailable() bool {
	return true
}

func (m *MockProvider) SetProxy(proxy bool) {}

func (m *MockProvider) SetSerialization(s motan.Serialization) {}

func (m *MockProvider) Call(request motan.Request) motan.Response {
	if m.MockResponse != nil {
		return m.MockResponse
	}
	return &motan.MotanResponse{ProcessTime: 1, Value: "ok"}
}

func (m *MockProvider) Destroy() {}

func (m *MockProvider) Initialize() {
}

func (m *MockProvider) SetService(s interface{}) {
	m.service = s
}

func (m *MockProvider) GetPath() string {
	return m.URL.Path
}
