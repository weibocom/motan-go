package provider

import (
	"errors"
	"fmt"
	"net/http"
	URL "net/url"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

// Default options for httpxProvider confs
const (
	ExtConfSection             = "http-service"
	ClientName                 = "weibo-mesh-http"
	DefaultWriteTimeout        = 1 * time.Second
	DefaultReadTimeout         = 1 * time.Second
	DefaultMaxConnsPerHost     = 1024
	DefaultMaxIdleConnDuration = 30 * time.Second
)

// HTTPXProvider struct
type HTTPXProvider struct {
	url        *motan.URL
	httpClient *fasthttp.Client
	srvURLMap  srvURLMapT
	gctx       *motan.Context
	mixVars    []string
}

// Initialize http provider
func (h *HTTPXProvider) Initialize() {
	h.httpClient = &fasthttp.Client{
		Name:                ClientName,
		WriteTimeout:        DefaultWriteTimeout,
		ReadTimeout:         DefaultReadTimeout,
		MaxConnsPerHost:     DefaultMaxConnsPerHost,
		MaxIdleConnDuration: DefaultMaxIdleConnDuration,
	}
	vlog.Infoln("Using fast-http client.")
	h.srvURLMap = make(srvURLMapT)
	urlConf, _ := h.gctx.Config.GetSection(ExtConfSection)
	if urlConf != nil {
		for confID, info := range urlConf {
			srvConf := make(srvConfT)
			for methodArrStr, getSrvConf := range info.(map[interface{}]interface{}) {
				methodArr := motan.TrimSplit(methodArrStr.(string), ",")
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

func (h *HTTPXProvider) buildReqURLX(request motan.Request) (string, string, error) {
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

// Destroy a HTTPXProvider
func (h *HTTPXProvider) Destroy() {
}

// SetSerialization for set a motan.SetSerialization to HTTPXProvider
func (h *HTTPXProvider) SetSerialization(s motan.Serialization) {}

// SetProxy for HTTPXProvider
func (h *HTTPXProvider) SetProxy(proxy bool) {}

// SetContext use to set globle config to HTTPXProvider
func (h *HTTPXProvider) SetContext(context *motan.Context) {
	h.gctx = context
}

// Call for do a motan call through this provider
func (h *HTTPXProvider) Call(request motan.Request) motan.Response {
	t := time.Now().UnixNano()
	resp := &motan.MotanResponse{Attachment: motan.NewStringMap(motan.DefaultAttachmentSize)}
	toType := make([]interface{}, 1)
	if err := request.ProcessDeserializable(toType); err != nil {
		fillException(resp, t, err)
		return resp
	}
	resp.RequestID = request.GetRequestID()
	httpReqURL, httpReqMethod, err := h.buildReqURLX(request)
	if err != nil {
		fillException(resp, t, err)
		return resp
	}
	//vlog.Infof("HTTPXProvider read to call: Method:%s, URL:%s", httpReqMethod, httpReqURL)
	queryStr, err := buildQueryStr(request, h.url, h.mixVars)
	if err != nil {
		fillException(resp, t, err)
		return resp
	}
	req, httpResp := fasthttp.AcquireRequest(), fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(httpResp)
	if httpReqMethod == "GET" {
		httpReqURL = httpReqURL + "?" + queryStr
	} else if httpReqMethod == "POST" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded") //设置后，post参数才可正常传递
		data, err := URL.ParseQuery(queryStr)
		if err != nil {
			vlog.Errorf("new HTTP Provider ParseQuery err: %v", err)
		}
		reqBody := strings.NewReader(data.Encode())
		req.SetBodyStream(reqBody, int(reqBody.Size()))
	}

	req.SetRequestURI(httpReqURL)
	req.Header.SetMethod(httpReqMethod)
	request.GetAttachments().Range(func(k, v string) bool {
		k = strings.Replace(k, "M_", "MOTAN-", -1)
		if k == motan.HostKey {
			return true
		}
		req.Header.Add(k, v)
		return true
	})

	ip := ""
	if remoteIP, exist := request.GetAttachments().Load(motan.RemoteIPKey); exist {
		ip = remoteIP
	} else {
		ip = request.GetAttachment(motan.HostKey)
	}
	req.Header.Add("x-forwarded-for", ip)

	err = h.httpClient.Do(req, httpResp)
	if err != nil {
		vlog.Errorf("new HTTP Provider Do HTTP Call, request:%+v, err: %v", req, err)
		fillException(resp, t, err)
		return resp
	}

	statusCode := httpResp.StatusCode()
	body := httpResp.Body()
	l := len(body)
	if l == 0 {
		vlog.Warningf("server_agent result is empty :%d,%d,%s", statusCode, request.GetRequestID(), httpReqURL)
	}
	resp.ProcessTime = int64((time.Now().UnixNano() - t) / 1e6)
	if err != nil {
		vlog.Errorf("new HTTP Provider Read body err: %v", err)
		resp.Exception = &motan.Exception{ErrCode: statusCode,
			ErrMsg: fmt.Sprintf("%s", err), ErrType: http.StatusServiceUnavailable}
		return resp
	}
	request.GetAttachments().Range(func(k, v string) bool {
		resp.SetAttachment(k, v)
		return true
	})
	httpResp.Header.VisitAll(func(k, v []byte) {
		resp.SetAttachment(string(k), string(v))
	})
	resp.Value = string(body)
	return resp
}

// GetName return this provider name
func (h *HTTPXProvider) GetName() string {
	return "HTTPXProvider"
}

// GetURL return the url that represent for this provider
func (h *HTTPXProvider) GetURL() *motan.URL {
	return h.url
}

// SetURL to set a motan to represent for this provider
func (h *HTTPXProvider) SetURL(url *motan.URL) {
	h.url = url
}

// GetMixVars return the HTTPXProvider mixVars
func (h *HTTPXProvider) GetMixVars() []string {
	return h.mixVars
}

// SetMixVars to set HTTPXProvider mixVars to this provider
func (h *HTTPXProvider) SetMixVars(mixVars []string) {
	h.mixVars = mixVars
}

// IsAvailable to check if this provider is sitll working well
func (h *HTTPXProvider) IsAvailable() bool {
	//TODO Provider 是否可用
	return true
}

// SetService to set services to this provider that wich can handle
func (h *HTTPXProvider) SetService(s interface{}) {
}

// GetPath return current url path from the provider's url
func (h *HTTPXProvider) GetPath() string {
	return h.url.Path
}
