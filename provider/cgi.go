package provider

// CGI RFC: https://datatracker.ietf.org/doc/rfc3875/?include_text=1

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	cgi "github.com/beberlei/fastcgi-serve/fcgiclient"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	// "github.com/yangchenxing/go-nginx-conf-parser"
)

const (
	CGIKeyPrefix   = "CGI_"
	DefaultCGIHost = "127.0.0.1"
	DefaultCGIPort = 9000
	HTTPMethodPOST = "POST"
	HTTPMethodGET  = "GET"
)

var serverEnvironment = map[string]string{"SERVER_SOFTWARE": "Motan / CGI"}
var NeededCGIEnv = []string{"REQUEST_METHOD", "SCRIPT_FILENAME", "DOCUMENT_ROOT"}

type CgiProvider struct {
	url *motan.URL
}

func (c *CgiProvider) Initialize() {
}

func (c *CgiProvider) Destroy() {
}

func (c *CgiProvider) SetSerialization(s motan.Serialization) {}

func (c *CgiProvider) SetProxy(proxy bool) {}

func (c *CgiProvider) Call(request motan.Request) motan.Response {
	defer func() {
		// @TODO if cgi server die, should let server node unavailable
		if err := recover(); err != nil {
			vlog.Errorln("cgi provider call error! ", err)
		}
	}()
	t := time.Now().UnixNano()
	resp := &motan.MotanResponse{Attachment: make(map[string]string)}
	toType := make([]interface{}, 1)
	if err := request.ProcessDeserializable(toType); err != nil {
		fillException(resp, t, err)
		return resp
	}
	env := make(map[string]string)
	reqParams := ""

	for name, value := range serverEnvironment {
		env[name] = value
	}
	cgiKey := ""
	for _, key := range NeededCGIEnv {
		cgiKey = CGIKeyPrefix + key
		if info, ok := c.url.Parameters[cgiKey]; ok {
			env[key] = info
		} else {
			vlog.Infof("NeededCGIEnv %s is not exist\n", cgiKey)
		}
	}

	for k, v := range request.GetAttachments() {
		env["MOTAN_"+k] = v
	}

	if env["REQUEST_METHOD"] == HTTPMethodGET {
		if queryStr, err := buildQueryStr(request, c.url, nil); err == nil {
			env["QUERY_STRING"] = queryStr
		}
	} else if env["REQUEST_METHOD"] == HTTPMethodPOST {
		if getReqParams, err := buildQueryStr(request, c.url, nil); err == nil {
			reqParams = getReqParams
		}
		env["CONTENT_TYPE"] = "application/x-www-form-urlencoded"
		env["CONTENT_LENGTH"] = strconv.Itoa(len(reqParams))
	}

	cgiHost := DefaultCGIHost
	cgiPort := DefaultCGIPort
	if host, ok := c.url.Parameters["CGI_HOST"]; ok {
		cgiHost = host
	}
	if portStr, ok := c.url.Parameters["CGI_PORT"]; ok {
		cgiPort, _ = strconv.Atoi(portStr)
	}
	ccgi, err := cgi.New(cgiHost, cgiPort)
	if err != nil {
		vlog.Errorf("new CGI err: %v", err)
		fillException(resp, t, err)
		return resp
	}
	content, _, err := ccgi.Request(env, reqParams)
	if err != nil {
		vlog.Errorf("CGI Call error: %+v\n", err)
		fillException(resp, t, err)
		return resp
	}

	statusCode, headers, body, err := ParseFastCgiResponse(string(content))
	resp.RequestID = request.GetRequestID()
	resp.ProcessTime = int64((time.Now().UnixNano() - t) / 1000000)
	if err != nil {
		//@TODO ErrTYpe
		resp.Exception = &motan.Exception{ErrCode: statusCode, ErrMsg: fmt.Sprintf("%s", err), ErrType: statusCode}
		return resp
	}
	for k, v := range headers {
		resp.SetAttachment(k, v)
	}
	// resp.Value, _ = MarshalX(body)
	resp.Value = body
	return resp
}

func ParseFastCgiResponse(content string) (int, map[string]string, string, error) {
	var headers map[string]string

	parts := strings.SplitN(content, "\r\n\r\n", 2)

	if len(parts) < 2 {
		return 502, headers, "", errors.New("Cannot parse FastCGI Response")
	}

	headerParts := strings.Split(parts[0], "\r\n")
	headers = make(map[string]string, len(headerParts))
	body := parts[1]
	status := 200

	if strings.HasPrefix(headerParts[0], "Status:") {
		lineParts := strings.SplitN(headerParts[0], " ", 3)
		status, _ = strconv.Atoi(lineParts[1])
	}

	for _, line := range headerParts {
		lineParts := strings.SplitN(line, ":", 2)

		if len(lineParts) < 2 {
			continue
		}

		lineParts[1] = strings.TrimSpace(lineParts[1])

		if lineParts[0] == "Status" {
			continue
		}

		headers[lineParts[0]] = lineParts[1]
	}

	return status, headers, body, nil
}

func (c *CgiProvider) GetName() string {
	return "CgiProvider"
}

func (c *CgiProvider) GetURL() *motan.URL {
	return c.url
}

func (c *CgiProvider) SetURL(url *motan.URL) {
	c.url = url
}

func (c *CgiProvider) IsAvailable() bool {
	//TODO Provider 是否可用
	return true
}

func (c *CgiProvider) SetService(s interface{}) {
}

func (c *CgiProvider) GetPath() string {
	return c.url.Path
}
