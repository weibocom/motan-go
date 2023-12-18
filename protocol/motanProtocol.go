package protocol

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	DefaultMetaSize         = 16
	DefaultMaxContentLength = 10 * 1024 * 1024
)

// message type
const (
	Req = iota
	Res
)

// message magic
const (
	MotanMagic      = 0xf1f1
	HeaderLength    = 13
	Version1        = 0
	Version2        = 1
	defaultProtocol = "motan2"
)

const (
	MPath          = "M_p"
	MMethod        = "M_m"
	MException     = "M_e"
	MProcessTime   = "M_pt"
	MMethodDesc    = "M_md"
	MGroup         = "M_g"
	MProxyProtocol = "M_pp"
	MVersion       = "M_v"
	MModule        = "M_mdu"
	MSource        = "M_s"
	MRequestID     = "M_rid"
	MTimeout       = "M_tmo"
)

type Header struct {
	Magic         uint16
	MsgType       uint8
	VersionStatus uint8
	Serialize     uint8
	RequestID     uint64
}

func (h *Header) Reset() {
	h.Magic = 0
	h.MsgType = 0
	h.VersionStatus = 0
	h.Serialize = 0
	h.RequestID = 0
}

func (h *Header) Clone() *Header {
	return &Header{
		Magic:         h.Magic,
		MsgType:       h.MsgType,
		VersionStatus: h.VersionStatus,
		Serialize:     h.Serialize,
		RequestID:     h.RequestID,
	}
}

type Message struct {
	Header     *Header
	Metadata   *motan.StringMap
	Body       []byte
	Type       int
	sendStatus atomic.Value
	// lazy init or reset when call Message.Encode or Message.EncodeWithoutBody
	bytesBuffer *motan.BytesBuffer
}

// serialize
const (
	Hessian = iota
	GrpcPb
	JSON
	Msgpack
	Hprose
	Pb
	Simple
	GrpcJSON
)

// message status
const (
	Normal = iota
	Exception
)

var (
	DefaultGzipLevel = gzip.BestSpeed

	defaultSerialize = Simple
	writerPool       = &sync.Pool{}                         // for gzip writer
	readBufPool      = &sync.Pool{}                         // for gzip read buffer
	writeBufPool     = &sync.Pool{New: func() interface{} { // for gzip write buffer
		return &bytes.Buffer{}
	}}
	messagePool = sync.Pool{New: func() interface{} {
		return &Message{Metadata: motan.NewStringMap(DefaultMetaSize), Header: &Header{}}
	}}
)

// errors
var (
	ErrVersion        = errors.New("message version not correct")
	ErrMagicNum       = errors.New("message magic number not correct")
	ErrStatus         = errors.New("message status not correct")
	ErrMetadata       = errors.New("decode meta data fail")
	ErrSerializeNum   = errors.New("message serialize number not correct")
	ErrSerializeNil   = errors.New("message serialize not found")
	ErrSerializedData = errors.New("message serialized data not correct")
	ErrOverSize       = errors.New("content length over the limit")
	ErrWrongSize      = errors.New("content length not correct")
)

// BuildRequestHeader build a proxy request header
func BuildRequestHeader(requestID uint64) *Header {
	return BuildHeader(Req, true, defaultSerialize, requestID, Normal)
}

func BuildResponseHeader(requestID uint64, msgStatus int) *Header {
	return BuildHeader(Res, false, defaultSerialize, requestID, msgStatus)
}

func BuildHeartbeat(requestID uint64, msgType int) *Message {
	request := &Message{
		Header:   BuildHeader(msgType, true, defaultSerialize, requestID, Normal),
		Metadata: motan.NewStringMap(DefaultMetaSize),
		Body:     make([]byte, 0),
		Type:     msgType,
	}
	request.Header.SetHeartbeat(true)
	return request
}

func (h *Header) SetVersion(version int) error {
	if version > 31 {
		return ErrVersion
	}
	h.VersionStatus = (h.VersionStatus & 0x07) | (uint8(version) << 3 & 0xf8)
	return nil
}

func (h *Header) GetVersion() int {
	return int(h.VersionStatus >> 3 & 0x1f)
}

func (h *Header) SetHeartbeat(isHeartbeat bool) {
	if isHeartbeat {
		h.MsgType = h.MsgType | 0x10
	} else {
		h.MsgType = h.MsgType & 0xef
	}
}
func (h *Header) IsHeartbeat() bool {
	return (h.MsgType & 0x10) == 0x10
}

func (h *Header) SetGzip(isgzip bool) {
	if isgzip {
		h.MsgType = h.MsgType | 0x08
	} else {
		h.MsgType = h.MsgType & 0xf7
	}
}

func (h *Header) IsGzip() bool {
	return (h.MsgType & 0x08) == 0x08
}

func (h *Header) SetOneWay(isOneWay bool) {
	if isOneWay {
		h.MsgType = h.MsgType | 0x04
	} else {
		h.MsgType = h.MsgType & 0xfb
	}
}

func (h *Header) IsOneWay() bool {
	return (h.MsgType & 0x04) == 0x04
}

func (h *Header) SetProxy(isProxy bool) {
	if isProxy {
		h.MsgType = h.MsgType | 0x02
	} else {
		h.MsgType = h.MsgType & 0xfd
	}
}

func (h *Header) IsProxy() bool {
	return (h.MsgType & 0x02) == 0x02
}

func (h *Header) SetRequest(isRequest bool) {
	if isRequest {
		h.MsgType = h.MsgType & 0xfe
	} else {
		h.MsgType = h.MsgType | 0x01
	}
}

func (h *Header) isRequest() bool {
	return (h.MsgType & 0x01) == 0x00
}

func (h *Header) SetStatus(status int) error {
	if status > 7 {
		return ErrStatus
	}
	h.VersionStatus = (h.VersionStatus & 0xf8) | (uint8(status) & 0x07)
	return nil
}

func (h *Header) GetStatus() int {
	return int(h.VersionStatus & 0x07)
}

func (h *Header) SetSerialize(serialize int) error {
	if serialize > 31 {
		return ErrSerializeNum
	}
	h.Serialize = (h.Serialize & 0x07) | (uint8(serialize) << 3 & 0xf8)
	return nil
}

func (h *Header) GetSerialize() int {
	return int(h.Serialize >> 3 & 0x1f)
}

// BuildHeader build header with common set
// TODO: remove 'requestID' parameter
func BuildHeader(msgType int, proxy bool, serialize int, requestID uint64, msgStatus int) *Header {
	var mtype uint8 = 0x00
	if proxy {
		mtype = mtype | 0x02
	}
	if msgType == Req {
		mtype = mtype & 0xfe
	} else {
		mtype = mtype | 0x01
	}

	//status
	status := 0x08 | (uint8(msgStatus) & 0x07)

	serial := 0x00 | (uint8(serialize) << 3)

	header := &Header{MotanMagic, mtype, status, serial, requestID}
	return header
}

// EncodeWithoutBody encode message expect Message.Body
// unexpected if call Message.GetEncodedBytes after rewrite result(*motan.BytesBuffer)
func (msg *Message) EncodeWithoutBody() *motan.BytesBuffer {
	var metaSize int
	msg.Metadata.Range(func(k, v string) bool {
		if VerifyMeta(k, v) {
			metaSize += len(k) + len(v) + 2
		}
		return true
	})
	if metaSize > 0 {
		// remove last char '\n'
		metaSize -= 1
	}
	if msg.bytesBuffer == nil {
		msg.bytesBuffer = motan.NewBytesBuffer(HeaderLength + metaSize + 8)
	} else {
		msg.bytesBuffer.Reset()
	}

	// encode header.
	msg.bytesBuffer.WriteUint16(MotanMagic)
	msg.bytesBuffer.WriteByte(msg.Header.MsgType)
	msg.bytesBuffer.WriteByte(msg.Header.VersionStatus)
	msg.bytesBuffer.WriteByte(msg.Header.Serialize)
	msg.bytesBuffer.WriteUint64(msg.Header.RequestID)

	// encode meta
	msg.bytesBuffer.WriteUint32(uint32(metaSize))
	if metaSize > 0 {
		msg.Metadata.Range(func(k, v string) bool {
			if VerifyMeta(k, v) {
				msg.bytesBuffer.WriteString(k)
				msg.bytesBuffer.WriteByte('\n')
				msg.bytesBuffer.WriteString(v)
				msg.bytesBuffer.WriteByte('\n')
			}
			return true
		})
		// rewrite last char '\n'
		msg.bytesBuffer.SetWPos(msg.bytesBuffer.GetWPos() - 1)
	}

	// encode body size
	bodySize := len(msg.Body)
	msg.bytesBuffer.WriteUint32(uint32(bodySize))
	return msg.bytesBuffer
}

// GetEncodedBytes get encoded headers Message.Body
// unexpected if repeat call Message.GetEncodedBytes after rewrite result([][]byte)
func (msg *Message) GetEncodedBytes() [][]byte {
	if msg.bytesBuffer == nil { // maybe not call Message.Encode or Message.EncodeWithoutBody
		msg.EncodeWithoutBody()
	}
	return [][]byte{msg.bytesBuffer.Bytes(), msg.Body}
}

// Encode result encoded result
// unexpected if call Message.GetEncodedBytes after rewrite result(*motan.BytesBuffer)
func (msg *Message) Encode() *motan.BytesBuffer {
	msg.EncodeWithoutBody()
	bodySize := len(msg.Body)
	if bodySize > 0 {
		msg.bytesBuffer.Write(msg.Body)
	}
	return msg.bytesBuffer
}

func (msg *Message) Reset() bool {
	if ok, v := msg.sendStatus.Load().(bool); ok && v {
		msg.Type = 0
		msg.Body = msg.Body[:0]
		msg.Header.Reset()
		msg.Metadata.Reset()
		msg.sendStatus.Store(false)
		return true
	}
	return false
}

func (msg *Message) Clone() interface{} {
	newMessage := &Message{
		Header: msg.Header.Clone(),
		Body:   msg.Body,
		Type:   msg.Type,
	}
	if msg.Metadata != nil {
		newMessage.Metadata = msg.Metadata.Copy()
	}
	return newMessage
}

func (msg *Message) SetAlreadySend() {
	msg.sendStatus.Store(true)
}

func CheckMotanVersion(buf *bufio.Reader) (version int, err error) {
	var b []byte
	b, err = buf.Peek(4)
	if err != nil {
		return -1, err
	}
	mn := binary.BigEndian.Uint16(b[:2])
	if mn != MotanMagic {
		vlog.Errorf("wrong magic num:%d, err:%v", mn, err)
		return -1, ErrMagicNum
	}
	return int(b[3] >> 3 & 0x1f), nil
}

func DecodeWithoutBody(buf *bufio.Reader, rs *[]byte, maxContentLength int) (msg *Message, bodySize int, err error) {
	readSlice := *rs
	// decode header
	_, err = io.ReadAtLeast(buf, readSlice[:HeaderLength], HeaderLength)
	if err != nil {
		return nil, 0, err
	}
	mn := binary.BigEndian.Uint16(readSlice[:2]) // TODO 不再验证
	if mn != MotanMagic {
		vlog.Errorf("wrong magic num:%d, err:%v", mn, err)
		return nil, 0, ErrMagicNum
	}
	// get a message from pool
	msg = AcquireMessage()
	defer func() {
		if err != nil {
			ReleaseMessage(msg)
		}
	}()
	msg.Header.Magic = MotanMagic
	msg.Header.MsgType = readSlice[2]
	msg.Header.VersionStatus = readSlice[3]
	version := msg.Header.GetVersion()
	if version != Version2 { // TODO 不再验证
		err = ErrVersion
		vlog.Errorf("unsupported protocol version number: %d", version)
		return nil, 0, ErrVersion
	}
	msg.Header.Serialize = readSlice[4]
	msg.Header.RequestID = binary.BigEndian.Uint64(readSlice[5:HeaderLength])

	// decode meta
	_, err = io.ReadAtLeast(buf, readSlice[:4], 4)
	if err != nil {
		return nil, 0, err
	}
	metasize := int(binary.BigEndian.Uint32(readSlice[:4]))
	if metasize > maxContentLength {
		err = ErrOverSize
		vlog.Errorf("meta over size. meta size:%d, max size:%d", metasize, maxContentLength)
		return nil, 0, ErrOverSize
	}
	if metasize > 0 {
		if cap(readSlice) < metasize {
			readSlice = make([]byte, metasize)
			*rs = readSlice
		}
		err = readBytes(buf, readSlice, metasize)
		if err != nil {
			return nil, 0, err
		}
		s, e := 0, 0
		var k string
		for i := 0; i <= metasize; i++ {
			if i == metasize || readSlice[i] == '\n' {
				e = i
				if k == "" {
					k = string(readSlice[s:e])
				} else {
					msg.Metadata.Store(k, string(readSlice[s:e]))
					k = ""
				}
				s = i + 1
			}
		}
		if k != "" {
			err = ErrMetadata
			vlog.Errorf("decode message fail, metadata not paired. header:%v, meta:%s", msg.Header, readSlice)
			return nil, 0, ErrMetadata
		}
	}

	//decode body
	_, err = io.ReadAtLeast(buf, readSlice[:4], 4)
	if err != nil {
		return nil, 0, err
	}
	bodySize = int(binary.BigEndian.Uint32(readSlice[:4]))
	return msg, bodySize, nil
}

func Decode(buf *bufio.Reader, readSlice *[]byte) (msg *Message, err error) {
	msg, _, err = DecodeWithTime(buf, readSlice, motan.DefaultMaxContentLength)
	return msg, err
}

// DecodeWithTime the parameter rs is a slice pointer, so the
// extension of *rs will affect the slice outside in order to
// reuse the readSlice which is used by reading header info from buf
func DecodeWithTime(buf *bufio.Reader, rs *[]byte, maxContentLength int) (msg *Message, start time.Time, err error) {
	start = time.Now() // record time when starting to read data
	msg, bodySize, err := DecodeWithoutBody(buf, rs, maxContentLength)
	if err != nil {
		return msg, start, err
	}
	if bodySize > maxContentLength {
		err = ErrOverSize
		vlog.Errorf("body over size. body size:%d, max size:%d", bodySize, maxContentLength)
		return nil, start, ErrOverSize
	}

	if bodySize > 0 {
		if cap(msg.Body) < bodySize {
			msg.Body = make([]byte, bodySize)
		}
		msg.Body = msg.Body[:bodySize]
		err = readBytes(buf, msg.Body, bodySize)
	} else {
		msg.Body = make([]byte, 0)
	}
	if err != nil {
		return nil, start, err
	}
	return msg, start, err
}

func DecodeGzipBody(body []byte) []byte {
	ret, err := DecodeGzip(body)
	if err != nil {
		vlog.Warningf("decode gzip body fail!%s", err.Error())
		return body
	}
	return ret
}

func readBytes(buf *bufio.Reader, readSlice []byte, size int) error {
	var s, n = 0, 0
	var err error
	for s < size && err == nil {
		n, err = buf.Read(readSlice[s:size])
		s += n
	}
	return err
}

func EncodeGzip(data []byte) ([]byte, error) {
	if len(data) > 0 {
		buf := writeBufPool.Get().(*bytes.Buffer)
		var w *gzip.Writer
		temp := writerPool.Get()
		if temp == nil {
			w, _ = gzip.NewWriterLevel(buf, DefaultGzipLevel)
		} else {
			w = temp.(*gzip.Writer)
			w.Reset(buf)
		}
		defer func() {
			buf.Reset()
			writeBufPool.Put(buf)
			writerPool.Put(w)
		}()
		_, err := w.Write(data)
		if err != nil {
			return nil, err
		}
		err = w.Flush()
		if err != nil {
			return nil, err
		}
		w.Close()
		ret := make([]byte, buf.Len())
		copy(ret, buf.Bytes())
		return ret, nil
	}
	return data, nil
}

func EncodeMessageGzip(msg *Message, gzipSize int) {
	if gzipSize > 0 && len(msg.Body) > gzipSize && !msg.Header.IsGzip() {
		data, err := EncodeGzip(msg.Body)
		if err != nil {
			vlog.Warningf("encode gzip fail! request id:%d, err:%s", msg.Header.RequestID, err.Error())
		} else {
			msg.Header.SetGzip(true)
			msg.Body = data
		}
	}
}

func DecodeGzip(data []byte) (ret []byte, err error) {
	if len(data) > 0 {
		r, err := gzip.NewReader(bytes.NewBuffer(data))
		if err != nil {
			return nil, err
		}
		var buf *bytes.Buffer
		temp := readBufPool.Get()
		if temp == nil {
			size := len(data)
			if size < 5000 {
				size = 2 * size
			} else {
				size = 4 * size
			}
			buf = bytes.NewBuffer(make([]byte, 0, size))
		} else {
			buf = temp.(*bytes.Buffer)
		}
		defer func() {
			buf.Reset()
			readBufPool.Put(buf)
			e := recover()
			if e == nil {
				return
			}
			if panicErr, ok := e.(error); ok && panicErr == bytes.ErrTooLarge {
				err = panicErr
			} else {
				panic(e)
			}
		}()

		_, err = buf.ReadFrom(r)
		ret := make([]byte, buf.Len())
		copy(ret, buf.Bytes())
		return ret, err
	}
	return data, nil
}

// ConvertToRequest convert motan2 protocol request message  to motan Request
func ConvertToRequest(request *Message, serialize motan.Serialization) (motan.Request, error) {
	motanRequest := motan.AcquireMotanRequest()
	motanRequest.RequestID = request.Header.RequestID
	if idStr, ok := request.Metadata.Load(MRequestID); !ok {
		if request.Header.IsProxy() {
			// we need convert it to a int64 string, some language has no uint64
			request.Metadata.Store(MRequestID, strconv.FormatInt(int64(request.Header.RequestID), 10))
		}
	} else {
		requestID, _ := strconv.ParseInt(idStr, 10, 64)
		motanRequest.RequestID = uint64(requestID)
	}

	motanRequest.ServiceName = request.Metadata.LoadOrEmpty(MPath)
	motanRequest.Method = request.Metadata.LoadOrEmpty(MMethod)
	motanRequest.MethodDesc = request.Metadata.LoadOrEmpty(MMethodDesc)
	motanRequest.Attachment = request.Metadata
	rc := motanRequest.GetRPCContext(true)
	rc.OriginalMessage = request
	rc.Proxy = request.Header.IsProxy()
	if request.Body != nil && len(request.Body) > 0 {
		rc.BodySize = len(request.Body)
		if request.Header.IsGzip() {
			request.Body = DecodeGzipBody(request.Body)
			request.Header.SetGzip(false)
		}
		if !rc.Proxy && serialize == nil {
			motan.ReleaseMotanRequest(motanRequest)
			return nil, ErrSerializeNil
		}
		motanRequest.Arguments = append(motanRequest.Arguments, &motan.DeserializableValue{Body: request.Body, Serialization: serialize})
	}
	return motanRequest, nil
}

// ConvertToReqMessage convert motan Request to protocol request
func ConvertToReqMessage(request motan.Request, serialize motan.Serialization) (*Message, error) {
	rc := request.GetRPCContext(true)
	if rc.Proxy && rc.OriginalMessage != nil {
		if msg, ok := rc.OriginalMessage.(*Message); ok {
			msg.Header.SetProxy(true)
			// Make sure group is the requested group
			msg.Metadata.Store(MGroup, request.GetAttachment(MGroup))
			EncodeMessageGzip(msg, rc.GzipSize)
			rc.BodySize = len(msg.Body)
			return msg, nil
		}
	}
	req := &Message{}
	if rc.Serialized { // params already serialized
		req.Header = BuildHeader(Req, false, rc.SerializeNum, request.GetRequestID(), Normal)
	} else {
		if serialize == nil {
			return nil, ErrSerializeNil
		}
		req.Header = BuildHeader(Req, false, serialize.GetSerialNum(), request.GetRequestID(), Normal)
	}
	if len(request.GetArguments()) > 0 {
		if rc.Serialized { // params already serialized
			if len(request.GetArguments()) == 1 {
				if b, ok := request.GetArguments()[0].([]byte); ok {
					req.Body = b
				} else {
					vlog.Warningf("convert request value fail! serialized value not []byte. request:%+v", request)
					return nil, ErrSerializedData
				}
			} else {
				vlog.Warningf("convert request value fail! serialized value size > 1. request:%+v", request)
				return nil, ErrSerializedData
			}
		} else {
			b, err := serialize.SerializeMulti(request.GetArguments())
			if err != nil {
				return nil, err
			}
			req.Body = b
		}
	}

	req.Metadata = request.GetAttachments()
	EncodeMessageGzip(req, rc.GzipSize)
	rc.BodySize = len(req.Body)
	if rc.Oneway {
		req.Header.SetOneWay(true)
	}
	if rc.Proxy {
		req.Header.SetProxy(true)
	}
	req.Header.SetSerialize(serialize.GetSerialNum())
	req.Metadata.Store(MPath, request.GetServiceName())
	req.Metadata.Store(MMethod, request.GetMethod())
	if request.GetAttachment(MProxyProtocol) == "" {
		req.Metadata.Store(MProxyProtocol, defaultProtocol)
	}

	if request.GetMethodDesc() != "" {
		req.Metadata.Store(MMethodDesc, request.GetMethodDesc())
	}
	return req, nil
}

// ConvertToResMessage convert motan Response to protocol response
func ConvertToResMessage(response motan.Response, serialize motan.Serialization) (*Message, error) {
	rc := response.GetRPCContext(true)
	if rc.Proxy && rc.OriginalMessage != nil {
		if msg, ok := rc.OriginalMessage.(*Message); ok {
			msg.Header.SetProxy(true)
			return msg, nil
		}
	}
	res := AcquireMessage()
	var msgType int
	if response.GetException() != nil {
		msgType = Exception
		response.SetAttachment(MException, ExceptionToJSON(response.GetException()))
		if rc.Proxy {
			rc.Serialized = true
		}
	} else {
		msgType = Normal
	}
	if rc.Serialized {
		res.Header = BuildHeader(Res, false, rc.SerializeNum, response.GetRequestID(), msgType)
	} else {
		if serialize == nil {
			return nil, ErrSerializeNil
		}
		res.Header = BuildHeader(Res, false, serialize.GetSerialNum(), response.GetRequestID(), msgType)
	}

	if response.GetValue() != nil {
		if rc.Serialized {
			if b, ok := response.GetValue().([]byte); ok {
				res.Body = b
			} else {
				vlog.Warningf("convert response value fail! serialized value not []byte. res:%+v", response)
				ReleaseMessage(res)
				return nil, ErrSerializedData
			}
		} else {
			b, err := serialize.Serialize(response.GetValue())
			if err != nil {
				ReleaseMessage(res)
				return nil, err
			}
			res.Body = b
		}
	}

	res.Metadata = response.GetAttachments()
	EncodeMessageGzip(res, rc.GzipSize)
	rc.BodySize = len(res.Body)
	if rc.Proxy {
		res.Header.SetProxy(true)
	}

	return res, nil
}

// ConvertToResponse convert protocol response to motan Response
func ConvertToResponse(response *Message, serialize motan.Serialization) (motan.Response, error) {
	mres := motan.AcquireMotanResponse()
	rc := mres.GetRPCContext(true)
	rc.Proxy = response.Header.IsProxy()
	mres.RequestID = response.Header.RequestID
	if pt, ok := response.Metadata.Load(MProcessTime); ok {
		if ptInt, err := strconv.Atoi(pt); err == nil {
			mres.SetProcessTime(int64(ptInt))
		}
	}
	if response.Header.GetStatus() == Normal && len(response.Body) > 0 {
		rc.BodySize = len(response.Body)
		if response.Header.IsGzip() {
			response.Body = DecodeGzipBody(response.Body)
			response.Header.SetGzip(false)
		}
		if !rc.Proxy && serialize == nil {
			motan.ReleaseMotanResponse(mres)
			return nil, ErrSerializeNil
		}
		dv := &motan.DeserializableValue{Body: response.Body, Serialization: serialize}
		mres.Value = dv
	}
	if response.Header.GetStatus() == Exception {
		e := response.Metadata.LoadOrEmpty(MException)
		if e != "" {
			var exception *motan.Exception
			err := json.Unmarshal([]byte(e), &exception)
			if err != nil {
				motan.ReleaseMotanResponse(mres)
				return nil, err
			}
			mres.Exception = exception
		}
	}
	mres.Attachment = response.Metadata
	rc.OriginalMessage = response
	rc.Proxy = response.Header.IsProxy()
	return mres, nil
}

func VerifyMeta(k, v string) bool {
	if k == "" || v == "" {
		return false
	}
	if strings.Contains(k, "\n") || strings.Contains(v, "\n") {
		vlog.Errorf("metadata not correct.k:%s, v:%s", k, v)
		return false
	}
	return true
}

func BuildExceptionResponse(requestID uint64, errmsg string) *Message {
	header := BuildHeader(Res, false, defaultSerialize, requestID, Exception)
	msg := &Message{Header: header, Metadata: motan.NewStringMap(DefaultMetaSize)}
	msg.Metadata.Store(MException, errmsg)
	return msg
}

func ExceptionToJSON(e *motan.Exception) string {
	errmsg, _ := json.Marshal(e)
	return string(errmsg)
}

func AcquireMessage() *Message {
	return messagePool.Get().(*Message)
}

func ReleaseMessage(msg *Message) {
	if msg != nil && msg.Reset() {
		messagePool.Put(msg)
	}
}
