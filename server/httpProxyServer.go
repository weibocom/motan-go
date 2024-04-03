package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/weibocom/motan-go/cluster"
	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
	mhttp "github.com/weibocom/motan-go/http"
	"github.com/weibocom/motan-go/log"
	"github.com/weibocom/motan-go/protocol"
)

const (
	HTTPProxyServerName       = "motan"
	DefaultTimeout            = 5 * time.Second
	DefaultKeepaliveTimeout   = 5 * time.Second
	DefaultMaxRequestBodySize = math.MaxInt32
)

const (
	HTTPProxyKeepaliveKey          = "httpProxyKeepalive"
	HTTPProxyKeepaliveTimeoutKey   = "httpProxyKeepaliveTimeout"
	HTTPProxyDefaultDomainKey      = "httpProxyDefaultDomain"
	HTTPProxyTimeoutKey            = "httpProxyTimeout"
	HTTPProxyMaxRequestBodySizeKey = "httpProxyMaxRequestBodySize"
	HTTPProxyEnableKey             = "httpProxyEnable"
)

type HTTPClusterGetter interface {
	GetHTTPCluster(host string) *cluster.HTTPCluster
}

type HTTPProxyServer struct {
	url            *core.URL
	clusterGetter  HTTPClusterGetter
	messageHandler core.MessageHandler
	httpHandler    fasthttp.RequestHandler
	httpServer     *fasthttp.Server
	httpClient     *fasthttp.Client
	deny           []string
	keepalive      bool
	defaultDomain  string
}

func (s *HTTPProxyServer) GetRuntimeInfo() map[string]interface{} {
	info := map[string]interface{}{}
	if s.url != nil {
		info[core.RuntimeUrlKey] = s.url.ToExtInfo()
	}
	info[core.RuntimeDenyKey] = s.deny
	info[core.RuntimeKeepaliveKey] = s.keepalive
	info[core.RuntimeDefaultDomainKey] = s.defaultDomain
	return info
}

func NewHTTPProxyServer(url *core.URL) *HTTPProxyServer {
	return &HTTPProxyServer{url: url}
}

func (s *HTTPProxyServer) Open(block bool, proxy bool, clusterGetter HTTPClusterGetter, messageHandler core.MessageHandler) error {
	os.Unsetenv("http_proxy")
	os.Unsetenv("https_proxy")
	s.clusterGetter = clusterGetter
	s.messageHandler = messageHandler
	s.keepalive, _ = strconv.ParseBool(s.url.GetParam(HTTPProxyKeepaliveKey, "true"))
	s.defaultDomain = s.url.GetParam(HTTPProxyDefaultDomainKey, "")
	s.deny = append(s.deny, "127.0.0.1:"+s.url.GetPortStr())
	s.deny = append(s.deny, "localhost:"+s.url.GetPortStr())
	s.deny = append(s.deny, core.GetLocalIP()+":"+s.url.GetPortStr())
	proxyTimeout := s.url.GetTimeDuration(HTTPProxyTimeoutKey, time.Millisecond, DefaultTimeout)
	keepaliveTimeout := s.url.GetTimeDuration(HTTPProxyKeepaliveTimeoutKey, time.Millisecond, DefaultKeepaliveTimeout)
	s.httpClient = &fasthttp.Client{
		Name: "motan",
		Dial: func(addr string) (net.Conn, error) {
			c, err := fasthttp.DialTimeout(addr, proxyTimeout)
			if err != nil {
				return c, err
			}
			return c, nil
		},
		MaxIdleConnDuration: keepaliveTimeout,
		ReadTimeout:         proxyTimeout,
		WriteTimeout:        proxyTimeout,
	}

	s.httpHandler = func(ctx *fasthttp.RequestCtx) {
		defer core.HandlePanic(func() {
			vlog.Errorf("Http proxy handler for [%s] panic", ctx.Request.URI().String())
			ctx.Response.Header.SetServer(HTTPProxyServerName)
			ctx.SetStatusCode(fasthttp.StatusBadGateway)
			ctx.SetBodyString("err_msg: http proxy panic")
		})
		httpReq := &ctx.Request
		requestPath := httpReq.URI().Path()
		if string(requestPath) == "/proxy_to_rpc" {
			s.doRPC(ctx)
			return
		}
		if ctx.IsConnect() {
			// For https proxy we just proxy it to the real domain
			proxyHost := string(ctx.Request.Host())
			backendConn, err := net.Dial("tcp", proxyHost)
			if err != nil {
				ctx.Response.SetStatusCode(fasthttp.StatusBadGateway)
				return
			}
			ctx.SetStatusCode(fasthttp.StatusOK)
			ctx.Response.SkipBody = true
			ctx.Hijack(func(c net.Conn) {
				wg := &sync.WaitGroup{}
				wg.Add(2)
				transport := func(dst io.WriteCloser, src io.ReadCloser) {
					defer wg.Done()
					defer dst.Close()
					defer src.Close()
					io.Copy(dst, src)
				}
				go transport(backendConn, c)
				go transport(c, backendConn)
				wg.Wait()
			})
			return
		}

		hostAndPort := string(httpReq.Header.Host())
		if strings.Index(hostAndPort, ":") == -1 {
			hostAndPort += ":80"
		}

		host, _, _ := net.SplitHostPort(hostAndPort)
		httpCluster := s.clusterGetter.GetHTTPCluster(host)
		if httpCluster == nil && s.defaultDomain != "" {
			httpReq.Header.SetHost(s.defaultDomain)
			hostAndPort = s.defaultDomain + ":80"
			httpCluster = s.clusterGetter.GetHTTPCluster(s.defaultDomain)
		}
		if httpCluster != nil {
			if service, ok := httpCluster.CanServe(string(ctx.Path())); ok {
				s.doHTTPRpcProxy(ctx, httpCluster, service)
				return
			}
		}
		// no upstream handler found, if direct request the host ip and do not set Host header we should reject it
		if requestPath[0] == '/' {
			for _, d := range s.deny {
				if hostAndPort == d {
					ctx.Response.SetStatusCode(fasthttp.StatusBadRequest)
					return
				}
			}
		}
		// TODO: if the request is form this server itself, we should reject it
		s.doHTTPProxy(ctx)
	}

	maxRequestBodySize := s.url.GetPositiveIntValue(HTTPProxyMaxRequestBodySizeKey, DefaultMaxRequestBodySize)
	s.httpServer = &fasthttp.Server{
		Handler:               s.httpHandler,
		Name:                  HTTPProxyServerName,
		NoDefaultServerHeader: true,
		MaxRequestBodySize:    int(maxRequestBodySize),
	}

	enableProxy, _ := strconv.ParseBool(s.url.GetParam(HTTPProxyEnableKey, "false"))
	if !enableProxy {
		vlog.Infof("http proxy disabled, do not listen http forwarded proxy port")
		return nil
	}

	if httpProxyUnixSockAddr := s.url.GetParam(core.HTTPProxyUnixSockKey, ""); httpProxyUnixSockAddr != "" {
		listener, err := core.ListenUnixSock(httpProxyUnixSockAddr)
		if err != nil {
			vlog.Errorf("listenUnixSock fail. err:%v", err)
			return err
		}

		if block {
			s.httpServer.Serve(listener)
		} else {
			go func() {
				s.httpServer.Serve(listener)
			}()
		}
		return nil
	}

	if block {
		s.httpServer.ListenAndServe(s.url.Host + ":" + s.url.GetPortStr())
	} else {
		go func() {
			s.httpServer.ListenAndServe(s.url.Host + ":" + s.url.GetPortStr())
		}()
	}
	return nil
}

func (s *HTTPProxyServer) GetURL() *core.URL {
	return s.url
}

func (s *HTTPProxyServer) SetURL(url *core.URL) {
	s.url = url
}

func (s *HTTPProxyServer) GetName() string {
	return "httpProxy"
}

func (s *HTTPProxyServer) Destroy() {
}

func (s *HTTPProxyServer) GetHTTPClient() *fasthttp.Client {
	return s.httpClient
}

func (s *HTTPProxyServer) doRPC(ctx *fasthttp.RequestCtx) {
	request := &ctx.Request
	response := &ctx.Response
	service := string(ctx.FormValue("service"))
	method := string(ctx.FormValue("method"))
	if service == "" || method == "" {
		ctx.SetStatusCode(http.StatusBadRequest)
		ctx.SetBodyString(fmt.Sprintf("service:%s or method:%s musn't be empty", service, method))
		return
	}
	group := string(ctx.FormValue("group"))
	argType := string(ctx.FormValue("arg_type"))
	argStr := string(ctx.FormValue("arg"))
	if argType != "string" {
		ctx.SetStatusCode(http.StatusBadRequest)
		ctx.SetBodyString(fmt.Sprintf("arg_type %s not support now", argType))
		return
	}
	motanArgs := []interface{}{argStr}
	motanRequest := &core.MotanRequest{
		RequestID:   endpoint.GenerateRequestID(),
		ServiceName: service,
		Method:      method,
		Arguments:   motanArgs,
	}
	motanRequest.SetAttachment(protocol.MGroup, group)
	motanRequest.SetAttachment(protocol.MPath, service)
	motanRequest.SetAttachment(protocol.MMethod, method)
	motanRequest.SetAttachment(protocol.MProxyProtocol, "motan2")
	request.Header.VisitAll(func(key, value []byte) {
		if len(value) == 0 {
			return
		}
		motanRequest.SetAttachment(string(key), string(value))
	})
	motanResponse := s.messageHandler.Call(motanRequest)
	responseException := motanResponse.GetException()
	if responseException != nil {
		ctx.SetStatusCode(http.StatusBadGateway)
		ctx.SetBodyString(responseException.ErrMsg)
		return
	}
	var responseBody []byte
	err := motanResponse.ProcessDeserializable(&responseBody)
	if err != nil {
		ctx.SetStatusCode(http.StatusBadGateway)
		ctx.SetBodyString("get response body failed: " + err.Error())
		return
	}
	motanResponse.GetAttachments().Range(func(k, v string) bool {
		response.Header.Set(k, v)
		return true
	})
	response.Header.Set("Content-Type", "text/plain")
	ctx.SetBody(motanResponse.GetValue().([]byte))
	return
}

func (s *HTTPProxyServer) doHTTPProxy(ctx *fasthttp.RequestCtx) {
	httpReq := &ctx.Request
	httpRes := &ctx.Response
	start := time.Now()
	if s.keepalive {
		httpReq.Header.Del("Connection")
	}
	err := s.httpClient.Do(httpReq, httpRes)
	if s.keepalive {
		httpRes.Header.Del("Connection")
	}
	if err != nil {
		vlog.Errorf("Proxy request %s by http failed: %s", string(httpReq.RequestURI()), err.Error())
		httpRes.Header.SetServer(HTTPProxyServerName)
		httpRes.Header.SetStatusCode(fasthttp.StatusBadGateway)
	}
	// TODO: configurable
	vlog.Infof("http-proxy %s %s %d %d", httpReq.Host(), httpReq.RequestURI(), httpRes.Header.StatusCode(), time.Now().Sub(start)/1e6)
}

func (s *HTTPProxyServer) doHTTPRpcProxy(ctx *fasthttp.RequestCtx, httpCluster *cluster.HTTPCluster, service string) {
	motanRequest := &core.MotanRequest{}
	motanRequest.ServiceName = service
	motanRequest.Method = string(ctx.Path())
	motanRequest.SetAttachment(mhttp.Proxy, "true")
	motanRequest.SetAttachment(protocol.MPath, service)

	headerBuffer := &bytes.Buffer{}
	// server do the url rewrite
	requestURI := ctx.Request.URI()
	ctx.Request.SetRequestURIBytes(requestURI.RequestURI())
	ctx.Request.Header.WriteTo(headerBuffer)
	headerBytes := headerBuffer.Bytes()
	// no need to copy body, because we hold it until request finished
	bodyBytes := ctx.Request.Body()
	motanRequest.SetArguments([]interface{}{headerBytes, bodyBytes})
	var reply []interface{}
	motanRequest.GetRPCContext(true).Reply = &reply
	motanResponse := httpCluster.Call(motanRequest)
	// exception is a motan internal exception
	if exception := motanResponse.GetException(); exception != nil {
		if exception.ErrCode == core.ENoEndpoints {
			vlog.Warningf("Http rpc proxy to [%s, %s] has no endpoints, try http proxy", string(requestURI.Path()), service)
			s.doHTTPProxy(ctx)
			return
		}
		vlog.Errorf("Http rpc proxy call failed: %s", exception.ErrMsg)
		ctx.Response.Header.SetServer(HTTPProxyServerName)
		ctx.Response.SetStatusCode(fasthttp.StatusBadGateway)
		ctx.Response.SetBodyString("err_msg: " + exception.ErrMsg)
		return
	}
	// we need process deserialize here, maybe the httpCluster should initialize without proxy as a normal client
	// and the serialization proceed by cluster itself
	err := motanResponse.ProcessDeserializable(&reply)
	if err != nil {
		vlog.Errorf("Deserialize rpc response failed: %s", err.Error())
		ctx.Response.Header.SetServer(HTTPProxyServerName)
		ctx.Response.SetStatusCode(fasthttp.StatusBadGateway)
		ctx.Response.SetBodyString("err_msg: " + err.Error())
		return
	}
	if reply[0] != nil {
		ctx.Response.Header.Read(bufio.NewReader(bytes.NewReader(reply[0].([]byte))))
	}
	if reply[1] != nil {
		ctx.Response.BodyWriter().Write(reply[1].([]byte))
	}
}
