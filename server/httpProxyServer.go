package server

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/weibocom/motan-go/cluster"
	"github.com/weibocom/motan-go/core"
)

const (
	HTTPProxy      = "HTTP_PROXY"
	DefaultTimeout = 5 * time.Second
)

type HTTPMessageHandler interface {
	core.MessageHandler
	GetHTTPCluster(host string) *cluster.HTTPCluster
}

// HTTPProxyMessageHandler 支持多个域名选择的处理
type HTTPProxyMessageHandler struct {
	hostClusters map[string]*cluster.HTTPCluster
}

func (h *HTTPProxyMessageHandler) GetHTTPCluster(host string) *cluster.HTTPCluster {
	return h.hostClusters[host]
}

func (h *HTTPProxyMessageHandler) AddHTTPCluster(host string, httpCluster *cluster.HTTPCluster) {
	nhcs := make(map[string]*cluster.HTTPCluster, len(h.hostClusters)+1)
	for host, hc := range h.hostClusters {
		nhcs[host] = hc
	}
	nhcs[host] = httpCluster
	h.hostClusters = nhcs
}

func (h *HTTPProxyMessageHandler) Call(request core.Request) (res core.Response) {
	host := request.GetAttachment("Host")
	hc := h.GetHTTPCluster(host)
	return hc.Call(request)
}

func (h *HTTPProxyMessageHandler) AddProvider(p core.Provider) error {
	return nil
}

func (h *HTTPProxyMessageHandler) RmProvider(p core.Provider) {
}

func (h *HTTPProxyMessageHandler) GetProvider(serviceName string) core.Provider {
	return nil
}

type HTTPProxyServer struct {
	url         *core.URL
	mh          HTTPMessageHandler
	httpHandler fasthttp.RequestHandler
	httpServer  *fasthttp.Server
	httpClient  *fasthttp.Client
}

func NewHTTPProxyServer(url *core.URL) *HTTPProxyServer {
	return &HTTPProxyServer{url: url}
}

func (s *HTTPProxyServer) Open(block bool, proxy bool, handler HTTPMessageHandler) error {
	os.Unsetenv("http_proxy")
	os.Unsetenv("https_proxy")
	s.mh = handler

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
		httpReq := ctx.Request
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
				//c.Write(proxyConnectionConnected)
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

		if httpReq.RequestURI()[0] == '/' {
			ctx.Response.SetStatusCode(fasthttp.StatusBadRequest)
			return
		}

		if hc := s.mh.GetHTTPCluster(string(httpReq.Host())); hc != nil {
			if service, ok := hc.CanServe(string(ctx.Path())); ok {
				s.doHTTPRpcProxy(ctx, service)
				return
			}
		}
		s.doHTTPProxy(ctx)
	}

	s.httpServer = &fasthttp.Server{
		Handler:               s.httpHandler,
		Name:                  "motan",
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
		httpRes.Header.SetServer("motan")
		httpRes.Header.SetStatusCode(fasthttp.StatusBadGateway)
	}
}

func (s *HTTPProxyServer) doHTTPRpcProxy(ctx *fasthttp.RequestCtx, service string) {
	mr := &core.MotanRequest{}
	mr.ServiceName = service
	mr.Method = string(ctx.Path())
	mr.SetAttachment("Host", string(ctx.Host()))
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
