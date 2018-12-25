package server

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/weibocom/motan-go/cluster"
	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/http"
	"github.com/weibocom/motan-go/log"
)

const (
	HTTPProxyServerName = "motan"
	DefaultTimeout      = 5 * time.Second
)

type HTTPClusterGetter interface {
	GetHTTPCluster(host string) *cluster.HTTPCluster
}

type HTTPProxyServer struct {
	url           *core.URL
	clusterGetter HTTPClusterGetter
	httpHandler   fasthttp.RequestHandler
	httpServer    *fasthttp.Server
	httpClient    *fasthttp.Client
	deny          []string
	keepalive     bool
}

func NewHTTPProxyServer(url *core.URL) *HTTPProxyServer {
	return &HTTPProxyServer{url: url}
}

func (s *HTTPProxyServer) Open(block bool, proxy bool, clusterGetter HTTPClusterGetter) error {
	os.Unsetenv("http_proxy")
	os.Unsetenv("https_proxy")
	s.clusterGetter = clusterGetter
	s.keepalive, _ = strconv.ParseBool(s.url.GetParam("httpProxyKeepalive", "true"))
	s.deny = append(s.deny, "127.0.0.1:"+s.url.GetPortStr())
	s.deny = append(s.deny, "localhost:"+s.url.GetPortStr())
	s.deny = append(s.deny, core.GetLocalIP()+":"+s.url.GetPortStr())

	// TODO: configurable timeout
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
		defer core.HandlePanic(func() {
			vlog.Errorf("Http proxy handler for [%s] panic", string(ctx.Request.URI().String()))
		})
		httpReq := &ctx.Request
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
		if httpReq.RequestURI()[0] == '/' {
			for _, d := range s.deny {
				if hostAndPort == d {
					ctx.Response.SetStatusCode(fasthttp.StatusBadRequest)
					return
				}
			}
		}

		host, _, _ := net.SplitHostPort(hostAndPort)
		if httpCluster := s.clusterGetter.GetHTTPCluster(host); httpCluster != nil {
			if service, ok := httpCluster.CanServe(string(ctx.Path())); ok {
				s.doHTTPRpcProxy(ctx, httpCluster, service)
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
	motanRequest.SetAttachment(http.Proxy, "true")

	headerBuffer := &bytes.Buffer{}
	headerWriter := bufio.NewWriter(headerBuffer)

	// server do the url rewrite
	requestURI := ctx.Request.URI()
	ctx.Request.SetRequestURIBytes(requestURI.RequestURI())
	ctx.Request.Header.WriteTo(headerWriter)
	headerWriter.Flush()
	headerBytes := headerBuffer.Bytes()
	// no need to copy body, because we hold it until request finished
	bodyBytes := ctx.Request.Body()
	motanRequest.SetArguments([]interface{}{headerBytes, bodyBytes})
	var reply []interface{}
	motanRequest.GetRPCContext(true).Reply = &reply
	motanResponse := httpCluster.Call(motanRequest)
	// exception is a motan internal exception
	// TODO: should we check the motanResponse nil?
	if motanResponse.GetException() != nil {
		vlog.Errorf("Http rpc proxy call failed: %s", motanResponse.GetException().ErrMsg)
		ctx.Response.Header.SetServer(HTTPProxyServerName)
		ctx.Response.Header.SetStatusCode(fasthttp.StatusBadGateway)
		return
	}
	// we need process deserialize here, maybe the httpCluster should initialize without proxy as a normal client
	// and the serialization proceed by cluster itself
	err := motanResponse.ProcessDeserializable(&reply)
	if err != nil {
		vlog.Errorf("Deserialize rpc response failed: %s", err.Error())
		ctx.Response.Header.SetServer(HTTPProxyServerName)
		ctx.Response.Header.SetStatusCode(fasthttp.StatusBadGateway)
		return
	}
	if reply[0] != nil {
		ctx.Response.Header.Read(bufio.NewReader(bytes.NewReader(reply[0].([]byte))))
	}
	if reply[1] != nil {
		ctx.Response.BodyWriter().Write(reply[1].([]byte))
	}
}
