package provider

import (
	"errors"
	"fmt"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
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
}

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
	}
	if specificConf, ok := h.srvURLMap[h.url.Parameters[motan.URLConfKey]][method]; ok {
		if getHTTPReqURL, ok := specificConf["URL_FORMAT"]; ok {
			httpReqURLFmt = getHTTPReqURL
		}
		if getHTTPReqMethod, ok := specificConf["HTTP_REQUEST_METHOD"]; ok {
			httpReqMethod = getHTTPReqMethod
		}
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

// Call for do a motan call through this provider
func (h *HTTPProvider) Call(request motan.Request) motan.Response {
	defer func() {
		if err := recover(); err != nil {
			vlog.Errorln("http provider call error! ", err)
		}
	}()
	t := time.Now().UnixNano()
	resp := &motan.MotanResponse{Attachment: make(map[string]string)}
	resp.RequestID = request.GetRequestID()
	httpReqURL, httpReqMethod, err := buildReqURL(request, h)
	if err != nil {
		resp.ProcessTime = int64((time.Now().UnixNano() - t) / 1e6)
		resp.Exception = &motan.Exception{ErrCode: http.StatusServiceUnavailable,
			ErrMsg: fmt.Sprintf("%s", err), ErrType: http.StatusServiceUnavailable}
		return resp
	}
	//vlog.Infof("HTTPProvider read to call: Method:%s, URL:%s", httpReqMethod, httpReqURL)
	queryStr := ""
	if getQueryStr, err := buildQueryStr(request, h.url); err == nil {
		queryStr = getQueryStr
	}
	var reqBody io.Reader
	if httpReqMethod == "GET" {
		httpReqURL = httpReqURL + "?" + queryStr
	} else if httpReqMethod == "POST" {
		data, err := url.ParseQuery(queryStr)
		if err != nil {
			vlog.Errorf("new HTTP Provider ParseQuery err: %v", err)
		}
		reqBody = strings.NewReader(data.Encode())
	}
	req, err := http.NewRequest(httpReqMethod, httpReqURL, reqBody)
	if err != nil {
		vlog.Errorf("new HTTP Provider NewRequest err: %v", err)
	}
	for k, v := range request.GetAttachments() {
		k = strings.Replace(k, "M_", "MOTAN-", -1)
		req.Header.Add(k, v)
	}
	var ip string
	remoteIP, exist := request.GetAttachments()[motan.RemoteIPKey]
	if exist {
		ip = remoteIP
	} else {
		ip = request.GetAttachment(motan.HostKey)
	}
	req.Header.Add("x-forwarded-for", ip)

	httpResp, err := h.httpClient.Do(req)
	if err != nil {
		vlog.Errorf("new HTTP Provider Do HTTP Call err: %v", err)
	}
	headers := httpResp.Header
	statusCode := httpResp.StatusCode
	defer httpResp.Body.Close()
	body, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		vlog.Errorf("new HTTP Provider Read body err: %v", err)
	}
	resp.ProcessTime = int64((time.Now().UnixNano() - t) / 1e6)
	if err != nil {
		//@TODO ErrTYpe
		resp.Exception = &motan.Exception{ErrCode: statusCode, ErrMsg: fmt.Sprintf("%s", err), ErrType: statusCode}
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
