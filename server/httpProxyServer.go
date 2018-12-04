package server

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/weibocom/motan-go/cluster"
	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	HTTPProxyServerName = "motan"
	HTTPProxy           = "HTTP_PROXY"
	DefaultTimeout      = 5 * time.Second
)

type HTTPMessageHandler interface {
	core.MessageHandler
	GetHTTPCluster(host string) *cluster.HTTPCluster
}

type HTTPProxyServer struct {
	url         *core.URL
	mh          HTTPMessageHandler
	httpHandler fasthttp.RequestHandler
	httpServer  *fasthttp.Server
	httpClient  *fasthttp.Client
	deny        []string
}

func NewHTTPProxyServer(url *core.URL) *HTTPProxyServer {
	return &HTTPProxyServer{url: url}
}

func (s *HTTPProxyServer) Open(block bool, proxy bool, handler HTTPMessageHandler) error {
	os.Unsetenv("http_proxy")
	os.Unsetenv("https_proxy")
	s.mh = handler
	s.deny = append(s.deny, "127.0.0.1:"+s.url.GetPortStr())
	s.deny = append(s.deny, "localhost:"+s.url.GetPortStr())
	s.deny = append(s.deny, core.GetLocalIP()+":"+s.url.GetPortStr())

	s.httpClient = &fasthttp.Client{
		Name: "motan",
		Dial: func(addr string) (net.Conn, error) {
			c, err := fasthttp.DialTimeout(addr, DefaultTimeout)
			if err != nil {
				return c, err
			}
			return c, nil
		},
		ReadTimeout:  DefaultTimeout,
		WriteTimeout: DefaultTimeout,
	}

	s.httpHandler = func(ctx *fasthttp.RequestCtx) {
		httpReq := &ctx.Request
		if ctx.IsConnect() {
			// this is for https proxy we just proxy it to the real domain
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
		if httpReq.RequestURI()[0] == '/' {
			for _, d := range s.deny {
				if hostAndPort == d {
					ctx.Response.SetStatusCode(fasthttp.StatusBadRequest)
					return
				}
			}
		}

		host, _, _ := net.SplitHostPort(hostAndPort)
		if hc := s.mh.GetHTTPCluster(host); hc != nil {
			if service, ok := hc.CanServe(string(ctx.Path())); ok {
				s.doHTTPRpcProxy(ctx, host, service)
				return
			}
		}
		s.doHTTPProxy(ctx)
	}

	s.httpServer = &fasthttp.Server{
		Handler:               s.httpHandler,
		Name:                  HTTPProxyServerName,
		NoDefaultServerHeader: true,
	}
	if block {
		s.httpServer.ListenAndServe(":" + s.url.GetPortStr())
	} else {
		go func() {
			s.httpServer.ListenAndServe(":" + s.url.GetPortStr())
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

func (s *HTTPProxyServer) doHTTPProxy(ctx *fasthttp.RequestCtx) {
	httpReq := &ctx.Request
	httpRes := &ctx.Response
	err := s.httpClient.Do(httpReq, httpRes)
	if err != nil {
		vlog.Errorf("Proxy request %s by http failed: %s", string(httpReq.RequestURI()), err.Error())
		httpRes.Header.SetServer(HTTPProxyServerName)
		httpRes.Header.SetStatusCode(fasthttp.StatusBadGateway)
	}
}

func (s *HTTPProxyServer) doHTTPRpcProxy(ctx *fasthttp.RequestCtx, host string, service string) {
	mr := &core.MotanRequest{}
	mr.ServiceName = service
	mr.Method = string(ctx.Path())
	mr.SetAttachment("domain", host)
	mr.SetAttachment(HTTPProxy, "true")

	headerBuffer := bytes.NewBuffer(nil)
	headerWriter := bufio.NewWriter(headerBuffer)

	// Server do the url rewrite
	requestURI := ctx.Request.URI()
	ctx.Request.SetRequestURIBytes(requestURI.RequestURI())
	ctx.Request.Header.WriteTo(headerWriter)
	headerWriter.Flush()
	headerBytes := headerBuffer.Bytes()
	bodyBytes := ctx.Request.Body()

	mr.SetArguments([]interface{}{headerBytes, bodyBytes})
	var reply []interface{}
	mr.GetRPCContext(true).Reply = &reply
	res := s.mh.Call(mr)
	if res.GetException() != nil {
		ctx.Response.Header.SetServer(HTTPProxyServerName)
		ctx.Response.Header.SetStatusCode(res.GetException().ErrCode)
		return
	}
	if reply[0] != nil {
		ctx.Response.Header.Read(bufio.NewReader(bytes.NewReader(reply[0].([]byte))))
	}
	if reply[1] != nil {
		ctx.Response.BodyWriter().Write(reply[1].([]byte))
	}
}
