package provider

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	URL "net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	motan "github.com/weibocom/motan-go/core"
	mhttp "github.com/weibocom/motan-go/http"
	"github.com/weibocom/motan-go/log"
)

type sConfT map[string]string
type srvConfT map[string]sConfT
type srvURLMapT map[string]srvConfT

// HTTPProvider struct
type HTTPProvider struct {
	url       *motan.URL
	srvURLMap srvURLMapT
	gctx      *motan.Context
	mixVars   []string
	// for transparent http proxy
	fastClient          *fasthttp.HostClient
	proxyAddr           string
	proxySchema         string
	locationMatcher     *mhttp.LocationMatcher
	maxConnections      int
	domain              string
	defaultHTTPMethod   string
	enableRewrite       bool
	enableHttpException bool
}

func (h *HTTPProvider) GetRuntimeInfo() map[string]interface{} {
	info := map[string]interface{}{
		motan.RuntimeNameKey: h.GetName(),
	}
	if h.url != nil {
		info[motan.RuntimeUrlKey] = h.url.ToExtInfo()
	}
	return info
}

const (
	// DefaultMotanMethodConfKey for default motan method conf, when make a http call without a specific motan method
	DefaultMotanMethodConfKey = "http_default_motan_method"
	// DefaultMotanHTTPMethod set a default http method
	DefaultMotanHTTPMethod = "GET"
	// MotanRequestHTTPMethodKey http method key in a motan request attachment
	MotanRequestHTTPMethodKey = "HTTP_Method"

	DefaultRequestTimeout = 1 * time.Second
)

// Initialize http provider
func (h *HTTPProvider) Initialize() {
	timeout := h.url.GetTimeDuration(motan.TimeOutKey, time.Millisecond, DefaultRequestTimeout)
	// http proxy connection keepalive duration, default 0 means unlimited
	keepaliveTimeout := h.url.GetTimeDuration(mhttp.KeepaliveTimeoutKey, time.Millisecond, 0)
	idleConnectionTimeout := h.url.GetTimeDuration(mhttp.IdleConnectionTimeoutKey, time.Millisecond, 5*time.Second)
	h.srvURLMap = make(srvURLMapT)
	urlConf, _ := h.gctx.Config.GetSection("http-service")
	if urlConf != nil {
		for confID, info := range urlConf {
			srvConf := make(srvConfT)
			for methodArrStr, getSrvConf := range info.(map[interface{}]interface{}) {
				methodArr := motan.TrimSplit(methodArrStr.(string), ",")
				for _, method := range methodArr {
					sconf := make(sConfT)
					for k, v := range getSrvConf.(map[interface{}]interface{}) {
						// @TODO gracefully panic when got a conf err, like more %s in URL_FORMAT
						sconf[k.(string)] = v.(string)
					}
					srvConf[method] = sconf
				}
			}
			h.srvURLMap[confID.(string)] = srvConf
		}
	}
	if getHTTPReqMethod, ok := h.url.Parameters["HTTP_REQUEST_METHOD"]; ok {
		h.defaultHTTPMethod = getHTTPReqMethod
	} else {
		h.defaultHTTPMethod = DefaultMotanHTTPMethod
	}
	h.domain = h.url.GetParam(mhttp.DomainKey, "")
	h.locationMatcher = mhttp.NewLocationMatcherFromContext(h.domain, h.gctx)
	h.proxyAddr = h.url.GetParam(mhttp.ProxyAddressKey, "")
	h.proxySchema = h.url.GetParam(mhttp.ProxySchemaKey, "http")
	h.maxConnections = int(h.url.GetPositiveIntValue(mhttp.MaxConnectionsKey, 1024))
	h.enableRewrite = true
	enableRewriteStr := h.url.GetParam(mhttp.EnableRewriteKey, "true")
	if enableRewrite, err := strconv.ParseBool(enableRewriteStr); err != nil {
		vlog.Errorf("%s should be a bool value, but got: %s", mhttp.EnableRewriteKey, enableRewriteStr)
	} else {
		h.enableRewrite = enableRewrite
	}
	h.enableHttpException = false
	enableHttpExceptionStr := h.url.GetParam(mhttp.EnableHttpExceptionKey, "false")
	if enableHttpException, err := strconv.ParseBool(enableHttpExceptionStr); err != nil {
		vlog.Errorf("%s should be a bool value, but got: %s", mhttp.EnableHttpExceptionKey, enableHttpExceptionStr)
	} else {
		h.enableHttpException = enableHttpException
	}
	h.fastClient = &fasthttp.HostClient{
		Name: "motan",
		Addr: h.proxyAddr,
		Dial: func(addr string) (net.Conn, error) {
			if strings.HasPrefix(addr, motan.UnixSockProtocolFlag) {
				return net.DialTimeout("unix", addr[len(motan.UnixSockProtocolFlag):], timeout)
			}
			c, err := fasthttp.DialTimeout(addr, timeout)
			if err != nil {
				return c, err
			}
			return c, nil
		},
		MaxConnDuration:           keepaliveTimeout,
		MaxIdleConnDuration:       idleConnectionTimeout,
		MaxIdemponentCallAttempts: 1, // do not retry for any type of request, by default fasthttp will retry idemponent type request
		MaxConns:                  h.maxConnections,
		ReadTimeout:               timeout,
		WriteTimeout:              timeout,
	}
}

// Destroy a HTTPProvider
func (h *HTTPProvider) Destroy() {
}

// SetSerialization for set a motan.SetSerialization to HTTPProvider
func (h *HTTPProvider) SetSerialization(s motan.Serialization) {}

// SetProxy for HTTPProvider
func (h *HTTPProvider) SetProxy(proxy bool) {}

// SetContext use to set global config to HTTPProvider
func (h *HTTPProvider) SetContext(context *motan.Context) {
	h.gctx = context
}

// rewrite do rewrite
func (h *HTTPProvider) rewrite(httpReq *fasthttp.Request, request motan.Request) (string, error) {
	if h.enableRewrite {
		var query []byte
		// init query string bytes if needed.
		if h.locationMatcher.NeedURLQueryString() {
			query = httpReq.URI().QueryString()
		}
		_, path, ok := h.locationMatcher.Pick(request.GetMethod(), query, true)
		if !ok {
			return "", errors.New("service not found")
		}
		return path, nil
	}
	return request.GetMethod(), nil
}

func buildReqURL(request motan.Request, h *HTTPProvider) (string, string, error) {
	method := request.GetMethod()
	httpReqURLFmt := h.url.Parameters["URL_FORMAT"]
	httpReqMethod := h.defaultHTTPMethod
	// when set a extconf check the specific method conf first,then use the DefaultMotanMethodConfKey conf
	if _, haveExtConf := h.srvURLMap[h.url.Parameters[motan.URLConfKey]]; haveExtConf {
		var specificConf = make(map[string]string, 2)
		if getSpecificConf, ok := h.srvURLMap[h.url.Parameters[motan.URLConfKey]][method]; ok {
			specificConf = getSpecificConf
		} else if getSpecificConf, ok := h.srvURLMap[h.url.Parameters[motan.URLConfKey]][DefaultMotanMethodConfKey]; ok {
			specificConf = getSpecificConf
		}
		if getHTTPReqURL, ok := specificConf["URL_FORMAT"]; ok {
			httpReqURLFmt = getHTTPReqURL
		}
		if getHTTPReqMethod, ok := specificConf["HTTP_REQUEST_METHOD"]; ok {
			httpReqMethod = getHTTPReqMethod
		}
	}
	// when motan request have a http method specific in attachment use this method
	if motanRequestHTTPMethod, ok := request.GetAttachments().Load(MotanRequestHTTPMethodKey); ok {
		httpReqMethod = motanRequestHTTPMethod
	}
	var httpReqURL string
	if count := strings.Count(httpReqURLFmt, "%s"); count > 0 {
		if count > 1 {
			errMsg := "Get err URL_FORMAT: " + httpReqURLFmt
			vlog.Errorln(errMsg)
			return httpReqURL, httpReqMethod, errors.New(errMsg)
		}
		httpReqURL = fmt.Sprintf(httpReqURLFmt, method)
	} else {
		httpReqURL = httpReqURLFmt
	}

	return httpReqURL, httpReqMethod, nil
}

func buildQueryStr(request motan.Request, url *motan.URL, mixVars []string) (res string, err error) {
	paramsTmp := request.GetArguments()

	var buffer bytes.Buffer
	if paramsTmp[0] != nil && len(paramsTmp) > 0 {
		// @if is simple serialization, only have paramsTmp[0]
		vparamsTmp := reflect.ValueOf(paramsTmp[0])

		t := fmt.Sprintf("%s", vparamsTmp.Type())
		buffer.WriteString(mhttp.ProxyRequestIDKey)
		buffer.WriteString("=")
		buffer.WriteString(fmt.Sprintf("%d", request.GetRequestID()))
		switch t {
		case "map[string]string":
			params := paramsTmp[0].(map[string]string)

			if mixVars != nil {
				for _, k := range mixVars {
					if _, contains := params[k]; !contains {
						if value, ok := request.GetAttachments().Load(k); ok {
							params[k] = value
						}
					}
				}
			}

			for k, v := range params {
				buffer.WriteString("&")
				buffer.WriteString(k)
				buffer.WriteString("=")
				buffer.WriteString(URL.QueryEscape(v))
			}
		case "string":
			buffer.WriteString("&")
			buffer.WriteString(URL.QueryEscape(paramsTmp[0].(string)))
		}
	}
	res = buffer.String()
	return res, err
}

func (h *HTTPProvider) DoTransparentProxy(request motan.Request, t int64, ip string) motan.Response {
	resp := mhttp.AcquireHttpMotanResponse()
	resp.RequestID = request.GetRequestID()
	var headerBytes []byte
	var bodyBytes []byte
	toType := []interface{}{&headerBytes, &bodyBytes}
	if err := request.ProcessDeserializable(toType); err != nil {
		fillHttpException(resp, http.StatusBadRequest, t, err.Error())
		return resp
	}
	// acquires new fasthttp Request and Response object
	httpReq := fasthttp.AcquireRequest()
	httpRes := fasthttp.AcquireResponse()
	// only release fast http request. The response will be released when Response is released
	defer fasthttp.ReleaseRequest(httpReq)
	// read http header into Request
	httpReq.Header.Read(bufio.NewReader(bytes.NewReader(headerBytes)))
	//do rewrite
	rewritePath := request.GetMethod()
	var err error
	rewritePath, err = h.rewrite(httpReq, request)
	if err != nil {
		fillHttpException(resp, http.StatusNotFound, t, err.Error())
		return resp
	}
	// set rewrite
	httpReq.URI().SetScheme(h.proxySchema)
	httpReq.URI().SetPath(rewritePath)
	request.GetAttachments().Range(func(k, v string) bool {
		if kk, ok := mhttp.InnerAttachmentsConvertMap[k]; ok {
			httpReq.Header.Set(kk, v)
		}
		return true
	})
	httpReq.Header.Del("Connection")
	if httpReq.Header.Peek(motan.XForwardedFor) == nil {
		httpReq.Header.Set(motan.XForwardedFor, ip)
	}
	if len(bodyBytes) != 0 {
		httpReq.BodyWriter().Write(bodyBytes)
	}
	err = h.fastClient.Do(httpReq, httpRes)
	if err != nil {
		fillHttpException(resp, http.StatusServiceUnavailable, t, err.Error())
		return resp
	}
	if h.enableHttpException && httpRes.StatusCode() >= 400 {
		fillHttpException(resp, httpRes.StatusCode(), t, string(httpRes.Body()))
		return resp
	}
	headerBuffer := &bytes.Buffer{}
	httpRes.Header.Del("Connection")
	httpRes.Header.WriteTo(headerBuffer)
	body := httpRes.Body()
	resp.ProcessTime = (time.Now().UnixNano() - t) / 1e6
	// record the response and release later
	resp.HttpResponse = httpRes
	resp.Value = []interface{}{headerBuffer.Bytes(), body}
	updateUpstreamStatusCode(resp, httpRes.StatusCode())
	return resp
}

// DoProxy deal with Request start from a rpc client
func (h *HTTPProvider) DoProxy(request motan.Request, t int64, ip string) motan.Response {
	resp := mhttp.AcquireHttpMotanResponse()
	resp.RequestID = request.GetRequestID()
	if err := request.ProcessDeserializable(nil); err != nil {
		fillHttpException(resp, http.StatusBadRequest, t, err.Error())
		return resp
	}
	// rpc client call to this server

	// acquires new fasthttp Request and Response object
	httpReq := fasthttp.AcquireRequest()
	httpRes := fasthttp.AcquireResponse()
	// do not release http response
	defer fasthttp.ReleaseRequest(httpReq)
	// convert motan request to fasthttp request
	err := mhttp.MotanRequestToFasthttpRequest(request, httpReq, h.defaultHTTPMethod)
	if err != nil {
		fillHttpException(resp, http.StatusBadRequest, t, err.Error())
		return resp
	}
	rewritePath := request.GetMethod()
	rewritePath, err = h.rewrite(httpReq, request)
	if err != nil {
		fillHttpException(resp, http.StatusNotFound, t, err.Error())
		return resp
	}
	httpReq.URI().SetScheme(h.proxySchema)
	httpReq.URI().SetPath(rewritePath)
	if len(httpReq.Header.Host()) == 0 {
		httpReq.Header.SetHost(h.domain)
	}
	if httpReq.Header.Peek(motan.XForwardedFor) == nil {
		httpReq.Header.Set(motan.XForwardedFor, ip)
	}
	err = h.fastClient.Do(httpReq, httpRes)
	if err != nil {
		fillHttpException(resp, http.StatusServiceUnavailable, t, err.Error())
		return resp
	}
	if h.enableHttpException && httpRes.StatusCode() >= 400 {
		fillHttpException(resp, httpRes.StatusCode(), t, string(httpRes.Body()))
		return resp
	}
	mhttp.FasthttpResponseToMotanResponse(resp, httpRes)
	resp.ProcessTime = (time.Now().UnixNano() - t) / 1e6
	updateUpstreamStatusCode(resp, httpRes.StatusCode())
	return resp
}

// DoFormatURLQuery use ordinary client and parse the format url
func (h *HTTPProvider) DoFormatURLQuery(request motan.Request, t int64, ip string) motan.Response {
	resp := mhttp.AcquireHttpMotanResponse()
	resp.RequestID = request.GetRequestID()
	toType := make([]interface{}, 1)
	if err := request.ProcessDeserializable(toType); err != nil {
		fillHttpException(resp, http.StatusBadRequest, t, err.Error())
		return resp
	}
	httpReqURL, httpReqMethod, err := buildReqURL(request, h)
	if err != nil {
		fillHttpException(resp, http.StatusServiceUnavailable, t, err.Error())
		return resp
	}
	queryStr, err := buildQueryStr(request, h.url, h.mixVars)
	if err != nil {
		fillHttpException(resp, http.StatusServiceUnavailable, t, err.Error())
		return resp
	}
	var reqBody io.Reader
	if httpReqMethod == "GET" {
		httpReqURL = httpReqURL + "?" + queryStr
	} else if httpReqMethod == "POST" {
		data, err := URL.ParseQuery(queryStr)
		if err != nil {
			vlog.Errorf("new HTTP Provider ParseQuery err: %v", err)
		}
		reqBody = strings.NewReader(data.Encode())
	}
	req, err := http.NewRequest(httpReqMethod, httpReqURL, reqBody)
	if err != nil {
		vlog.Errorf("new HTTP Provider NewRequest err: %v", err)
		fillHttpException(resp, http.StatusServiceUnavailable, t, err.Error())
		return resp
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded") //设置后，post参数才可正常传递
	request.GetAttachments().Range(func(k, v string) bool {
		k = strings.Replace(k, "M_", "MOTAN-", -1)
		req.Header.Add(k, v)
		return true
	})
	if req.Header.Get(motan.XForwardedFor) == "" {
		req.Header.Add(motan.XForwardedFor, ip)
	}

	timeout := h.url.GetTimeDuration(motan.TimeOutKey, time.Millisecond, DefaultRequestTimeout)
	c := http.Client{
		Transport: &http.Transport{
			Dial: func(netw, addr string) (net.Conn, error) {
				deadline := time.Now().Add(timeout)
				c, err := net.DialTimeout(netw, addr, timeout)
				if err != nil {
					return nil, err
				}
				c.SetDeadline(deadline)
				return c, nil
			},
		},
	}

	httpResp, err := c.Do(req)
	if err != nil {
		vlog.Errorf("new HTTP Provider Do HTTP Call err: %v", err)
		fillHttpException(resp, http.StatusServiceUnavailable, t, err.Error())
		return resp
	}
	defer httpResp.Body.Close()
	headers := httpResp.Header
	statusCode := httpResp.StatusCode
	body, err := ioutil.ReadAll(httpResp.Body)
	l := len(body)
	if l == 0 {
		vlog.Warningf("server_agent result is empty :%d,%d,%s", statusCode, request.GetRequestID(), httpReqURL)
	}
	resp.ProcessTime = (time.Now().UnixNano() - t) / 1e6
	if err != nil {
		vlog.Errorf("new HTTP Provider Read body err: %v", err)
		resp.Exception = &motan.Exception{ErrCode: statusCode,
			ErrMsg: fmt.Sprintf("%s", err), ErrType: http.StatusServiceUnavailable}
		return resp
	}
	if h.enableHttpException && statusCode >= 400 {
		fillHttpException(resp, statusCode, t, string(body))
		return resp
	}
	request.GetAttachments().Range(func(k, v string) bool {
		resp.SetAttachment(k, v)
		return true
	})
	for k, v := range headers {
		resp.SetAttachment(k, v[0])
	}
	resp.Value = string(body)
	updateUpstreamStatusCode(resp, httpResp.StatusCode)
	return resp
}

// Call for do a motan call through this provider
func (h *HTTPProvider) Call(request motan.Request) motan.Response {
	t := time.Now().UnixNano()
	doTransparentProxy, _ := strconv.ParseBool(request.GetAttachment(mhttp.Proxy))
	ip := ""
	if remoteIP, exist := request.GetAttachments().Load(motan.RemoteIPKey); exist {
		ip = remoteIP
	} else {
		ip = request.GetAttachment(motan.HostKey)
	}
	// Ok here we do transparent http proxy and return
	if doTransparentProxy {
		return h.DoTransparentProxy(request, t, ip)
	}
	if h.proxyAddr != "" {
		return h.DoProxy(request, t, ip)
	}
	return h.DoFormatURLQuery(request, t, ip)
}

// GetName return this provider name
func (h *HTTPProvider) GetName() string {
	return "HTTPProvider"
}

// GetURL return the url that represent for this provider
func (h *HTTPProvider) GetURL() *motan.URL {
	return h.url
}

// SetURL to set a motan to represent for this provider
func (h *HTTPProvider) SetURL(url *motan.URL) {
	h.url = url
}

// GetMixVars return the HTTPProvider mixVars
func (h *HTTPProvider) GetMixVars() []string {
	return h.mixVars
}

// SetMixVars to set HTTPProvider mixVars to this provider
func (h *HTTPProvider) SetMixVars(mixVars []string) {
	h.mixVars = mixVars
}

// IsAvailable to check if this provider is sitll working well
func (h *HTTPProvider) IsAvailable() bool {
	//TODO Provider 是否可用
	return true
}

// SetService to set services to this provider that wich can handle
func (h *HTTPProvider) SetService(s interface{}) {
}

// GetPath return current url path from the provider's url
func (h *HTTPProvider) GetPath() string {
	return h.url.Path
}

func fillExceptionWithCode(resp *motan.MotanResponse, code int, start int64, err error) {
	resp.ProcessTime = (time.Now().UnixNano() - start) / 1e6
	resp.Exception = &motan.Exception{ErrCode: code, ErrMsg: fmt.Sprintf("%s", err), ErrType: code}
}

func fillHttpExceptionWithCode(resp *mhttp.HttpMotanResponse, statusCode int, errType int, start int64, msg string) {
	resp.ProcessTime = (time.Now().UnixNano() - start) / 1e6
	resp.Exception = &motan.Exception{ErrCode: statusCode, ErrMsg: msg, ErrType: errType}
}

func fillHttpException(resp *mhttp.HttpMotanResponse, statusCode int, start int64, msg string) {
	fillHttpExceptionWithCode(resp, statusCode, motan.BizException, start, msg)
}

func fillException(resp *motan.MotanResponse, start int64, err error) {
	fillExceptionWithCode(resp, http.StatusServiceUnavailable, start, err)
}

func updateUpstreamStatusCode(resp *mhttp.HttpMotanResponse, statusCode int) {
	resp.SetAttachment(motan.MetaUpstreamCode, strconv.Itoa(statusCode))
}
