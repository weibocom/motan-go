package provider

import (
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

const (
	HTTPKeyPrefix = "HTTP_"
)

var NeededHTTPConf = []string{"", "URL"}

type HttpProvider struct {
	url        *motan.URL
	httpClient http.Client
}

func (h *HttpProvider) Initialize() {
	h.httpClient = http.Client{Timeout: 1 * time.Second}
}

func (h *HttpProvider) Destroy() {
}

func (h *HttpProvider) SetSerialization(s motan.Serialization) {}

func (h *HttpProvider) SetProxy(proxy bool) {}

func (h *HttpProvider) Call(request motan.Request) motan.Response {
	defer func() {
		if err := recover(); err != nil {
			vlog.Errorln("http provider call error! ", err)
		}
	}()
	t := time.Now().UnixNano()
	httpReqMethod := h.url.Parameters["HTTP_REQUEST_METHOD"]
	httpReqUrl := h.url.Parameters["HTTP_URL"]
	queryStr := ""
	if getQueryStr, err := buildQueryStr(request, h.url); err == nil {
		queryStr = getQueryStr
	}
	var reqBody io.Reader
	if httpReqMethod == "GET" {
		httpReqUrl = httpReqUrl + "?" + queryStr
	} else if httpReqMethod == "POST" {
		data, err := url.ParseQuery(queryStr)
		if err != nil {
			vlog.Errorf("new HTTP Provider ParseQuery err: %v", err)
		}
		reqBody = strings.NewReader(data.Encode())
	}
	req, err := http.NewRequest(httpReqMethod, httpReqUrl, reqBody)
	if err != nil {
		vlog.Errorf("new HTTP Provider NewRequest err: %v", err)
	}
	for k, v := range request.GetAttachments() {
		k = strings.Replace(k, "M_", "MOTAN-", -1)
		req.Header.Add(k, v)
	}
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
	resp := &motan.MotanResponse{Attachment: make(map[string]string)}
	resp.RequestID = request.GetRequestID()
	resp.ProcessTime = int64((time.Now().UnixNano() - t) / 1000000)
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

func (h *HttpProvider) GetName() string {
	return "HttpProvider"
}

func (h *HttpProvider) GetURL() *motan.URL {
	return h.url
}

func (h *HttpProvider) SetURL(url *motan.URL) {
	h.url = url
}

func (h *HttpProvider) IsAvailable() bool {
	//TODO Provider 是否可用
	return true
}

func (h *HttpProvider) SetService(s interface{}) {
}

func (h *HttpProvider) GetPath() string {
	return h.url.Path
}
