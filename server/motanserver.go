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
	URL              *motan.URL
	handler          motan.MessageHandler
	listener         net.Listener
	extFactory       motan.ExtensionFactory
	proxy            bool
	isDestroyed      chan bool
	maxContextLength int

	heartbeatEnabled bool
}

func (m *MotanServer) Open(block bool, proxy bool, handler motan.MessageHandler, extFactory motan.ExtensionFactory) error {
	m.isDestroyed = make(chan bool, 1)
	m.heartbeatEnabled = true

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
	m.maxContextLength = int(m.URL.GetPositiveIntValue(motan.MaxContentLength, motan.DefaultMaxContentLength))
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
	decodeBuf := make([]byte, motan.DefaultDecodeLength)
	for {
		v, err := mpro.CheckMotanVersion(buf)
		if err != nil {
			if err.Error() != "EOF" {
				vlog.Warningf("check motan version fail! con:%s, err:%s.", conn.RemoteAddr().String(), err.Error())
			}
			break
		}
		if v == mpro.Version1 {
			v1Msg, t, err := mpro.ReadV1Message(buf, m.maxContextLength)
			if err != nil {
				vlog.Warningf("decode motan v1 message fail! con:%s, err:%s.", conn.RemoteAddr().String(), err.Error())
				break
			}
			go m.processV1(v1Msg, t, ip, conn)
		} else if v == mpro.Version2 {
			msg, t, err := mpro.DecodeWithTime(buf, &decodeBuf, m.maxContextLength)
			if err != nil {
				vlog.Warningf("decode motan v2 message fail! con:%s, err:%s.", conn.RemoteAddr().String(), err.Error())
				break
			}

			go m.processV2(msg, t, ip, conn)
		} else {
			vlog.Warningf("unsupported motan version! version:%d con:%s", v, conn.RemoteAddr().String())
			break
		}
	}
}

func (m *MotanServer) processV2(msg *mpro.Message, start time.Time, ip string, conn net.Conn) {
	defer motan.HandlePanic(nil)
	msg.Metadata.Store(motan.HostKey, ip)
	msg.Header.SetProxy(m.proxy)
	// TODO request , response reuse
	var mres motan.Response
	var mreq motan.Request
	var res *mpro.Message
	var tc *motan.TraceContext
	lastRequestID := msg.Header.RequestID
	if msg.Header.IsHeartbeat() {
		if !m.heartbeatEnabled {
			conn.Close()
			return
		}
		res = mpro.BuildHeartbeat(msg.Header.RequestID, mpro.Res)
	} else {
		tc = motan.TracePolicy(msg.Header.RequestID, msg.Metadata)
		if tc != nil {
			tc.Addr = ip
			tc.PutReqSpan(&motan.Span{Name: motan.Receive, Time: start})
			tc.PutReqSpan(&motan.Span{Name: motan.Decode, Time: time.Now()})
		}
		serialization := m.extFactory.GetSerialization("", msg.Header.GetSerialize())
		req, err := mpro.ConvertToRequest(msg, serialization)
		if err != nil {
			vlog.Errorf("motan server convert to motan request fail. rid :%d, service: %s, method:%s,err:%s\n", msg.Header.RequestID, msg.Metadata.LoadOrEmpty(mpro.MPath), msg.Metadata.LoadOrEmpty(mpro.MMethod), err.Error())
			res = mpro.BuildExceptionResponse(msg.Header.RequestID, mpro.ExceptionToJSON(&motan.Exception{ErrCode: 500, ErrMsg: "deserialize fail. err:" + err.Error() + " method:" + msg.Metadata.LoadOrEmpty(mpro.MMethod), ErrType: motan.ServiceException}))
		} else {
			mreq = req
			reqCtx := req.GetRPCContext(true)
			reqCtx.ExtFactory = m.extFactory
			reqCtx.RequestReceiveTime = start
			if tc != nil {
				tc.PutReqSpan(&motan.Span{Name: motan.Convert, Time: time.Now()})
				req.GetRPCContext(true).Tc = tc
			}
			callStart := time.Now()
			mres = m.handler.Call(req)
			if tc != nil {
				// clusterFilter end
				tc.PutResSpan(&motan.Span{Name: motan.ClFilter, Time: time.Now()})
			}
			if mres != nil {
				resCtx := mres.GetRPCContext(true)
				resCtx.Proxy = m.proxy
				if mres.GetAttachment(mpro.MProcessTime) == "" {
					mres.SetAttachment(mpro.MProcessTime, strconv.FormatInt(int64(time.Now().Sub(callStart)/1e6), 10))
				}
				res, err = mpro.ConvertToResMessage(mres, serialization)
				if tc != nil {
					tc.PutResSpan(&motan.Span{Name: motan.Convert, Time: time.Now()})
				}
			} else {
				err = errors.New("handler call return nil")
			}

			if err != nil {
				res = mpro.BuildExceptionResponse(msg.Header.RequestID, mpro.ExceptionToJSON(&motan.Exception{ErrCode: 500, ErrMsg: "convert to response fail. err:" + err.Error(), ErrType: motan.ServiceException}))
			}
		}
	}
	// recover the communication identifier
	res.Header.RequestID = lastRequestID
	resBuf := res.Encode()
	// reuse BytesBuffer
	defer motan.ReleaseBytesBuffer(resBuf)
	if tc != nil {
		tc.PutResSpan(&motan.Span{Name: motan.Encode, Time: time.Now()})
	}
	conn.SetWriteDeadline(time.Now().Add(motan.DefaultWriteTimeout))
	_, err := conn.Write(resBuf.Bytes())
	if err != nil {
		vlog.Errorf("connection will close. conn: %s, err:%s", conn.RemoteAddr().String(), err.Error())
		conn.Close()
	}
	resSendTime := time.Now()
	if mreq != nil {
		reqCtx := mreq.GetRPCContext(true)
		reqCtx.ResponseSendTime = resSendTime
	}
	if mres != nil {
		resCtx := mres.GetRPCContext(true)
		resCtx.OnFinish()
	}
	if tc != nil {
		tc.PutResSpan(&motan.Span{Name: motan.Send, Time: resSendTime})
	}
	// 回收message
	mpro.PutMessageBackToPool(msg)
	// 回收request
	if motanReq, ok := mreq.(*motan.MotanRequest); ok {
		motan.PutMotanRequestBackPool(motanReq)
	}
	if motanResp, ok := mres.(*motan.MotanResponse); ok {
		motan.PutMotanResponseBackPool(motanResp)
	}
}

func (m *MotanServer) processV1(msg *mpro.MotanV1Message, start time.Time, ip string, conn net.Conn) {
	defer motan.HandlePanic(nil)
	var res motan.Response
	var result []byte
	var reqCtx *motan.RPCContext
	req, err := mpro.DecodeMotanV1Request(msg)
	if err != nil {
		vlog.Errorf("decode v1 request fail. conn: %s, err:%s", conn.RemoteAddr().String(), err.Error())
		result = mpro.BuildV1ExceptionResponse(msg.Rid, err.Error())
	} else {
		req.SetAttachment(motan.HostKey, ip)
		reqCtx = req.GetRPCContext(true)
		reqCtx.ExtFactory = m.extFactory
		reqCtx.RequestReceiveTime = start
		if mpro.IsV1HeartbeatReq(req) {
			if !m.heartbeatEnabled {
				conn.Close()
				return
			}
			result = mpro.BuildV1HeartbeatRes(req.GetRequestID())
		} else {
			// TraceContext Currently not supported in protocol v1
			callStart := time.Now()
			res = m.handler.Call(req)
			if res != nil {
				resCtx := res.GetRPCContext(true)
				resCtx.Proxy = m.proxy
				if res.GetAttachment(mpro.MProcessTime) == "" {
					res.SetAttachment(mpro.MProcessTime, strconv.FormatInt(int64(time.Now().Sub(callStart)/1e6), 10))
				}
				result, err = mpro.EncodeMotanV1Response(res)
				if err != nil { // not close connection when encode error occurs.
					vlog.Errorf("encode v1 response fail. conn: %s, err:%s, res:%s", conn.RemoteAddr().String(), err.Error(), motan.GetResInfo(res))
					return
				}
			}
		}
	}

	if len(result) > 0 {
		conn.SetWriteDeadline(time.Now().Add(motan.DefaultWriteTimeout))
		_, err = conn.Write(result)
		if err != nil {
			vlog.Errorf("connection will close. conn: %s, err:%s", conn.RemoteAddr().String(), err.Error())
			conn.Close()
		}
	} else {
		vlog.Errorf("process v1 message fail: no result to send. conn: %s, req:%s, res:%s, result:%v, err: %v", conn.RemoteAddr().String(), motan.GetReqInfo(req), motan.GetResInfo(res), result, err)
	}
	if reqCtx != nil {
		reqCtx.ResponseSendTime = time.Now()
	}
	if res != nil {
		resCtx := res.GetRPCContext(true)
		resCtx.OnFinish()
	}
}

// SetHeartbeat true: enable heartbeat, false: disable heartbeat
func (m *MotanServer) SetHeartbeat(enabled bool) {
	m.heartbeatEnabled = enabled
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
