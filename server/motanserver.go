package server

import (
	"bufio"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	"github.com/weibocom/motan-go/metrics"
	mpro "github.com/weibocom/motan-go/protocol"
	"github.com/weibocom/motan-go/registry"
)

var currentConnections int64

var motanServerOnce sync.Once

func incrConnections() {
	atomic.AddInt64(&currentConnections, 1)
}

func decrConnections() {
	atomic.AddInt64(&currentConnections, -1)
}

func getConnections() int64 {
	return atomic.LoadInt64(&currentConnections)
}

type MotanServer struct {
	URL         *motan.URL
	handler     motan.MessageHandler
	listener    net.Listener
	extFactory  motan.ExtensionFactory
	proxy       bool
	isDestroyed chan bool
}

func (m *MotanServer) Open(block bool, proxy bool, handler motan.MessageHandler, extFactory motan.ExtensionFactory) error {
	m.isDestroyed = make(chan bool, 1)

	motanServerOnce.Do(func() {
		metrics.RegisterStatusSampleFunc("motan_server_connection_count", getConnections)
	})

	var lis net.Listener
	if unixSockAddr := m.URL.GetParam(motan.UnixSockKey, ""); unixSockAddr != "" {
		listener, err := motan.ListenUnixSock(unixSockAddr)
		if err != nil {
			vlog.Errorf("listenUnixSock fail. err:%v", err)
			return err
		}
		lis = listener
	} else {
		addr := ":" + strconv.Itoa(int(m.URL.Port))
		if registry.IsAgent(m.URL) {
			addr = m.URL.Host + addr
		}
		lisTmp, err := net.Listen("tcp", addr)
		if err != nil {
			vlog.Errorf("listen port:%d fail. err: %v", m.URL.Port, err)
			return err
		}
		lis = lisTmp
	}

	m.listener = lis
	m.handler = handler
	m.extFactory = extFactory
	m.proxy = proxy
	vlog.Infof("motan server is started. port:%d", m.URL.Port)
	if block {
		m.run()
	} else {
		go m.run()
	}
	return nil
}

func (m *MotanServer) GetMessageHandler() motan.MessageHandler {
	return m.handler
}

func (m *MotanServer) SetMessageHandler(mh motan.MessageHandler) {
	m.handler = mh
}

func (m *MotanServer) GetURL() *motan.URL {
	return m.URL
}

func (m *MotanServer) SetURL(url *motan.URL) {
	m.URL = url
}

func (m *MotanServer) GetName() string {
	return "motan2"
}

func (m *MotanServer) Destroy() {
	err := m.listener.Close()
	if err == nil {
		m.isDestroyed <- true
		vlog.Infof("motan server destroy success.url %v", m.URL)
	} else {
		vlog.Errorf("motan server destroy fail.url %v, err :%s", m.URL, err.Error())
	}
}

func (m *MotanServer) run() {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			select {
			case <-m.isDestroyed:
				vlog.Infof("Motan agent server been Destroyed and stoped.")
				return
			default:
				vlog.Errorf("motan server accept from port %v fail. err:%s", m.listener.Addr(), err.Error())
			}
		} else {
			if c, ok := conn.(*net.TCPConn); ok {
				c.SetNoDelay(true)
				c.SetKeepAlive(true)
			}
			go m.handleConn(conn)
		}
	}
}

func (m *MotanServer) handleConn(conn net.Conn) {
	incrConnections()
	defer decrConnections()
	defer conn.Close()
	defer motan.HandlePanic(nil)
	buf := bufio.NewReader(conn)

	var ip string
	if ta, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		ip = ta.IP.String()
	} else {
		ip = getRemoteIP(conn.RemoteAddr().String())
	}

	for {
		request, t, err := mpro.DecodeWithTime(buf)
		if err != nil {
			if err.Error() != "EOF" {
				vlog.Warningf("decode motan message fail! con:%s, err:%s.", conn.RemoteAddr().String(), err.Error())
			}
			break
		}

		request.Metadata.Store(motan.HostKey, ip)
		var trace *motan.TraceContext
		if !request.Header.IsHeartbeat() {
			trace = motan.TracePolicy(request.Header.RequestID, request.Metadata)
			if trace != nil {
				trace.Addr = ip
				trace.PutReqSpan(&motan.Span{Name: motan.Receive, Time: t})
				trace.PutReqSpan(&motan.Span{Name: motan.Decode, Time: time.Now()})
			}
		}
		go m.processReq(request, trace, conn)
	}
}

func (m *MotanServer) processReq(request *mpro.Message, tc *motan.TraceContext, conn net.Conn) {
	defer motan.HandlePanic(nil)
	request.Header.SetProxy(m.proxy)
	// TODO request , response reuse
	var res *mpro.Message
	lastRequestID := request.Header.RequestID
	if request.Header.IsHeartbeat() {
		res = mpro.BuildHeartbeat(request.Header.RequestID, mpro.Res)
	} else {
		var mres motan.Response
		serialization := m.extFactory.GetSerialization("", request.Header.GetSerialize())
		req, err := mpro.ConvertToRequest(request, serialization)
		if err != nil {
			vlog.Errorf("motan server convert to motan request fail. rid :%d, service: %s, method:%s,err:%s\n", request.Header.RequestID, request.Metadata.LoadOrEmpty(mpro.MPath), request.Metadata.LoadOrEmpty(mpro.MMethod), err.Error())
			res = mpro.BuildExceptionResponse(request.Header.RequestID, mpro.ExceptionToJSON(&motan.Exception{ErrCode: 500, ErrMsg: "deserialize fail. err:" + err.Error() + " method:" + request.Metadata.LoadOrEmpty(mpro.MMethod), ErrType: motan.ServiceException}))
		} else {
			req.GetRPCContext(true).ExtFactory = m.extFactory
			if tc != nil {
				tc.PutReqSpan(&motan.Span{Name: motan.Convert, Time: time.Now()})
				req.GetRPCContext(true).Tc = tc
			}
			start := time.Now()
			mres = m.handler.Call(req)
			if tc != nil {
				// clusterFilter end
				tc.PutResSpan(&motan.Span{Name: motan.ClFilter, Time: time.Now()})
			}
			if mres != nil {
				mres.GetRPCContext(true).Proxy = m.proxy
				if mres.GetAttachment(mpro.MProcessTime) == "" {
					mres.SetAttachment(mpro.MProcessTime, strconv.FormatInt(int64(time.Now().Sub(start)/1e6), 10))
				}
				res, err = mpro.ConvertToResMessage(mres, serialization)
				if tc != nil {
					tc.PutResSpan(&motan.Span{Name: motan.Convert, Time: time.Now()})
				}
			} else {
				err = errors.New("handler call return nil")
			}

			if err != nil {
				res = mpro.BuildExceptionResponse(request.Header.RequestID, mpro.ExceptionToJSON(&motan.Exception{ErrCode: 500, ErrMsg: "convert to response fail. err:" + err.Error(), ErrType: motan.ServiceException}))
			}
		}
	}
	// recover the communication identifier
	res.Header.RequestID = lastRequestID
	resBuf := res.Encode()
	if tc != nil {
		tc.PutResSpan(&motan.Span{Name: motan.Encode, Time: time.Now()})
	}

	conn.SetWriteDeadline(time.Now().Add(motan.DefaultWriteTimeout))
	_, err := conn.Write(resBuf.Bytes())
	if err != nil {
		vlog.Errorf("connection will close. conn: %s, err:%s", conn.RemoteAddr().String(), err.Error())
		conn.Close()
	}
	if tc != nil {
		tc.PutResSpan(&motan.Span{Name: motan.Send, Time: time.Now()})
	}
}

func getRemoteIP(address string) string {
	var ip string
	index := strings.Index(address, ":")
	if index > 0 {
		ip = string(address[:index])
	} else {
		ip = address
	}
	return ip
}
