package endpoint

import (
	"bufio"
	"errors"
	"fmt"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	mpro "github.com/weibocom/motan-go/protocol"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	defaultChannelPoolSize      = 3
	defaultRequestTimeout       = 1000 * time.Millisecond
	defaultConnectTimeout       = 1000 * time.Millisecond
	defaultKeepaliveInterval    = 1000 * time.Millisecond
	defaultConnectRetryInterval = 60 * time.Second
	defaultErrorCountThreshold  = 10
	defaultLazyInit             = false
	defaultAsyncInitConnection  atomic.Value
	ErrChannelShutdown          = fmt.Errorf("the channel has been shutdown")
	ErrSendRequestTimeout       = fmt.Errorf("timeout err: send request timeout")
	ErrRecvRequestTimeout       = fmt.Errorf("timeout err: receive request timeout")
	ErrUnsupportedMessage       = fmt.Errorf("stream notify : Unsupported message")

	defaultAsyncResponse = &motan.MotanResponse{Attachment: motan.NewStringMap(motan.DefaultAttachmentSize), RPCContext: &motan.RPCContext{AsyncCall: true}}

	errPanic     = errors.New("panic error")
	v2StreamPool = sync.Pool{New: func() interface{} {
		return &V2Stream{
			recvNotifyCh: make(chan struct{}, 1),
		}
	}}
)

type MotanEndpoint struct {
	url                          *motan.URL
	lock                         sync.Mutex
	channels                     *V2ChannelPool
	destroyed                    bool
	destroyCh                    chan struct{}
	available                    atomic.Value
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

	// for heartbeat requestID
	keepaliveID      uint64
	keepaliveRunning bool
	serialization    motan.Serialization
}

func (m *MotanEndpoint) setAvailable(available bool) {
	m.available.Store(available)
}

func (m *MotanEndpoint) SetSerialization(s motan.Serialization) {
	m.serialization = s
}

func (m *MotanEndpoint) SetProxy(proxy bool) {
	m.proxy = proxy
}

func (m *MotanEndpoint) Initialize() {
	m.destroyCh = make(chan struct{}, 1)
	m.available.Store(false) // init value
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

func (m *MotanEndpoint) Destroy() {
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

func (m *MotanEndpoint) GetRequestTimeout(request motan.Request) time.Duration {
	timeout := m.url.GetMethodPositiveIntValue(request.GetMethod(), request.GetMethodDesc(), motan.TimeOutKey, m.requestTimeoutMillisecond)
	minTimeout := m.minRequestTimeoutMillisecond
	maxTimeout := m.maxRequestTimeoutMillisecond
	if minTimeout == 0 {
		minTimeout = timeout / 2
	}
	if maxTimeout == 0 {
		maxTimeout = timeout * 2
	}
	reqTimeout, _ := strconv.ParseInt(request.GetAttachment(mpro.MTimeout), 10, 64)
	if reqTimeout >= minTimeout && reqTimeout <= maxTimeout {
		timeout = reqTimeout
	}
	return time.Duration(timeout) * time.Millisecond
}

func (m *MotanEndpoint) Call(request motan.Request) motan.Response {
	rc := request.GetRPCContext(true)
	rc.Proxy = m.proxy
	rc.GzipSize = int(m.url.GetIntValue(motan.GzipSizeKey, 0))

	if m.channels == nil {
		vlog.Errorf("motanEndpoint %s error: channels is null", m.url.GetAddressStr())
		m.recordErrAndKeepalive()
		return m.defaultErrMotanResponse(request, "motanEndpoint error: channels is null")
	}
	if rc.AsyncCall {
		rc.Result.StartTime = time.Now().UnixNano()
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
	group := GetRequestGroup(request)
	if group != m.url.Group && m.url.Group != "" {
		request.SetAttachment(mpro.MGroup, m.url.Group)
	}

	var msg *mpro.Message
	msg, err = mpro.ConvertToReqMessage(request, m.serialization)

	if err != nil {
		vlog.Errorf("convert motan request fail! ep: %s, req: %s, err:%s", m.url.GetAddressStr(), motan.GetReqInfo(request), err.Error())
		return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "convert motan request fail!", ErrType: motan.ServiceException})
	}
	if rc.Tc != nil {
		rc.Tc.PutReqSpan(&motan.Span{Name: motan.Convert, Addr: m.GetURL().GetAddressStr(), Time: time.Now()})
	}
	recvMsg, err := channel.Call(msg, deadline, rc)
	if err != nil {
		vlog.Errorf("motanEndpoint call fail. ep:%s, req:%s, msgid:%d, error: %s", m.url.GetAddressStr(), motan.GetReqInfo(request), msg.Header.RequestID, err.Error())
		m.recordErrAndKeepalive()
		return m.defaultErrMotanResponse(request, "channel call error:"+err.Error())
	}
	if rc.AsyncCall {
		return defaultAsyncResponse
	}
	recvMsg.Header.SetProxy(m.proxy)
	recvMsg.Header.RequestID = request.GetRequestID()
	response, err := mpro.ConvertToResponse(recvMsg, m.serialization)
	if rc.Tc != nil {
		rc.Tc.PutResSpan(&motan.Span{Name: motan.Convert, Time: time.Now()})
	}
	if err != nil {
		vlog.Errorf("convert to response fail.ep: %s, req: %s, err:%s", m.url.GetAddressStr(), motan.GetReqInfo(request), err.Error())
		return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "convert response fail!" + err.Error(), ErrType: motan.ServiceException})
	}
	excep := response.GetException()
	if excep != nil && excep.ErrType != motan.BizException {
		m.recordErrAndKeepalive()
	} else {
		// reset errorCount
		m.resetErr()
	}
	if !m.proxy {
		if err = response.ProcessDeserializable(rc.Reply); err != nil {
			return m.defaultErrMotanResponse(request, err.Error())
		}
	}
	response.GetRPCContext(true).RemoteAddr = channel.address
	return response
}

func (m *MotanEndpoint) initChannelPoolWithRetry(factory ConnFactory, config *ChannelConfig, retryInterval time.Duration) {
	defer motan.HandlePanic(nil)
	channels, err := NewV2ChannelPool(m.clientConnection, factory, config, m.serialization, m.lazyInit)
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
					channels, err := NewV2ChannelPool(m.clientConnection, factory, config, m.serialization, m.lazyInit)
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

func (m *MotanEndpoint) recordErrAndKeepalive() {
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

func (m *MotanEndpoint) resetErr() {
	atomic.StoreUint32(&m.errorCount, 0)
}

func (m *MotanEndpoint) keepalive() {
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
			m.keepaliveID++
			if channel, err := m.channels.Get(); err != nil {
				vlog.Infof("[keepalive] failed. url:%s, requestID=%d, err:%s", m.url.GetIdentity(), m.keepaliveID, err.Error())
			} else {
				_, err = channel.Call(mpro.BuildHeartbeat(m.keepaliveID, mpro.Req), defaultRequestTimeout, nil)
				if err == nil {
					m.setAvailable(true)
					m.resetErr()
					vlog.Infof("[keepalive] heartbeat success. url: %s", m.url.GetIdentity())
					return
				}
				vlog.Infof("[keepalive] heartbeat failed. url:%s, requestID=%d, err:%s", m.url.GetIdentity(), m.keepaliveID, err.Error())
			}
		case <-m.destroyCh:
			return
		}
	}
}

func (m *MotanEndpoint) defaultErrMotanResponse(request motan.Request, errMsg string) motan.Response {
	return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{
		ErrCode: 400,
		ErrMsg:  errMsg,
		ErrType: motan.ServiceException,
	})
}

func (m *MotanEndpoint) GetName() string {
	return "motanEndpoint"
}

func (m *MotanEndpoint) GetURL() *motan.URL {
	return m.url
}

func (m *MotanEndpoint) SetURL(url *motan.URL) {
	m.url = url
}

func (m *MotanEndpoint) IsAvailable() bool {
	return m.available.Load().(bool)
}

// ChannelConfig : ChannelConfig
type ChannelConfig struct {
	MaxContentLength int
}

func DefaultConfig() *ChannelConfig {
	return &ChannelConfig{
		MaxContentLength: motan.DefaultMaxContentLength,
	}
}

func VerifyConfig(config *ChannelConfig) error {
	if config.MaxContentLength <= 0 {
		return fmt.Errorf("MaxContentLength of ChannelConfig must be positive")
	}
	return nil
}

type V2Channel struct {
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
	streams    map[uint64]*V2Stream
	streamLock sync.Mutex
	// heartbeat
	heartbeats    map[uint64]*V2Stream
	heartbeatLock sync.Mutex

	// shutdown
	shutdown     bool
	shutdownErr  error
	shutdownCh   chan struct{}
	shutdownLock sync.Mutex
}

type V2Stream struct {
	channel  *V2Channel
	sendMsg  *mpro.Message
	streamId uint64
	// recv msg
	recvMsg      *mpro.Message
	recvNotifyCh chan struct{}
	// timeout
	deadline time.Time

	rc          *motan.RPCContext
	isHeartBeat bool
	timer       *time.Timer
	canRelease  atomic.Value // state indicates whether the stream needs to be recycled by the V2Channel
}

func (s *V2Stream) Reset() {
	// try consume
	select {
	case <-s.recvNotifyCh:
	default:
	}
	s.channel = nil
	s.sendMsg = nil
	s.recvMsg = nil
	s.rc = nil
}

func (s *V2Stream) Send() error {
	if s.timer == nil {
		s.timer = time.NewTimer(s.deadline.Sub(time.Now()))
	} else {
		s.timer.Reset(s.deadline.Sub(time.Now()))
	}
	defer s.timer.Stop()

	s.sendMsg.Encode0()
	if s.rc != nil && s.rc.Tc != nil {
		s.rc.Tc.PutReqSpan(&motan.Span{Name: motan.Encode, Addr: s.channel.address, Time: time.Now()})
	}
	ready := sendReady{message: s.sendMsg}
	select {
	case s.channel.sendCh <- ready:
		// read/write data race
		if s.rc != nil {
			sendTime := time.Now()
			s.rc.RequestSendTime = sendTime
			if s.rc.Tc != nil {
				s.rc.Tc.PutReqSpan(&motan.Span{Name: motan.Send, Addr: s.channel.address, Time: sendTime})
			}
			if s.rc.AsyncCall {
				// only return send success, it can release in V2Stream.notify
				s.canRelease.Store(true)
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
func (s *V2Stream) Recv() (*mpro.Message, error) {
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
		msg := s.recvMsg
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

func (s *V2Stream) notify(msg *mpro.Message, t time.Time) {
	if s.rc != nil {
		s.rc.ResponseReceiveTime = t
		if s.rc.Tc != nil {
			s.rc.Tc.PutResSpan(&motan.Span{Name: motan.Receive, Addr: s.channel.address, Time: t})
			s.rc.Tc.PutResSpan(&motan.Span{Name: motan.Decode, Time: time.Now()})
		}
		if s.rc.AsyncCall {
			defer func() {
				s.RemoveFromChannel()
				releaseV2Stream(s)
			}()
			msg.Header.SetProxy(s.rc.Proxy)
			result := s.rc.Result
			response, err := mpro.ConvertToResponse(msg, s.channel.serialization)
			if err != nil {
				vlog.Errorf("convert to response fail. ep: %s, requestid:%d, err:%s", s.channel.address, msg.Header.RequestID, err.Error())
				result.Error = err
				result.Done <- result
				return
			}
			if err = response.ProcessDeserializable(result.Reply); err != nil {
				result.Error = err
			}
			response.SetProcessTime((time.Now().UnixNano() - result.StartTime) / 1000000)
			if s.rc.Tc != nil {
				s.rc.Tc.PutResSpan(&motan.Span{Name: motan.Convert, Addr: s.channel.address, Time: time.Now()})
			}
			result.Done <- result
			return
		}
	}

	s.recvMsg = msg
	s.recvNotifyCh <- struct{}{}
}

func (s *V2Stream) SetDeadline(deadline time.Duration) {
	s.deadline = time.Now().Add(deadline)
}

func (c *V2Channel) newStream(msg *mpro.Message, rc *motan.RPCContext) (*V2Stream, error) {
	if msg == nil || msg.Header == nil {
		return nil, errors.New("msg is invalid")
	}
	if c.IsClosed() {
		return nil, ErrChannelShutdown
	}

	s := acquireV2Stream()
	s.channel = c
	s.sendMsg = msg
	if s.recvNotifyCh == nil {
		s.recvNotifyCh = make(chan struct{}, 1)
	}
	s.deadline = time.Now().Add(1 * time.Second)
	s.rc = rc
	// RequestID is communication identifier, it is own by channel
	msg.Header.RequestID = GenerateRequestID()
	s.streamId = msg.Header.RequestID
	if msg.Header.IsHeartbeat() {
		c.heartbeatLock.Lock()
		c.heartbeats[msg.Header.RequestID] = s
		c.heartbeatLock.Unlock()
		s.isHeartBeat = true
	} else {
		c.streamLock.Lock()
		c.streams[msg.Header.RequestID] = s
		c.streamLock.Unlock()
		s.isHeartBeat = false
	}
	if rc != nil && rc.AsyncCall { // release by Stream.Notify
		s.canRelease.Store(false)
	} else { // release by Channel self
		s.canRelease.Store(true)
	}
	return s, nil
}

func (s *V2Stream) RemoveFromChannel() bool {
	var exist bool
	if s.isHeartBeat {
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

type sendReady struct {
	message   *mpro.Message // motan2 protocol message
	v1Message []byte        //motan1 protocol, synchronize subsequent if motan1 protocol message optimization
}

func (c *V2Channel) Call(msg *mpro.Message, deadline time.Duration, rc *motan.RPCContext) (*mpro.Message, error) {
	stream, err := c.newStream(msg, rc)
	if err != nil {
		return nil, err
	}
	defer func() {
		if rc == nil || !rc.AsyncCall {
			releaseV2Stream(stream)
		}
	}()

	stream.SetDeadline(deadline)
	if err = stream.Send(); err != nil {
		return nil, err
	}
	if rc != nil && rc.AsyncCall {
		return nil, nil
	}
	return stream.Recv()
}

func (c *V2Channel) IsClosed() bool {
	return c.shutdown
}

func (c *V2Channel) recv() {
	defer motan.HandlePanic(func() {
		c.closeOnErr(errPanic)
	})
	if err := c.recvLoop(); err != nil {
		c.closeOnErr(err)
	}
}

func (c *V2Channel) recvLoop() error {
	decodeBuf := make([]byte, mpro.DefaultBufferSize)
	for {
		res, t, err := mpro.DecodeWithTime(c.bufRead, &decodeBuf, c.config.MaxContentLength)
		if err != nil {
			return err
		}
		//TODO async
		var handleErr error
		if res.Header.IsHeartbeat() {
			handleErr = c.handleHeartbeat(res, t)
		} else {
			handleErr = c.handleMessage(res, t)
		}
		if handleErr != nil {
			return handleErr
		}
	}
}

func (c *V2Channel) send() {
	defer motan.HandlePanic(func() {
		c.closeOnErr(errPanic)
	})
	for {
		select {
		case ready := <-c.sendCh:
			if ready.message != nil {
				// can`t reuse net.Buffers
				// len and cap will be 0 after writev consume, 'out of range panic' will happen when reuse
				var sendBuf net.Buffers = ready.message.GetEncodedBytes()
				// TODO need async?
				c.conn.SetWriteDeadline(time.Now().Add(motan.DefaultWriteTimeout))
				_, err := sendBuf.WriteTo(c.conn)
				ready.message.SetCanRelease()
				if err != nil {
					vlog.Errorf("Failed to write channel. ep: %s, err: %s", c.address, err.Error())
					c.closeOnErr(err)
					return
				}
			}
		case <-c.shutdownCh:
			return
		}
	}
}

func (c *V2Channel) handleHeartbeat(msg *mpro.Message, t time.Time) error {
	c.heartbeatLock.Lock()
	stream := c.heartbeats[msg.Header.RequestID]
	delete(c.heartbeats, msg.Header.RequestID)
	c.heartbeatLock.Unlock()
	if stream == nil {
		vlog.Warningf("handle heartbeat message, missing stream: %d, ep:%s", msg.Header.RequestID, c.address)
	} else {
		stream.notify(msg, t)
	}
	return nil
}

func (c *V2Channel) handleMessage(msg *mpro.Message, t time.Time) error {
	c.streamLock.Lock()
	stream := c.streams[msg.Header.RequestID]
	delete(c.streams, msg.Header.RequestID)
	c.streamLock.Unlock()
	if stream == nil {
		vlog.Warningf("handle recv message, missing stream: %d, ep:%s", msg.Header.RequestID, c.address)
	} else {
		stream.notify(msg, t)
	}
	return nil
}

func (c *V2Channel) closeOnErr(err error) {
	c.shutdownLock.Lock()
	if c.shutdownErr == nil {
		c.shutdownErr = err
	}
	shutdown := c.shutdown
	c.shutdownLock.Unlock()
	if !shutdown { // not normal close
		vlog.Warningf("motan channel will close. ep:%s, err: %s\n", c.address, err.Error())
		c.Close()
	}
}

func (c *V2Channel) Close() error {
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

type ConnFactory func() (net.Conn, error)

type V2ChannelPool struct {
	channels      chan *V2Channel
	channelsLock  sync.Mutex
	factory       ConnFactory
	config        *ChannelConfig
	serialization motan.Serialization
}

func (c *V2ChannelPool) getChannels() chan *V2Channel {
	channels := c.channels
	return channels
}

func (c *V2ChannelPool) Get() (*V2Channel, error) {
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
		channel = buildV2Channel(conn, c.config, c.serialization)
	}
	if err := retV2ChannelPool(channels, channel); err != nil && channel != nil {
		channel.closeOnErr(err)
	}
	if channel == nil {
		return nil, errors.New("channel is nil")
	}
	return channel, nil
}

func retV2ChannelPool(channels chan *V2Channel, channel *V2Channel) (error error) {
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

func (c *V2ChannelPool) Close() error {
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

func NewV2ChannelPool(poolCap int, factory ConnFactory, config *ChannelConfig, serialization motan.Serialization, lazyInit bool) (*V2ChannelPool, error) {
	if poolCap <= 0 {
		return nil, errors.New("invalid capacity settings")
	}
	channelPool := &V2ChannelPool{
		channels:      make(chan *V2Channel, poolCap),
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

			channelPool.channels <- buildV2Channel(conn, config, serialization)
		}
	}
	return channelPool, nil
}

func buildV2Channel(conn net.Conn, config *ChannelConfig, serialization motan.Serialization) *V2Channel {
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
	channel := &V2Channel{
		conn:          conn,
		config:        config,
		bufRead:       bufio.NewReader(conn),
		sendCh:        make(chan sendReady, 256),
		streams:       make(map[uint64]*V2Stream, 64),
		heartbeats:    make(map[uint64]*V2Stream),
		shutdownCh:    make(chan struct{}),
		serialization: serialization,
		address:       conn.RemoteAddr().String(),
	}

	go channel.recv()

	go channel.send()

	return channel
}

func SetMotanEPDefaultAsynInit(ai bool) {
	defaultAsyncInitConnection.Store(ai)
}

func GetDefaultMotanEPAsynInit() bool {
	res := defaultAsyncInitConnection.Load()
	if res == nil {
		return true
	}
	return res.(bool)
}

func acquireV2Stream() *V2Stream {
	return v2StreamPool.Get().(*V2Stream)
}

func releaseV2Stream(stream *V2Stream) {
	if stream != nil {
		if v, ok := stream.canRelease.Load().(bool); ok && v {
			stream.Reset()
			v2StreamPool.Put(stream)
		}
	}
}
