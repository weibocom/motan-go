package provider

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"github.com/weibocom/motan-go/config"
	"github.com/weibocom/motan-go/core"
	mhttp "github.com/weibocom/motan-go/http"
	"github.com/weibocom/motan-go/serialize"
	"net/http"
	"os"
	"testing"
	"time"
)

const httpProviderTestData = `
http-locations:
  test.domain:
  - match: /
    type: start
    upstream: test1
    rewriteRules:
    - 'exact /Test2/1 /(.*) /test'
  - match: ^/test2/.*
    type: regexp
    upstream: test2
    rewriteRules:
    - '!iregexp ^/Test2/1/.* ^/test2/(.*) /test/$1'
  - match: ^/test3/.*
    type: iregexp
    upstream: test3
    rewriteRules:
    - 'start / ^/(.*) /test/$1'
  - match: ^(/|/2/)(p1|p2).*
    type: regexp
    upstream: test4
    rewriteRules:
    - 'start / ^/(p1|p2)/(.*) /2/$1/$2'
`

func TestHTTPProvider_Call(t *testing.T) {
	context := &core.Context{}
	context.Config, _ = config.NewConfigFromReader(bytes.NewReader([]byte(httpProviderTestData)))
	providerURL := &core.URL{Protocol: "http", Path: "test4"}
	providerURL.PutParam(mhttp.DomainKey, "test.domain")
	providerURL.PutParam("requestTimeout", "2000")
	providerURL.PutParam("proxyAddress", "localhost:8090")
	provider := &HTTPProvider{url: providerURL, gctx: context}
	provider.Initialize()
	req := &core.MotanRequest{}
	req.ServiceName = "test4"
	req.Method = "/p1/test"
	req.SetAttachment("Host", "test.domain")
	req.SetAttachment(mhttp.QueryString, "a=b")
	assert.Nil(t, provider.Call(req).GetException())
	assert.Equal(t, "/2/p1/test?a=b", string(provider.Call(req).GetValue().([]byte)))

	req.SetAttachment(mhttp.Proxy, "true")
	httpReq := fasthttp.AcquireRequest()
	httpReq.Header.SetMethod("GET")
	httpReq.SetRequestURI("/p1/test?a=b")
	httpReq.Header.SetHost("test.domain")
	headerBuffer := &bytes.Buffer{}
	httpReq.Header.WriteTo(headerBuffer)
	req.Arguments = []interface{}{headerBuffer.Bytes(), nil}
	serialization := &serialize.SimpleSerialization{}
	body, _ := serialization.SerializeMulti(req.Arguments)
	req.Arguments = []interface{}{&core.DeserializableValue{Serialization: serialization, Body: body}}
	assert.Equal(t, "/2/p1/test?a=b", string(provider.Call(req).GetValue().([]interface{})[1].([]byte)))

	// runtime info
	info := provider.GetRuntimeInfo()
	assert.NotNil(t, info)

	urlInfo, ok := info[core.RuntimeUrlKey]
	assert.True(t, ok)
	assert.Equal(t, providerURL.ToExtInfo(), urlInfo)
}

func TestHTTPProvider_Http_Exception(t *testing.T) {
	context := &core.Context{}
	context.Config, _ = config.NewConfigFromReader(bytes.NewReader([]byte(httpProviderTestData)))
	providerURL := &core.URL{Protocol: "http", Path: "test4"}
	providerURL.PutParam(mhttp.DomainKey, "test.domain")
	providerURL.PutParam("requestTimeout", "2000")
	providerURL.PutParam("proxyAddress", "localhost:8091")
	provider := &HTTPProvider{url: providerURL, gctx: context}
	provider.Initialize()
	req := &core.MotanRequest{}
	req.ServiceName = "test4"
	req.Method = "/p1/test"
	req.SetAttachment("Host", "test.domain")
	req.SetAttachment(mhttp.QueryString, "a=b")
	assert.Nil(t, provider.Call(req).GetException())
}

func TestHTTPProvider_Http_EnableException(t *testing.T) {
	context := &core.Context{}
	context.Config, _ = config.NewConfigFromReader(bytes.NewReader([]byte(httpProviderTestData)))
	providerURL := &core.URL{Protocol: "http", Path: "test4"}
	providerURL.PutParam(mhttp.DomainKey, "test.domain")
	providerURL.PutParam("requestTimeout", "2000")
	providerURL.PutParam("proxyAddress", "localhost:8091")
	providerURL.PutParam(mhttp.EnableHttpExceptionKey, "true")
	provider := &HTTPProvider{url: providerURL, gctx: context}
	provider.Initialize()
	req := &core.MotanRequest{}
	req.ServiceName = "test4"
	req.Method = "/p1/test"
	req.SetAttachment("Host", "test.domain")
	req.SetAttachment(mhttp.QueryString, "a=b")
	exception := provider.Call(req).GetException()
	assert.NotNil(t, exception)
	assert.Equal(t, exception.ErrCode, 500)
	assert.Equal(t, exception.ErrType, core.BizException)
	assert.Equal(t, exception.ErrMsg, "request failed")
}

func TestMain(m *testing.M) {
	go func() {
		var addr = ":8090"
		handler := &http.ServeMux{}
		handler.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
			request.ParseForm()
			writer.Write([]byte(request.URL.String()))
		})
		http.ListenAndServe(addr, handler)
	}()
	go func() {
		// 返回500的server
		var addr = ":8091"
		handler := &http.ServeMux{}
		handler.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
			request.ParseForm()
			writer.WriteHeader(500)
			writer.Write([]byte("request failed"))
		})
		http.ListenAndServe(addr, handler)
	}()
	time.Sleep(time.Second * 3)
	os.Exit(m.Run())
}
