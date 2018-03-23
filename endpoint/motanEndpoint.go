package endpoint

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	mpro "github.com/weibocom/motan-go/protocol"
)

var (
	defaultChannelPoolSize     = 3
	defaultRequestTimeout      = 1000 * time.Millisecond
	defaultConnectTimeout      = 1000 * time.Millisecond
	defaultKeepaliveInterval   = 10 * time.Second
	defaultErrorCountThreshold = 10
	ErrChannelShutdown         = fmt.Errorf("The channel has been shutdown")
	ErrSendRequestTimeout      = fmt.Errorf("Timeout err: send request timeout")
	ErrRecvRequestTimeout      = fmt.Errorf("Timeout err: receive request timeout")

	defaultAsyncResonse = &motan.MotanResponse{Attachment: make(map[string]string, 0), RPCContext: &motan.RPCContext{AsyncCall: true}}
)

type MotanEndpoint struct {
	url        *motan.URL
	channels   *ChannelPool
	destroyCh  chan struct{}
	available  bool
	errorCount uint32
	proxy      bool

	// for heartbeat requestid
	keepaliveID   uint64
	serialization motan.Serialization
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
	connectTimeout := m.url.GetTimeDuration("connectTimeout", time.Millisecond, defaultConnectTimeout)

	factory := func() (net.Conn, error) {
		return net.DialTimeout("tcp", m.url.GetAddressStr(), connectTimeout)
	}
	channels, err := NewChannelPool(defaultChannelPoolSize, factory, nil, m.serialization)
	if err != nil {
		vlog.Errorf("Channel pool init failed. err:%s\n", err.Error())
		// retry connect
		go func() {
			ticker := time.NewTicker(60 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					channels, err := NewChannelPool(defaultChannelPoolSize, factory, nil, m.serialization)
					if err == nil {
						m.channels = channels
						m.setAvailable(true)
						vlog.Infof("Channel pool init success. url:%s\n", m.url.GetAddressStr())
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
		vlog.Infof("Channel pool init success. url:%s\n", m.url.GetAddressStr())
	}
}

func (m *MotanEndpoint) Destroy() {
	m.setAvailable(false)
	m.destroyCh <- struct{}{}
	if m.channels != nil {
		vlog.Infof("motan2 endpoint %s will destroyed", m.url.GetAddressStr())
		m.channels.Close()
	}
}

func (m *MotanEndpoint) Call(request motan.Request) motan.Response {
	rc := request.GetRPCContext(true)
	rc.Proxy = m.proxy
	rc.GzipSize = int(m.url.GetIntValue(motan.GzipSizeKey, 0))

	if m.channels == nil {
		vlog.Errorf("motanEndpoint %s error: channels is null\n", m.url.GetAddressStr())
		m.recordErrAndKeepalive()
		return m.defaultErrMotanResponse(request, "motanEndpoint error: channels is null")
	}
	startTime := time.Now().UnixNano()
	if rc != nil && rc.AsyncCall {
		rc.Result.StartTime = startTime
	}
	// get a channel
	channel, err := m.channels.Get()
	if err != nil {
		vlog.Errorf("motanEndpoint %s error: can not get a channel, msg: %s\n", m.url.GetAddressStr(), err.Error())
		m.recordErrAndKeepalive()
		return m.defaultErrMotanResponse(request, "can not get a channel")
	}
	// get request timeout
	deadline := m.url.GetTimeDuration("requestTimeout", time.Millisecond, defaultRequestTimeout)

	// do call
	group := GetRequestGroup(request)
	if group != m.url.Group && m.url.Group != "" {
		request.SetAttachment(mpro.MGroup, m.url.Group)
	}

	var msg *mpro.Message
	msg, err = mpro.ConvertToReqMessage(request, m.serialization)

	if err != nil {
		vlog.Errorf("convert motan request fail! ep: %s, req: %s, err:%s\n", m.url.GetAddressStr(), motan.GetReqInfo(request), err.Error())
		return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "convert motan request fail!", ErrType: motan.ServiceException})
	}
	recvMsg, err := channel.Call(msg, deadline, rc)
	if err != nil {
		vlog.Errorf("motanEndpoint call fail. ep:%s, req:%s, msgid:%d, error: %s\n", m.url.GetAddressStr(), motan.GetReqInfo(request), msg.Header.RequestID, err.Error())
		m.recordErrAndKeepalive()
		return m.defaultErrMotanResponse(request, "channel call error:"+err.Error())
	}
	// reset errorCount
	m.resetErr()
	if rc != nil && rc.AsyncCall {
		return defaultAsyncResonse
	}
	recvMsg.Header.SetProxy(m.proxy)
	response, err := mpro.ConvertToResponse(recvMsg, m.serialization)
	if err != nil {
		vlog.Errorf("convert to response fail.ep: %s, req: %s, err:%s\n", m.url.GetAddressStr(), motan.GetReqInfo(request), err.Error())
		return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "convert response fail!" + err.Error(), ErrType: motan.ServiceException})

	}
	response.ProcessDeserializable(rc.Reply)
	response.SetProcessTime(int64((time.Now().UnixNano() - startTime) / 1000000))
	return response
}

func (m *MotanEndpoint) recordErrAndKeepalive() {
	errCount := atomic.AddUint32(&m.errorCount, 1)
	if errCount == uint32(defaultErrorCountThreshold) {
		m.setAvailable(false)
		vlog.Infoln("Referer disable:" + m.url.GetIdentity())
		go m.keepalive()
	}
}

func (m *MotanEndpoint) resetErr() {
	atomic.StoreUint32(&m.errorCount, 0)
}

func (m *MotanEndpoint) keepalive() {
	ticker := time.NewTicker(defaultKeepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.keepaliveID++
			if channel, err := m.channels.Get(); err != nil {
				vlog.Infof("[keepalive] failed. url:%s, requestID=%d, err:%s\n", m.url.GetIdentity(), m.keepaliveID, err.Error())
			} else {
				_, err = channel.Call(mpro.BuildHeartbeat(m.keepaliveID, mpro.Req), defaultRequestTimeout, nil)
				if err == nil {
					m.setAvailable(true)
					vlog.Infof("[keepalive] heartbeat success. url: %s\n", m.url.GetIdentity())
					return
				}
				vlog.Infof("[keepalive] heartbeat failed. url:%s, requestID=%d, err:%s\n", m.url.GetIdentity(), m.keepaliveID, err.Error())
			}
		case <-m.destroyCh:
			return
		}
	}
}

func (m *MotanEndpoint) defaultErrMotanResponse(request motan.Request, errMsg string) motan.Response {
	response := &motan.MotanResponse{
		RequestID:  request.GetRequestID(),
		Attachment: make(map[string]string),
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

// Config : Config
type Config struct {
	RequestTimeout time.Duration
}

func DefaultConfig() *Config {
	return &Config{
		RequestTimeout: defaultRequestTimeout,
	}
}

func VerifyConfig(config *Config) error {
	if config.RequestTimeout <= 0 {
		return fmt.Errorf("RequestTimeout interval must be positive")
	}
	return nil
}

type Channel struct {
	// config
	config        *Config
	serialization motan.Serialization
	address       string

	// connection
	conn    io.ReadWriteCloser
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
	channel *Channel
	sendMsg *mpro.Message
	// recv msg
	recvMsg      *mpro.Message
	recvNotifyCh chan struct{}
	// timeout
	deadline time.Time

	rc          *motan.RPCContext
	isClose     bool
	isHeartBeat bool
}

func (s *Stream) Send() error {
	timer := time.NewTimer(s.deadline.Sub(time.Now()))
	defer timer.Stop()

	buf := s.sendMsg.Encode()

	ready := sendReady{data: buf.Bytes()}
	select {
	case s.channel.sendCh <- ready:
		return nil
	case <-timer.C:
		return ErrSendRequestTimeout
	case <-s.channel.shutdownCh:
		return ErrChannelShutdown
	}
}

// Recv sync recv
func (s *Stream) Recv() (*mpro.Message, error) {
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

func (s *Stream) notify(msg *mpro.Message) {
	defer func() {
		s.Close()
	}()
	if s.rc != nil && s.rc.AsyncCall {
		msg.Header.SetProxy(s.rc.Proxy)
		result := s.rc.Result
		response, err := mpro.ConvertToResponse(msg, s.channel.serialization)
		if err != nil {
			vlog.Errorf("convert to response fail. ep: %s, requestid:%d, err:%s\n", s.channel.address, msg.Header.RequestID, err.Error())
			result.Error = err
			result.Done <- result
			return
		}
		response.ProcessDeserializable(result.Reply)
		response.SetProcessTime(int64((time.Now().UnixNano() - result.StartTime) / 1000000))
		result.Done <- result
		return
	}
	s.recvMsg = msg
	s.recvNotifyCh <- struct{}{}
}

func (s *Stream) SetDeadline(deadline time.Duration) {
	s.deadline = time.Now().Add(deadline)
}

func (c *Channel) NewStream(msg *mpro.Message, rc *motan.RPCContext) (*Stream, error) {
	if msg == nil || msg.Header == nil {
		return nil, errors.New("msg is invalid")
	}
	if c.IsClosed() {
		return nil, ErrChannelShutdown
	}
	s := &Stream{
		channel:      c,
		sendMsg:      msg,
		recvNotifyCh: make(chan struct{}, 1),
		deadline:     time.Now().Add(1 * time.Second),
		rc:           rc,
	}
	if msg.Header.RequestID == 0 {
		msg.Header.RequestID = GenerateRequestID()
	}

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

func (s *Stream) Close() {
	if !s.isClose {
		if s.isHeartBeat {
			s.channel.heartbeatLock.Lock()
			delete(s.channel.heartbeats, s.sendMsg.Header.RequestID)
			s.channel.heartbeatLock.Unlock()
		} else {
			s.channel.streamLock.Lock()
			delete(s.channel.streams, s.sendMsg.Header.RequestID)
			s.channel.streamLock.Unlock()
		}
		s.isClose = true
	}
}

type sendReady struct {
	data []byte
}

func (c *Channel) Call(msg *mpro.Message, deadline time.Duration, rc *motan.RPCContext) (*mpro.Message, error) {
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

func (c *Channel) IsClosed() bool {
	return c.shutdown
}

func (c *Channel) recv() {
	if err := c.recvLoop(); err != nil {
		c.closeOnErr(err)
	}
}

func (c *Channel) recvLoop() error {
	for {
		res, err := mpro.Decode(c.bufRead)
		if err != nil {
			return err
		}
		//TODO async
		var handleErr error
		if res.Header.IsHeartbeat() {
			handleErr = c.handleHeartbeat(res)
		} else {
			handleErr = c.handleMessage(res)
		}
		if handleErr != nil {
			return handleErr
		}
	}
}

func (c *Channel) send() {
	for {
		select {
		case ready := <-c.sendCh:
			if ready.data != nil {
				// TODO need async?
				sent := 0
				for sent < len(ready.data) {
					n, err := c.conn.Write(ready.data[sent:])
					if err != nil {
						vlog.Errorf("Failed to write channel. ep: %s, err: %s\n", c.address, err.Error())
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

func (c *Channel) handleHeartbeat(msg *mpro.Message) error {
	c.heartbeatLock.Lock()
	stream := c.heartbeats[msg.Header.RequestID]
	c.heartbeatLock.Unlock()
	if stream == nil {
		vlog.Warningf("handle heartbeat message, missing stream: %d, ep:%s\n", msg.Header.RequestID, c.address)
	} else {
		stream.notify(msg)
	}
	return nil
}

func (c *Channel) handleMessage(msg *mpro.Message) error {
	c.streamLock.Lock()
	stream := c.streams[msg.Header.RequestID]
	c.streamLock.Unlock()
	if stream == nil {
		vlog.Warningf("handle recv message, missing stream: %d, ep:%s\n", msg.Header.RequestID, c.address)
	} else {
		stream.notify(msg)
	}
	return nil
}

func (c *Channel) closeOnErr(err error) {
	c.shutdownLock.Lock()
	if c.shutdownErr == nil {
		c.shutdownErr = err
	}
	if c.shutdown != true { // not normal close
		vlog.Warningf("motan channel will close. ep:%s, err: %s\n", c.address, err.Error())
		c.shutdownLock.Unlock()
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

type ConnFactory func() (net.Conn, error)

type ChannelPool struct {
	channels      chan *Channel
	channelsLock  sync.Mutex
	factory       ConnFactory
	config        *Config
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
			vlog.Errorf("create channel failed. err:%s\n", err.Error())
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

func NewChannelPool(poolCap int, factory ConnFactory, config *Config, serialization motan.Serialization) (*ChannelPool, error) {
	if poolCap <= 0 {
		return nil, errors.New("invalid capacity settings")
	}
	channelPool := &ChannelPool{
		channels:      make(chan *Channel, poolCap),
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
		channelPool.channels <- buildChannel(conn, config, serialization)
	}
	return channelPool, nil
}

func buildChannel(conn net.Conn, config *Config, serialization motan.Serialization) *Channel {
	if conn == nil {
		return nil
	}
	if config == nil {
		config = DefaultConfig()
	}
	if err := VerifyConfig(config); err != nil {
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
