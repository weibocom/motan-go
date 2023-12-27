package endpoint

import (
	"bufio"
	"errors"
	"github.com/panjf2000/ants/v2"
	motan "github.com/weibocom/motan-go/core"
	vlog "github.com/weibocom/motan-go/log"
	mpro "github.com/weibocom/motan-go/protocol"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	streamPool = sync.Pool{New: func() interface{} {
		return &Stream{
			recvNotifyCh: make(chan struct{}, 1),
		}
	}}
	handleMsgPool, _ = ants.NewPool(10000)
)

// MotanCommonEndpoint supports motan v1, v2 protocols
type MotanCommonEndpoint struct {
	url                          *motan.URL
	lock                         sync.Mutex
	channels                     *ChannelPool
	destroyed                    bool
	destroyCh                    chan struct{}
	available                    bool
	errorCount                   uint32
	proxy                        bool
	errorCountThreshold          int64
	keepaliveInterval            time.Duration
	requestTimeoutMillisecond    int64
	minRequestTimeoutMillisecond int64
	maxRequestTimeoutMillisecond int64
	clientConnection             int
	lazyInit                     bool
	maxContentLength             int
	heartbeatVersion             int
	gzipSize                     int

	keepaliveRunning bool
	serialization    motan.Serialization

	DefaultVersion int // default encode version
}

func (m *MotanCommonEndpoint) setAvailable(available bool) {
	m.available = available
}

func (m *MotanCommonEndpoint) SetSerialization(s motan.Serialization) {
	m.serialization = s
}

func (m *MotanCommonEndpoint) SetProxy(proxy bool) {
	m.proxy = proxy
}

func (m *MotanCommonEndpoint) Initialize() {
	m.destroyCh = make(chan struct{}, 1)
	connectTimeout := m.url.GetTimeDuration(motan.ConnectTimeoutKey, time.Millisecond, defaultConnectTimeout)
	connectRetryInterval := m.url.GetTimeDuration(motan.ConnectRetryIntervalKey, time.Millisecond, defaultConnectRetryInterval)
	m.errorCountThreshold = m.url.GetIntValue(motan.ErrorCountThresholdKey, int64(defaultErrorCountThreshold))
	m.keepaliveInterval = m.url.GetTimeDuration(motan.KeepaliveIntervalKey, time.Millisecond, defaultKeepaliveInterval)
	m.requestTimeoutMillisecond = m.url.GetPositiveIntValue(motan.TimeOutKey, int64(defaultRequestTimeout/time.Millisecond))
	m.minRequestTimeoutMillisecond, _ = m.url.GetInt(motan.MinTimeOutKey)
	m.maxRequestTimeoutMillisecond, _ = m.url.GetInt(motan.MaxTimeOutKey)
	m.clientConnection = int(m.url.GetPositiveIntValue(motan.ClientConnectionKey, int64(defaultChannelPoolSize)))
	m.maxContentLength = int(m.url.GetPositiveIntValue(motan.MaxContentLength, int64(mpro.DefaultMaxContentLength)))
	m.lazyInit = m.url.GetBoolValue(motan.LazyInit, defaultLazyInit)
	asyncInitConnection := m.url.GetBoolValue(motan.AsyncInitConnection, GetDefaultMotanEPAsynInit())
	m.heartbeatVersion = -1
	m.DefaultVersion = mpro.Version2
	m.gzipSize = int(m.url.GetIntValue(motan.GzipSizeKey, 0))
	factory := func() (net.Conn, error) {
		address := m.url.GetAddressStr()
		if strings.HasPrefix(address, motan.UnixSockProtocolFlag) {
			return net.DialTimeout("unix", address[len(motan.UnixSockProtocolFlag):], connectTimeout)
		}
		return net.DialTimeout("tcp", address, connectTimeout)
	}
	config := &ChannelConfig{MaxContentLength: m.maxContentLength}
	if asyncInitConnection {
		go m.initChannelPoolWithRetry(factory, config, connectRetryInterval)
	} else {
		m.initChannelPoolWithRetry(factory, config, connectRetryInterval)
	}
}

func (m *MotanCommonEndpoint) Destroy() {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.destroyed {
		return
	}
	m.setAvailable(false)
	m.destroyCh <- struct{}{}
	m.destroyed = true
	if m.channels != nil {
		vlog.Infof("motan2 endpoint %s will destroyed", m.url.GetAddressStr())
		m.channels.Close()
	}
}

func (m *MotanCommonEndpoint) GetRequestTimeout(request motan.Request) time.Duration {
	timeout := m.url.GetMethodPositiveIntValue(request.GetMethod(), request.GetMethodDesc(), motan.TimeOutKey, m.requestTimeoutMillisecond)
	minTimeout := m.minRequestTimeoutMillisecond
	maxTimeout := m.maxRequestTimeoutMillisecond
	if minTimeout == 0 {
		minTimeout = timeout / 2
	}
	if maxTimeout == 0 {
		maxTimeout = timeout * 2
	}
	rt := request.GetAttachment(mpro.MTimeout)
	if rt != "" {
		reqTimeout, _ := strconv.ParseInt(rt, 10, 64)
		if reqTimeout >= minTimeout && reqTimeout <= maxTimeout {
			timeout = reqTimeout
		}
	}
	return time.Duration(timeout) * time.Millisecond
}

func (m *MotanCommonEndpoint) Call(request motan.Request) motan.Response {
	rc := request.GetRPCContext(true)
	rc.Proxy = m.proxy
	rc.GzipSize = m.gzipSize

	if m.channels == nil {
		vlog.Errorf("motanEndpoint %s error: channels is null", m.url.GetAddressStr())
		m.recordErrAndKeepalive()
		return m.defaultErrMotanResponse(request, "motanEndpoint error: channels is null")
	}
	// get a channel
	channel, err := m.channels.Get()
	if err != nil {
		vlog.Errorf("motanEndpoint %s error: can not get a channel, msg: %s", m.url.GetAddressStr(), err.Error())
		m.recordErrAndKeepalive()
		return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{
			ErrCode: motan.ENoChannel,
			ErrMsg:  "can not get a channel",
			ErrType: motan.ServiceException,
		})
	}
	deadline := m.GetRequestTimeout(request)
	// do call
	m.fitGroup(request)
	response, err := channel.Call(request, deadline, rc)
	if err != nil {
		vlog.Errorf("motanEndpoint call fail. ep:%s, req:%s, error: %s", m.url.GetAddressStr(), motan.GetReqInfo(request), err.Error())
		m.recordErrAndKeepalive()
		return m.defaultErrMotanResponse(request, "channel call error:"+err.Error())
	}
	excep := response.GetException()
	if excep != nil && excep.ErrType != motan.BizException {
		m.recordErrAndKeepalive()
	} else {
		// reset errorCount
		m.resetErr()
		if m.heartbeatVersion == -1 { // init heartbeat version with first request version
			if _, ok := rc.OriginalMessage.(*mpro.MotanV1Message); ok {
				m.heartbeatVersion = mpro.Version1
			} else {
				m.heartbeatVersion = mpro.Version2
			}
		}
	}
	if !m.proxy {
		if err = response.ProcessDeserializable(rc.Reply); err != nil {
			return m.defaultErrMotanResponse(request, err.Error())
		}
	}
	response.GetRPCContext(true).RemoteAddr = channel.address
	return response
}

// Due to traffic switching, the group of the cluster may not same as the endpoint's
func (m *MotanCommonEndpoint) fitGroup(r motan.Request) {
	if r.GetRPCContext(true).IsMotanV1 { // motan v1 use group key
		group := r.GetAttachment(motan.GroupKey)
		if group != m.url.Group && m.url.Group != "" {
			r.SetAttachment(motan.GroupKey, m.url.Group)
		}
	} else {
		group := GetRequestGroup(r)
		if group != m.url.Group && m.url.Group != "" {
			r.SetAttachment(mpro.MGroup, m.url.Group)
		}
	}
}

func (m *MotanCommonEndpoint) initChannelPoolWithRetry(factory ConnFactory, config *ChannelConfig, retryInterval time.Duration) {
	defer motan.HandlePanic(nil)
	channels, err := NewChannelPool(m.clientConnection, factory, config, m.serialization, m.lazyInit)
	if err != nil {
		vlog.Errorf("Channel pool init failed. url: %v, err:%s", m.url, err.Error())
		// retry connect
		go func() {
			defer motan.HandlePanic(nil)
			// TODO: retry after 2^n * timeUnit
			ticker := time.NewTicker(retryInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					channels, err := NewChannelPool(m.clientConnection, factory, config, m.serialization, m.lazyInit)
					if err == nil {
						m.channels = channels
						m.setAvailable(true)
						vlog.Infof("Channel pool init success. url:%s", m.url.GetAddressStr())
						return
					}
				case <-m.destroyCh:
					return
				}
			}
		}()
	} else {
		m.channels = channels
		m.setAvailable(true)
		vlog.Infof("Channel pool init success. url:%s", m.url.GetAddressStr())
	}
}

func (m *MotanCommonEndpoint) recordErrAndKeepalive() {
	// errorCountThreshold <= 0 means not trigger keepalive
	if m.errorCountThreshold > 0 {
		errCount := atomic.AddUint32(&m.errorCount, 1)
		// ensure trigger keepalive
		if errCount >= uint32(m.errorCountThreshold) {
			m.setAvailable(false)
			vlog.Infoln("Referer disable:" + m.url.GetIdentity())
			go m.keepalive()
		}
	}
}

func (m *MotanCommonEndpoint) resetErr() {
	atomic.StoreUint32(&m.errorCount, 0)
}

func (m *MotanCommonEndpoint) keepalive() {
	m.lock.Lock()
	// if the endpoint has been destroyed, we should not do keepalive
	if m.destroyed {
		m.lock.Unlock()
		return
	}
	// ensure only one keepalive handler
	if m.keepaliveRunning {
		m.lock.Unlock()
		return
	}
	m.keepaliveRunning = true
	m.lock.Unlock()

	defer func() {
		m.lock.Lock()
		m.keepaliveRunning = false
		m.lock.Unlock()
	}()

	defer motan.HandlePanic(nil)
	ticker := time.NewTicker(m.keepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if channel, err := m.channels.Get(); err != nil {
				vlog.Infof("[keepalive] failed. url:%s, err:%s", m.url.GetIdentity(), err.Error())
			} else {
				_, err = channel.HeartBeat(m.heartbeatVersion)
				if err == nil {
					m.setAvailable(true)
					m.resetErr()
					vlog.Infof("[keepalive] heartbeat success. url: %s", m.url.GetIdentity())
					return
				}
				vlog.Infof("[keepalive] heartbeat failed. url:%s, err:%s", m.url.GetIdentity(), err.Error())
			}
		case <-m.destroyCh:
			return
		}
	}
}

func (m *MotanCommonEndpoint) defaultErrMotanResponse(request motan.Request, errMsg string) motan.Response {
	return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{
		ErrCode: 400,
		ErrMsg:  errMsg,
		ErrType: motan.ServiceException,
	})
}

func (m *MotanCommonEndpoint) GetName() string {
	return "motanCommonsEndpoint"
}

func (m *MotanCommonEndpoint) GetURL() *motan.URL {
	return m.url
}

func (m *MotanCommonEndpoint) SetURL(url *motan.URL) {
	m.url = url
}

func (m *MotanCommonEndpoint) IsAvailable() bool {
	return m.available
}

type Channel struct {
	// config
	config        *ChannelConfig
	serialization motan.Serialization
	address       string

	// connection
	conn    net.Conn
	bufRead *bufio.Reader

	// send
	sendCh chan sendReady

	// stream
	streams    map[uint64]*Stream
	streamLock sync.Mutex
	// heartbeat
	heartbeats    map[uint64]*Stream
	heartbeatLock sync.Mutex

	// shutdown
	shutdown     bool
	shutdownErr  error
	shutdownCh   chan struct{}
	shutdownLock sync.Mutex
}

type Stream struct {
	channel          *Channel
	streamId         uint64         // RequestID is communication identifier, it is owned by channel
	req              motan.Request  // for send
	res              motan.Response // for receive
	recvNotifyCh     chan struct{}
	deadline         time.Time // for timeout
	rc               *motan.RPCContext
	isHeartbeat      bool // for heartbeat
	heartbeatVersion int  // for heartbeat
	timer            *time.Timer
	canRelease       atomic.Value // state indicates whether the stream needs to be recycled by the channel
}

func (s *Stream) Reset() {
	// try consume
	select {
	case <-s.recvNotifyCh:
	default:
	}
	s.channel = nil
	s.req = nil
	s.res = nil
	s.rc = nil
}

func (s *Stream) Send() (err error) {
	if s.timer == nil {
		s.timer = time.NewTimer(s.deadline.Sub(time.Now()))
	} else {
		s.timer.Reset(s.deadline.Sub(time.Now()))
	}
	defer s.timer.Stop()

	var bytes []byte
	var msg *mpro.Message
	if s.isHeartbeat {
		if s.heartbeatVersion == mpro.Version1 {
			bytes = mpro.BuildV1HeartbeatReq(s.streamId)
		} else { // motan2 as default heartbeat
			msg = mpro.BuildHeartbeat(s.streamId, mpro.Req)
		}
	} else { // normal request
		// s.rc should not nil while send request
		if _, ok := s.rc.OriginalMessage.(*mpro.MotanV1Message); ok { // encode motan v1
			bytes, err = mpro.EncodeMotanV1Request(s.req, s.streamId)
			if err != nil {
				vlog.Errorf("encode v1 request fail, not contains MotanV1Message. req:%s", motan.GetReqInfo(s.req))
				return err
			}
		} else { // encode motan v2
			msg, err = mpro.ConvertToReqMessage(s.req, s.channel.serialization)
			if err != nil {
				vlog.Errorf("convert motan request fail! ep: %s, req: %s, err:%s", s.channel.address, motan.GetReqInfo(s.req), err.Error())
				return err
			}
			msg.Header.RequestID = s.streamId
			if s.rc.Tc != nil {
				s.rc.Tc.PutReqSpan(&motan.Span{Name: motan.Convert, Addr: s.channel.address, Time: time.Now()})
			}
		}
	}

	ready := sendReady{}
	if msg != nil { // encode v2 message
		msg.Encode0()
		ready.message = msg
	} else {
		ready.v1Message = bytes
	}
	if s.rc != nil && s.rc.Tc != nil {
		s.rc.Tc.PutReqSpan(&motan.Span{Name: motan.Encode, Addr: s.channel.address, Time: time.Now()})
	}
	select {
	case s.channel.sendCh <- ready:
		if s.rc != nil {
			sendTime := time.Now()
			s.rc.RequestSendTime = sendTime
			if s.rc.Tc != nil {
				s.rc.Tc.PutReqSpan(&motan.Span{Name: motan.Send, Addr: s.channel.address, Time: sendTime})
			}
		}
		return nil
	case <-s.timer.C:
		return ErrSendRequestTimeout
	case <-s.channel.shutdownCh:
		return ErrChannelShutdown
	}
}

// Recv sync recv
func (s *Stream) Recv() (motan.Response, error) {
	defer func() {
		// only timeout or shutdown before channel.handleMsg, call RemoveFromChannel will be true,
		// which means stream can release
		if s.RemoveFromChannel() {
			s.canRelease.Store(true)
		}
	}()
	if s.timer == nil {
		s.timer = time.NewTimer(s.deadline.Sub(time.Now()))
	} else {
		s.timer.Reset(s.deadline.Sub(time.Now()))
	}
	defer s.timer.Stop()
	select {
	case <-s.recvNotifyCh:
		msg := s.res
		if msg == nil {
			return nil, errors.New("recv err: recvMsg is nil")
		}
		return msg, nil
	case <-s.timer.C:
		// stream may be referenced by V2Stream.notify, can`t release
		s.canRelease.Store(false)
		return nil, ErrRecvRequestTimeout
	case <-s.channel.shutdownCh:
		// stream may be referenced by V2Stream.notify, can`t release
		s.canRelease.Store(false)
		return nil, ErrChannelShutdown
	}
}

func (s *Stream) notify(msg interface{}, t time.Time) {
	decodeTime := time.Now()
	var res motan.Response
	var v2Msg *mpro.Message
	var err error
	if mres, ok := msg.(*motan.MotanResponse); ok { // response (v1 decoded)
		if s.req != nil {
			mres.RequestID = s.req.GetRequestID()
		}
		res = mres
	} else if v2Msg, ok = msg.(*mpro.Message); ok { // v2 message
		if s.rc != nil {
			v2Msg.Header.SetProxy(s.rc.Proxy)
		}
		if s.req != nil {
			v2Msg.Header.RequestID = s.req.GetRequestID()
		}
		res, err = mpro.ConvertToResponse(v2Msg, s.channel.serialization)
		if err != nil {
			vlog.Errorf("convert to response fail. ep: %s, requestId:%d, err:%s", s.channel.address, v2Msg.Header.RequestID, err.Error())
			res = motan.BuildExceptionResponse(v2Msg.Header.RequestID, &motan.Exception{
				ErrCode: motan.EConvertMsg,
				ErrMsg:  "convert response fail",
				ErrType: motan.ServiceException,
			})
		}
	} else { // unknown message
		vlog.Errorf("stream notify: unsupported msg. msg:%v. ep: %s", msg, s.channel.address)
		err = ErrUnsupportedMessage
		rid := s.streamId // stream id as default rid(for heartbeat request)
		if s.req != nil {
			rid = s.req.GetRequestID()
		}
		res = motan.BuildExceptionResponse(rid, &motan.Exception{
			ErrCode: motan.EUnkonwnMsg,
			ErrMsg:  "receive unsupported msg",
			ErrType: motan.ServiceException,
		})
	}
	if s.rc != nil { // not heartbeat
		s.rc.ResponseReceiveTime = t
		if s.rc.Tc != nil {
			s.rc.Tc.PutResSpan(&motan.Span{Name: motan.Receive, Addr: s.channel.address, Time: t})
			s.rc.Tc.PutResSpan(&motan.Span{Name: motan.Decode, Addr: s.channel.address, Time: decodeTime})
			s.rc.Tc.PutResSpan(&motan.Span{Name: motan.Convert, Addr: s.channel.address, Time: time.Now()})
		}
	}
	s.res = res
	s.recvNotifyCh <- struct{}{}
}

func (s *Stream) SetDeadline(deadline time.Duration) {
	s.deadline = time.Now().Add(deadline)
}

func (c *Channel) newStream(req motan.Request, rc *motan.RPCContext, deadline time.Duration) (*Stream, error) {
	if c.IsClosed() {
		return nil, ErrChannelShutdown
	}
	s := acquireStream()
	s.streamId = GenerateRequestID()
	s.channel = c
	s.isHeartbeat = false
	s.req = req
	s.deadline = time.Now().Add(deadline)
	s.rc = rc
	s.canRelease.Store(true)
	c.streamLock.Lock()
	c.streams[s.streamId] = s
	c.streamLock.Unlock()
	return s, nil
}

func (c *Channel) newHeartbeatStream(heartbeatVersion int) (*Stream, error) {
	if c.IsClosed() {
		return nil, ErrChannelShutdown
	}
	s := acquireStream()
	s.streamId = GenerateRequestID()
	s.channel = c
	s.isHeartbeat = true
	s.heartbeatVersion = heartbeatVersion
	s.deadline = time.Now().Add(defaultRequestTimeout)
	s.canRelease.Store(true)
	c.heartbeatLock.Lock()
	c.heartbeats[s.streamId] = s
	c.heartbeatLock.Unlock()
	return s, nil
}

func (s *Stream) RemoveFromChannel() bool {
	var exist bool
	if s.isHeartbeat {
		s.channel.heartbeatLock.Lock()
		if _, exist = s.channel.heartbeats[s.streamId]; exist {
			delete(s.channel.heartbeats, s.streamId)
		}
		s.channel.heartbeatLock.Unlock()
	} else {
		s.channel.streamLock.Lock()
		if _, exist = s.channel.streams[s.streamId]; exist {
			delete(s.channel.streams, s.streamId)
		}
		s.channel.streamLock.Unlock()
	}
	return exist
}

// Call send request to the server.
//
//	about return: exception in response will record error count, err will not.
func (c *Channel) Call(req motan.Request, deadline time.Duration, rc *motan.RPCContext) (motan.Response, error) {
	stream, err := c.newStream(req, rc, deadline)
	if err != nil {
		return nil, err
	}
	defer releaseStream(stream)

	if err = stream.Send(); err != nil {
		return nil, err
	}
	return stream.Recv()
}

func (c *Channel) HeartBeat(heartbeatVersion int) (motan.Response, error) {
	stream, err := c.newHeartbeatStream(heartbeatVersion)
	if err != nil {
		return nil, err
	}
	defer releaseStream(stream)

	if err = stream.Send(); err != nil {
		return nil, err
	}
	return stream.Recv()
}

func (c *Channel) IsClosed() bool {
	return c.shutdown
}

func (c *Channel) recv() {
	defer motan.HandlePanic(func() {
		c.closeOnErr(errPanic)
	})
	if err := c.recvLoop(); err != nil {
		c.closeOnErr(err)
	}
}

func (c *Channel) recvLoop() error {
	decodeBuf := make([]byte, mpro.DefaultBufferSize)
	for {
		v, err := mpro.CheckMotanVersion(c.bufRead)
		if err != nil {
			return err
		}
		var msg interface{}
		var t time.Time
		if v == mpro.Version1 {
			msg, t, err = mpro.ReadV1Message(c.bufRead, c.config.MaxContentLength)
		} else if v == mpro.Version2 {
			msg, t, err = mpro.DecodeWithTime(c.bufRead, &decodeBuf, c.config.MaxContentLength)
		} else {
			vlog.Warningf("unsupported motan version! version:%d con:%s.", v, c.conn.RemoteAddr().String())
			err = mpro.ErrVersion
		}
		if err != nil {
			return err
		}

		handleMsgPool.Submit(func() {
			c.handleMsg(msg, t)
		})
	}
}

func (c *Channel) handleMsg(msg interface{}, t time.Time) {
	var isHeartbeat bool
	var rid uint64
	if v1msg, ok := msg.(*mpro.MotanV1Message); ok {
		res, err := mpro.DecodeMotanV1Response(v1msg)
		if err != nil {
			vlog.Errorf("decode v1 response fail. v1msg:%v, err:%v", v1msg, err)
		}
		isHeartbeat = mpro.IsV1HeartbeatRes(res)
		rid = v1msg.Rid
		msg = res
	} else if v2msg, ok := msg.(*mpro.Message); ok {
		isHeartbeat = v2msg.Header.IsHeartbeat()
		rid = v2msg.Header.RequestID
	} else {
		//should not here
		vlog.Errorf("unsupported msg:%v", msg)
		return
	}

	var stream *Stream
	if isHeartbeat {
		c.heartbeatLock.Lock()
		stream = c.heartbeats[rid]
		delete(c.heartbeats, rid)
		c.heartbeatLock.Unlock()
	} else {
		c.streamLock.Lock()
		stream = c.streams[rid]
		delete(c.streams, rid)
		c.streamLock.Unlock()
	}
	if stream == nil {
		vlog.Warningf("handle message, missing stream: %d, ep:%s, isHeartbeat:%t", rid, c.address, isHeartbeat)
		return
	}
	stream.notify(msg, t)
}

func (c *Channel) send() {
	defer motan.HandlePanic(func() {
		c.closeOnErr(errPanic)
	})
	for {
		select {
		case ready := <-c.sendCh:
			// can`t reuse net.Buffers
			// len and cap will be 0 after writev, 'out of range panic' will happen when reuse
			var sendBuf net.Buffers
			if ready.message != nil { // motan2
				sendBuf = ready.message.GetEncodedBytes()
			} else {
				sendBuf = [][]byte{ready.v1Message}
			}
			c.conn.SetWriteDeadline(time.Now().Add(motan.DefaultWriteTimeout))
			_, err := sendBuf.WriteTo(c.conn)
			if ready.message != nil {
				// message canRelease condition before reset
				ready.message.SetCanRelease()
			}
			if err != nil {
				vlog.Errorf("Failed to write channel. ep: %s, err: %s", c.address, err.Error())
				c.closeOnErr(err)
				return
			}
		case <-c.shutdownCh:
			return
		}
	}
}

func (c *Channel) closeOnErr(err error) {
	c.shutdownLock.Lock()
	if c.shutdownErr == nil {
		c.shutdownErr = err
	}
	shutdown := c.shutdown
	c.shutdownLock.Unlock()
	if !shutdown { // not normal close
		if err != nil && err.Error() != "EOF" {
			vlog.Warningf("motan channel will close. ep:%s, err: %s\n", c.address, err.Error())
		}
		c.Close()
	}
}

func (c *Channel) Close() error {
	c.shutdownLock.Lock()
	defer c.shutdownLock.Unlock()
	if c.shutdown {
		return nil
	}
	c.shutdown = true
	close(c.shutdownCh)
	c.conn.Close()
	return nil
}

type ChannelPool struct {
	channels      chan *Channel
	channelsLock  sync.Mutex
	factory       ConnFactory
	config        *ChannelConfig
	serialization motan.Serialization
}

func (c *ChannelPool) getChannels() chan *Channel {
	channels := c.channels
	return channels
}

func (c *ChannelPool) Get() (*Channel, error) {
	channels := c.getChannels()
	if channels == nil {
		return nil, errors.New("channels is nil")
	}
	channel, ok := <-channels
	if ok && (channel == nil || channel.IsClosed()) {
		conn, err := c.factory()
		if err != nil {
			vlog.Errorf("create channel failed. err:%s", err.Error())
		} else {
			if c, ok := conn.(*net.TCPConn); ok {
				c.SetNoDelay(true)
			}
		}
		channel = buildChannel(conn, c.config, c.serialization)
	}
	if err := retChannelPool(channels, channel); err != nil && channel != nil {
		channel.closeOnErr(err)
	}
	if channel == nil {
		return nil, errors.New("channel is nil")
	}
	return channel, nil
}

func retChannelPool(channels chan *Channel, channel *Channel) (error error) {
	defer func() {
		if err := recover(); err != nil {
			error = errors.New("ChannelPool has been closed")
		}
	}()
	if channels == nil {
		return errors.New("channels is nil")
	}
	channels <- channel
	return nil
}

func (c *ChannelPool) Close() error {
	c.channelsLock.Lock() // to prevent channels closed many times
	channels := c.channels
	c.channels = nil
	c.factory = nil
	c.config = nil
	c.channelsLock.Unlock()
	if channels == nil {
		return nil
	}
	close(channels)
	for channel := range channels {
		if channel != nil {
			channel.Close()
		}
	}
	return nil
}

func NewChannelPool(poolCap int, factory ConnFactory, config *ChannelConfig, serialization motan.Serialization, lazyInit bool) (*ChannelPool, error) {
	if poolCap <= 0 {
		return nil, errors.New("invalid capacity settings")
	}
	channelPool := &ChannelPool{
		channels:      make(chan *Channel, poolCap),
		factory:       factory,
		config:        config,
		serialization: serialization,
	}
	if lazyInit {
		for i := 0; i < poolCap; i++ {
			//delay logic just push nil into channelPool. when the first request comes in,
			//endpoint will build a connection from factory
			channelPool.channels <- nil
		}
	} else {
		for i := 0; i < poolCap; i++ {
			conn, err := factory()
			if err != nil {
				channelPool.Close()
				return nil, err
			}
			if c, ok := conn.(*net.TCPConn); ok {
				c.SetNoDelay(true)
			}
			channelPool.channels <- buildChannel(conn, config, serialization)
		}
	}
	return channelPool, nil
}

func buildChannel(conn net.Conn, config *ChannelConfig, serialization motan.Serialization) *Channel {
	if conn == nil {
		return nil
	}
	if config == nil {
		config = DefaultConfig()
	}
	if err := VerifyConfig(config); err != nil {
		vlog.Errorf("can not build Channel, ChannelConfig check fail. err:%v", err)
		return nil
	}
	channel := &Channel{
		conn:          conn,
		config:        config,
		bufRead:       bufio.NewReader(conn),
		sendCh:        make(chan sendReady, 256),
		streams:       make(map[uint64]*Stream, 64),
		heartbeats:    make(map[uint64]*Stream),
		shutdownCh:    make(chan struct{}),
		serialization: serialization,
		address:       conn.RemoteAddr().String(),
	}

	go channel.recv()

	go channel.send()

	return channel
}

func acquireStream() *Stream {
	return streamPool.Get().(*Stream)
}

func releaseStream(stream *Stream) {
	if stream != nil {
		if v, ok := stream.canRelease.Load().(bool); ok && v {
			stream.Reset()
			streamPool.Put(stream)
		}
	}
}
