package provider

import (
	"errors"
	"fmt"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	"io"
	"io/ioutil"
	"net/http"
	URL "net/url"
	"reflect"
	"strings"
	"time"
)

type sConfT map[string]string
type srvConfT map[string]sConfT
type srvURLMapT map[string]srvConfT

// HTTPProvider struct
type HTTPProvider struct {
	url        *motan.URL
	httpClient http.Client
	srvURLMap  srvURLMapT
	gctx       *motan.Context
	mixVars    []string
}

const (
	// DefaultMotanMethodConfKey for default motan method conf, when make a http call without a specific motan method
	DefaultMotanMethodConfKey = "http_default_motan_method"
	// DefaultMotanHTTPMethod set a default http method
	DefaultMotanHTTPMethod = "GET"
	// MotanRequestHTTPMethodKey http method key in a motan request attachment
	MotanRequestHTTPMethodKey = "HTTP_Method"
)

// Initialize http provider
func (h *HTTPProvider) Initialize() {
	h.httpClient = http.Client{Timeout: 1 * time.Second}
	h.srvURLMap = make(srvURLMapT)
	urlConf, _ := h.gctx.Config.GetSection("http-service")
	if urlConf != nil {
		for confID, info := range urlConf {
			srvConf := make(srvConfT)
			for methodArrStr, getSrvConf := range info.(map[interface{}]interface{}) {
				methodArr := strings.Split(methodArrStr.(string), ",")
				for _, method := range methodArr {
					sconf := make(sConfT)
					for k, v := range getSrvConf.(map[interface{}]interface{}) {
						// @TODO gracful panic when got a conf err, like more %s in URL_FORMAT
						sconf[k.(string)] = v.(string)
					}
					srvConf[method] = sconf
				}
			}
			h.srvURLMap[confID.(string)] = srvConf
		}
	}
}

// Destroy a HTTPProvider
func (h *HTTPProvider) Destroy() {
}

// SetSerialization for set a motan.SetSerialization to HTTPProvider
func (h *HTTPProvider) SetSerialization(s motan.Serialization) {}

// SetProxy for HTTPProvider
func (h *HTTPProvider) SetProxy(proxy bool) {}

// SetContext use to set globle config to HTTPProvider
func (h *HTTPProvider) SetContext(context *motan.Context) {
	h.gctx = context
}

func buildReqURL(request motan.Request, h *HTTPProvider) (string, string, error) {
	method := request.GetMethod()
	httpReqURLFmt := h.url.Parameters["URL_FORMAT"]
	httpReqMethod := ""
	if getHTTPReqMethod, ok := h.url.Parameters["HTTP_REQUEST_METHOD"]; ok {
		httpReqMethod = getHTTPReqMethod
	} else {
		httpReqMethod = DefaultMotanHTTPMethod
	}
	// when set a extconf check the specific method conf first,then use the DefaultMotanMethodConfKey conf
	if _, haveExtConf := h.srvURLMap[h.url.Parameters[motan.URLConfKey]]; haveExtConf {
		if specificConf, ok := h.srvURLMap[h.url.Parameters[motan.URLConfKey]][method]; ok {
			if getHTTPReqURL, ok := specificConf["URL_FORMAT"]; ok {
				httpReqURLFmt = getHTTPReqURL
			}
			if getHTTPReqMethod, ok := specificConf["HTTP_REQUEST_METHOD"]; ok {
				httpReqMethod = getHTTPReqMethod
			}
		} else if specificConf, ok := h.srvURLMap[h.url.Parameters[motan.URLConfKey]][DefaultMotanMethodConfKey]; ok {
			if getHTTPReqURL, ok := specificConf["URL_FORMAT"]; ok {
				httpReqURLFmt = getHTTPReqURL
			}
			if getHTTPReqMethod, ok := specificConf["HTTP_REQUEST_METHOD"]; ok {
				httpReqMethod = getHTTPReqMethod
			}
		}
	}
	// when motan request have a http method specific in attachment use this method
	if motanRequestHTTPMethod, ok := request.GetAttachments()[MotanRequestHTTPMethodKey]; ok {
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
	if paramsTmp != nil && len(paramsTmp) > 0 {
		// @if is simple, then only have paramsTmp[0]
		// @TODO multi value support
		vparamsTmp := reflect.ValueOf(paramsTmp[0])
		t := fmt.Sprintf("%s", vparamsTmp.Type())
		switch t {
		case "map[string]string":
			params := paramsTmp[0].(map[string]string)

			if mixVars != nil {
				for _, k := range mixVars {
					if _, contains := params[k]; !contains {
						if value, ok := request.GetAttachments()[k]; ok {
							params[k] = value
						}
					}
				}
			}

			start := 1
			for k, v := range params {
				if start == 1 {
					res = k + "=" + URL.QueryEscape(v)
					start++
					continue
				}
				res = res + "&" + k + "=" + URL.QueryEscape(v)
			}
		case "string":
			res = URL.QueryEscape(paramsTmp[0].(string))
		}
	}
	res += "&requestIdFromClient=" + fmt.Sprintf("%d",request.GetRequestID())
	return res, err
}

// Call for do a motan call through this provider
func (h *HTTPProvider) Call(request motan.Request) motan.Response {
	defer func() {
		if err := recover(); err != nil {
			vlog.Errorln("http provider call error! ", err)
		}
	}()
	t := time.Now().UnixNano()
	resp := &motan.MotanResponse{Attachment: make(map[string]string)}
	if err := request.ProcessDeserializable(nil); err != nil {
		fillException(resp, t, err)
		return resp
	}
	resp.RequestID = request.GetRequestID()
	httpReqURL, httpReqMethod, err := buildReqURL(request, h)
	if err != nil {
		fillException(resp, t, err)
		return resp
	}
	//vlog.Infof("HTTPProvider read to call: Method:%s, URL:%s", httpReqMethod, httpReqURL)
	queryStr, err := buildQueryStr(request, h.url, h.mixVars)
	if err != nil {
		fillException(resp, t, err)
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
		fillException(resp, t, err)
		return resp
	}
	for k, v := range request.GetAttachments() {
		k = strings.Replace(k, "M_", "MOTAN-", -1)
		req.Header.Add(k, v)
	}

	ip := ""
	if remoteIP, exist := request.GetAttachments()[motan.RemoteIPKey]; exist {
		ip = remoteIP
	} else {
		ip = request.GetAttachment(motan.HostKey)
	}
	req.Header.Add("x-forwarded-for", ip)

	httpResp, err := h.httpClient.Do(req)
	if err != nil {
		vlog.Errorf("new HTTP Provider Do HTTP Call err: %v", err)
		fillException(resp, t, err)
		return resp
	}
	headers := httpResp.Header
	statusCode := httpResp.StatusCode
	defer httpResp.Body.Close()
	body, err := ioutil.ReadAll(httpResp.Body)
	l := 0
	l = len(body)
	if l == 0 {
		vlog.Warningf("server_agent result is empty.req:%d,%d,%+v,%s,%s\n", statusCode,l,request,httpReqMethod, httpReqURL)
	}
	resp.ProcessTime = int64((time.Now().UnixNano() - t) / 1e6)
	if err != nil {
		vlog.Errorf("new HTTP Provider Read body err: %v", err)
		resp.Exception = &motan.Exception{ErrCode: statusCode,
			ErrMsg: fmt.Sprintf("%s", err), ErrType: http.StatusServiceUnavailable}
		return resp
	}
	for k, v := range request.GetAttachments() {
		resp.SetAttachment(k, v)
	}
	for k, v := range headers {
		resp.SetAttachment(k, v[0])
	}
	resp.Value = string(body)
	return resp
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

func fillException(resp *motan.MotanResponse, start int64, err error) {
	resp.ProcessTime = int64((time.Now().UnixNano() - start) / 1e6)
	resp.Exception = &motan.Exception{ErrCode: http.StatusServiceUnavailable,
		ErrMsg: fmt.Sprintf("%s", err), ErrType: http.StatusServiceUnavailable}
}
