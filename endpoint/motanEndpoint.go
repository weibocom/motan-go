package endpoint

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	mpro "github.com/weibocom/motan-go/protocol"
)

var (
	defaultChannelPoolSize      = 3
	defaultRequestTimeout       = 1000 * time.Millisecond
	defaultConnectTimeout       = 1000 * time.Millisecond
	defaultKeepaliveInterval    = 1000 * time.Millisecond
	defaultConnectRetryInterval = 60 * time.Second
	defaultErrorCountThreshold  = 10
	ErrChannelShutdown          = fmt.Errorf("the channel has been shutdown")
	ErrSendRequestTimeout       = fmt.Errorf("timeout err: send request timeout")
	ErrRecvRequestTimeout       = fmt.Errorf("timeout err: receive request timeout")
	ErrUnsupportedMessage       = fmt.Errorf("stream notify : Unsupported message")

	defaultAsyncResponse = &motan.MotanResponse{Attachment: motan.NewStringMap(motan.DefaultAttachmentSize), RPCContext: &motan.RPCContext{AsyncCall: true}}

	errPanic = errors.New("panic error")
)

type MotanEndpoint struct {
	url                          *motan.URL
	lock                         sync.Mutex
	channels                     *V2ChannelPool
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
	maxContentLength             int

	// for heartbeat requestID
	keepaliveID      uint64
	keepaliveRunning bool
	serialization    motan.Serialization
}

func (m *MotanEndpoint) setAvailable(available bool) {
	m.available = available
}

func (m *MotanEndpoint) SetSerialization(s motan.Serialization) {
	m.serialization = s
}

func (m *MotanEndpoint) SetProxy(proxy bool) {
	m.proxy = proxy
}

func (m *MotanEndpoint) Initialize() {
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
	factory := func() (net.Conn, error) {
		return net.DialTimeout("tcp", m.url.GetAddressStr(), connectTimeout)
	}
	config := &ChannelConfig{MaxContentLength: m.maxContentLength}
	channels, err := NewV2ChannelPool(m.clientConnection, factory, config, m.serialization)
	if err != nil {
		vlog.Errorf("Channel pool init failed. url: %v, err:%s", m.url, err.Error())
		// retry connect
		go func() {
			defer motan.HandlePanic(nil)
			// TODO: retry after 2^n * timeUnit
			ticker := time.NewTicker(connectRetryInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					channels, err := NewV2ChannelPool(m.clientConnection, factory, config, m.serialization)
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
	startTime := time.Now().UnixNano()
	if rc.AsyncCall {
		rc.Result.StartTime = startTime
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
	if excep != nil && excep.ErrCode == 503 {
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
	return response
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
	response := &motan.MotanResponse{
		RequestID:  request.GetRequestID(),
		Attachment: motan.NewStringMap(motan.DefaultAttachmentSize),
		Exception: &motan.Exception{
			ErrCode: 400,
			ErrMsg:  errMsg,
			ErrType: motan.ServiceException,
		},
	}
	return response
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
	return m.available
}

// ChannelConfig : ChannelConfig
type ChannelConfig struct {
	MaxContentLength int
	Serialization    motan.Serialization
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
	channel *V2Channel
	sendMsg *mpro.Message
	// recv msg
	recvMsg      *mpro.Message
	recvNotifyCh chan struct{}
	// timeout
	deadline time.Time

	rc          *motan.RPCContext
	isClose     atomic.Value // bool
	isHeartBeat bool
}

func (s *V2Stream) Send() error {
	timer := time.NewTimer(s.deadline.Sub(time.Now()))
	defer timer.Stop()

	buf := s.sendMsg.Encode()
	if s.rc != nil && s.rc.Tc != nil {
		s.rc.Tc.PutReqSpan(&motan.Span{Name: motan.Encode, Addr: s.channel.address, Time: time.Now()})
	}
	ready := sendReady{data: buf.Bytes()}
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
	case <-timer.C:
		return ErrSendRequestTimeout
	case <-s.channel.shutdownCh:
		return ErrChannelShutdown
	}
}

// Recv sync recv
func (s *V2Stream) Recv() (*mpro.Message, error) {
	defer func() {
		s.Close()
	}()
	timer := time.NewTimer(s.deadline.Sub(time.Now()))
	defer timer.Stop()
	select {
	case <-s.recvNotifyCh:
		msg := s.recvMsg
		if msg == nil {
			return nil, errors.New("recv err: recvMsg is nil")
		}
		return msg, nil
	case <-timer.C:
		return nil, ErrRecvRequestTimeout
	case <-s.channel.shutdownCh:
		return nil, ErrChannelShutdown
	}
}

func (s *V2Stream) notify(msg *mpro.Message, t time.Time) {
	defer func() {
		s.Close()
	}()
	if s.rc != nil {
		s.rc.ResponseReceiveTime = t
		if s.rc.Tc != nil {
			s.rc.Tc.PutResSpan(&motan.Span{Name: motan.Receive, Addr: s.channel.address, Time: t})
			s.rc.Tc.PutResSpan(&motan.Span{Name: motan.Decode, Time: time.Now()})
		}
		if s.rc.AsyncCall {
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
			response.SetProcessTime(int64((time.Now().UnixNano() - result.StartTime) / 1000000))
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

func (c *V2Channel) NewStream(msg *mpro.Message, rc *motan.RPCContext) (*V2Stream, error) {
	if msg == nil || msg.Header == nil {
		return nil, errors.New("msg is invalid")
	}
	if c.IsClosed() {
		return nil, ErrChannelShutdown
	}
	s := &V2Stream{
		channel:      c,
		sendMsg:      msg,
		recvNotifyCh: make(chan struct{}, 1),
		deadline:     time.Now().Add(1 * time.Second),
		rc:           rc,
	}
	s.isClose.Store(false)
	// RequestID is communication identifier, it is own by channel
	msg.Header.RequestID = GenerateRequestID()
	if msg.Header.IsHeartbeat() {
		c.heartbeatLock.Lock()
		c.heartbeats[msg.Header.RequestID] = s
		c.heartbeatLock.Unlock()
		s.isHeartBeat = true
	} else {
		c.streamLock.Lock()
		c.streams[msg.Header.RequestID] = s
		c.streamLock.Unlock()
	}
	return s, nil
}

func (s *V2Stream) Close() {
	if !s.isClose.Load().(bool) {
		if s.isHeartBeat {
			s.channel.heartbeatLock.Lock()
			delete(s.channel.heartbeats, s.sendMsg.Header.RequestID)
			s.channel.heartbeatLock.Unlock()
		} else {
			s.channel.streamLock.Lock()
			delete(s.channel.streams, s.sendMsg.Header.RequestID)
			s.channel.streamLock.Unlock()
		}
		s.isClose.Store(true)
	}
}

type sendReady struct {
	data []byte
}

func (c *V2Channel) Call(msg *mpro.Message, deadline time.Duration, rc *motan.RPCContext) (*mpro.Message, error) {
	stream, err := c.NewStream(msg, rc)
	if err != nil {
		return nil, err
	}
	stream.SetDeadline(deadline)
	if err := stream.Send(); err != nil {
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
	for {
		res, t, err := mpro.DecodeWithTime(c.bufRead, c.config.MaxContentLength)
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
			if ready.data != nil {
				// TODO need async?
				c.conn.SetWriteDeadline(time.Now().Add(motan.DefaultWriteTimeout))
				sent := 0
				for sent < len(ready.data) {
					n, err := c.conn.Write(ready.data[sent:])
					if err != nil {
						vlog.Errorf("Failed to write channel. ep: %s, err: %s", c.address, err.Error())
						c.closeOnErr(err)
						return
					}
					sent += n
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

func NewV2ChannelPool(poolCap int, factory ConnFactory, config *ChannelConfig, serialization motan.Serialization) (*V2ChannelPool, error) {
	if poolCap <= 0 {
		return nil, errors.New("invalid capacity settings")
	}
	channelPool := &V2ChannelPool{
		channels:      make(chan *V2Channel, poolCap),
		factory:       factory,
		config:        config,
		serialization: serialization,
	}
	for i := 0; i < poolCap; i++ {
		conn, err := factory()
		if err != nil {
			channelPool.Close()
			return nil, err
		}
		_ = conn.(*net.TCPConn).SetNoDelay(true)
		channelPool.channels <- buildV2Channel(conn, config, serialization)
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
