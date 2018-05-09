package protocol

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

//message type
const (
	Req = iota
	Res
)

// message magic
const (
	MotanMagic      = 0xf1f1
	HeaderLength    = 13
	defaultProtocol = "motan2"
)
const (
	MPath          = "M_p"
	MMethod        = "M_m"
	MExceptionn    = "M_e"
	MProcessTime   = "M_pt"
	MMethodDesc    = "M_md"
	MGroup         = "M_g"
	MProxyProtocol = "M_pp"
	MVersion       = "M_v"
	MModule        = "M_mdu"
	MSource        = "M_s"
)

type Header struct {
	Magic         uint16
	MsgType       uint8
	VersionStatus uint8
	Serialize     uint8
	RequestID     uint64
}

type Message struct {
	Header   *Header
	Metadata map[string]string
	Body     []byte
	Type     int
}

//serialize
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
	defaultSerialize = Simple
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
		Metadata: make(map[string]string),
		Body:     make([]byte, 0),
		Type:     msgType,
	}
	request.Header.SetHeartbeat(true)
	return request
}

func (h *Header) SetVersion(version int) error {
	if version > 31 {
		return errors.New("motan header: version should not great than 31")
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
		return errors.New("motan header: status should not great than 7")
	}
	h.VersionStatus = (h.VersionStatus & 0xf8) | (uint8(status) & 0x07)
	return nil
}

func (h *Header) GetStatus() int {
	return int(h.VersionStatus & 0x07)
}

func (h *Header) SetSerialize(serialize int) error {
	if serialize > 31 {
		return errors.New("motan header: serialize should not great than 31")
	}
	h.Serialize = (h.Serialize & 0x07) | (uint8(serialize) << 3 & 0xf8)
	return nil
}

func (h *Header) GetSerialize() int {
	return int(h.Serialize >> 3 & 0x1f)
}

// BuildHeader build header with common set
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
	status := uint8(0x08 | (uint8(msgStatus) & 0x07))

	serial := uint8(0x00 | (uint8(serialize) << 3))

	header := &Header{MotanMagic, mtype, status, serial, requestID}
	return header
}

func (msg *Message) Encode() (buf *bytes.Buffer) {
	metabuf := bytes.NewBuffer(make([]byte, 0, 256))
	for k, v := range msg.Metadata {
		metabuf.WriteString(k)
		metabuf.WriteString("\n")
		metabuf.WriteString(v)
		metabuf.WriteString("\n")
	}
	var metasize int32
	if metabuf.Len() > 0 {
		metasize = int32(metabuf.Len() - 1)
	} else {
		metasize = 0
	}

	bodysize := int32(len(msg.Body))
	buf = bytes.NewBuffer(make([]byte, 0, (HeaderLength + metasize + bodysize + 8)))

	// encode header.
	temp := make([]byte, 8, 8)
	binary.BigEndian.PutUint16(temp, msg.Header.Magic)
	buf.Write(temp[:2])
	buf.WriteByte(msg.Header.MsgType)
	buf.WriteByte(msg.Header.VersionStatus)
	buf.WriteByte(msg.Header.Serialize)
	binary.BigEndian.PutUint64(temp, msg.Header.RequestID)
	buf.Write(temp)

	// encode meta
	binary.BigEndian.PutUint32(temp, uint32(metasize))
	buf.Write(temp[:4])
	if metasize > 0 {
		buf.Write(metabuf.Bytes()[:metasize])
	}

	// encode body
	binary.BigEndian.PutUint32(temp, uint32(bodysize))
	buf.Write(temp[:4])
	if bodysize > 0 {
		buf.Write(msg.Body)
	}
	return buf
}

func Decode(buf *bufio.Reader) (msg *Message, err error) {
	temp := make([]byte, HeaderLength, HeaderLength)

	// decode header
	_, err = io.ReadAtLeast(buf, temp, HeaderLength)
	if err != nil {
		return nil, err
	}
	mn := binary.BigEndian.Uint16(temp[:2])
	if mn != MotanMagic {
		vlog.Errorf("worng magic num:%d, err:%v\n", mn, err)
		return nil, errors.New("motan magic num not correct.")
	}
	header := &Header{Magic: MotanMagic}
	header.MsgType = temp[2]
	header.VersionStatus = temp[3]
	header.Serialize = temp[4]
	header.RequestID = binary.BigEndian.Uint64(temp[5:])

	// decode meta
	_, err = io.ReadAtLeast(buf, temp[:4], 4)
	if err != nil {
		return nil, err
	}
	metasize := int(binary.BigEndian.Uint32(temp[:4]))
	metamap := make(map[string]string)
	if metasize > 0 {
		metadata, err := readBytes(buf, metasize)
		if err != nil {
			return nil, err
		}
		s, e := 0, 0
		var k string
		for i := 0; i <= metasize; i++ {
			if i == metasize || metadata[i] == '\n' {
				e = i
				if k == "" {
					k = string(metadata[s:e])
				} else {
					metamap[k] = string(metadata[s:e])
					k = ""
				}
				s = i + 1
			}
		}
		if k != "" {
			vlog.Errorf("decode message fail, metadata not paired. header:%v, meta:%s\n", header, metadata)
			return nil, errors.New("decode message fail, metadata not paired")
		}
	}

	//decode body
	_, err = io.ReadAtLeast(buf, temp[:4], 4)
	if err != nil {
		return nil, err
	}
	bodysize := int(binary.BigEndian.Uint32(temp[:4]))
	var body []byte
	if bodysize > 0 {
		body, err = readBytes(buf, bodysize)
	} else {
		body = make([]byte, 0)
	}
	if err != nil {
		return nil, err
	}
	msg = &Message{header, metamap, body, Req}
	return msg, err
}

func DecodeGzipBody(body []byte) []byte {
	ret, err := DecodeGzip(body)
	if err != nil {
		vlog.Warningf("decode gzip body fail!%s\n", err.Error())
		return body
	}
	return ret
}

func readBytes(buf *bufio.Reader, size int) ([]byte, error) {
	tempbytes := make([]byte, size)
	var s, n int = 0, 0
	var err error
	for s < size && err == nil {
		n, err = buf.Read(tempbytes[s:])
		s += n
	}
	return tempbytes, err
}

// EncodeGzip : encode gzip
func EncodeGzip(data []byte) ([]byte, error) {
	if len(data) > 0 {
		var b bytes.Buffer
		w := gzip.NewWriter(&b)
		defer w.Close()
		_, e1 := w.Write(data)
		if e1 != nil {
			return nil, e1
		}
		e2 := w.Flush()
		if e2 != nil {
			return nil, e2
		}
		w.Close()
		return b.Bytes(), nil
	}
	return data, nil
}

// DecodeGzip : decode gzip
func DecodeGzip(data []byte) ([]byte, error) {
	if len(data) > 0 {
		b := bytes.NewBuffer(data)
		r, e1 := gzip.NewReader(b)
		if e1 != nil {
			return nil, e1
		}
		defer r.Close()
		ret, e2 := ioutil.ReadAll(r)
		if e2 != nil {
			return nil, e2
		}
		return ret, nil
	}
	return data, nil
}

// ConvertToRequest convert motan2 protocol request message  to motan Request
func ConvertToRequest(request *Message, serialize motan.Serialization) (motan.Request, error) {
	motanRequest := &motan.MotanRequest{Arguments: make([]interface{}, 0)}
	motanRequest.RequestID = request.Header.RequestID
	motanRequest.ServiceName = request.Metadata[MPath]
	motanRequest.Method = request.Metadata[MMethod]
	motanRequest.MethodDesc = request.Metadata[MMethodDesc]
	motanRequest.Attachment = request.Metadata
	rc := motanRequest.GetRPCContext(true)
	rc.OriginalMessage = request
	rc.Proxy = request.Header.IsProxy()
	if request.Body != nil && len(request.Body) > 0 {
		if request.Header.IsGzip() {
			request.Body = DecodeGzipBody(request.Body)
			request.Header.SetGzip(false)
		}
		if !rc.Proxy && serialize == nil {
			return nil, errors.New("serialization is nil")
		}
		dv := &motan.DeserializableValue{Body: request.Body, Serialization: serialize}
		motanRequest.Arguments = []interface{}{dv}
	}

	return motanRequest, nil
}

// ConvertToReqMessage convert motan Request to protocol request
func ConvertToReqMessage(request motan.Request, serialize motan.Serialization) (*Message, error) {
	rc := request.GetRPCContext(true)
	if rc.Proxy && rc.OriginalMessage != nil {
		if msg, ok := rc.OriginalMessage.(*Message); ok {
			msg.Header.SetProxy(true)
			return msg, nil
		}
	}
	req := &Message{Metadata: make(map[string]string)}
	if rc.Serialized { // params already serialized
		req.Header = BuildHeader(Req, false, rc.SerializeNum, request.GetRequestID(), Normal)
	} else {
		if serialize == nil {
			return nil, errors.New("serialization is nil")
		}
		req.Header = BuildHeader(Req, false, serialize.GetSerialNum(), request.GetRequestID(), Normal)
	}
	if len(request.GetArguments()) > 0 {
		if rc.Serialized { // params already serialized
			if len(request.GetArguments()) == 1 {
				if b, ok := request.GetArguments()[0].([]byte); ok {
					req.Body = b
				} else {
					vlog.Warningf("convert request value fail! serialized value not []byte. request:%+v\n", request)
					return nil, errors.New("convert request value fail! serialized value not []byte")
				}
			} else {
				vlog.Warningf("convert request value fail! serialized value size > 1. request:%+v\n", request)
				return nil, errors.New("convert request value fail! serialized value size > 1")
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
	if rc.GzipSize > 0 && len(req.Body) > rc.GzipSize {
		data, err := EncodeGzip(req.Body)
		if err != nil {
			vlog.Errorf("encode gzip fail! %s, err %s\n", motan.GetReqInfo(request), err.Error())
		} else {
			req.Header.SetGzip(true)
			req.Body = data
		}
	}
	if rc.Oneway {
		req.Header.SetOneWay(true)
	}
	if rc.Proxy {
		req.Header.SetProxy(true)
	}
	req.Header.SetSerialize(serialize.GetSerialNum())
	req.Metadata[MPath] = request.GetServiceName()
	req.Metadata[MMethod] = request.GetMethod()
	if request.GetAttachment(MProxyProtocol) == "" {
		req.Metadata[MProxyProtocol] = defaultProtocol
	}

	if request.GetMethodDesc() != "" {
		req.Metadata[MMethodDesc] = request.GetMethodDesc()
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

	res := &Message{Metadata: make(map[string]string)}
	var msgType int
	if response.GetException() != nil {
		msgType = Exception
		response.SetAttachment(MExceptionn, ExceptionToJSON(response.GetException()))
	} else {
		msgType = Normal
	}
	if rc.Serialized {
		res.Header = BuildHeader(Res, false, rc.SerializeNum, response.GetRequestID(), msgType)
	} else {
		if serialize == nil {
			return nil, errors.New("serialization is nil")
		}
		res.Header = BuildHeader(Res, false, serialize.GetSerialNum(), response.GetRequestID(), msgType)
	}

	if response.GetValue() != nil {
		if rc.Serialized {
			if b, ok := response.GetValue().([]byte); ok {
				res.Body = b
			} else {
				vlog.Warningf("convert response value fail! serialized value not []byte. res:%+v\n", response)
				return nil, errors.New("convert response value fail! serialized value not []byte")
			}
		} else {
			b, err := serialize.Serialize(response.GetValue())
			if err != nil {
				return nil, err
			}
			res.Body = b
		}
	}

	res.Metadata = response.GetAttachments()

	if rc.GzipSize > 0 && len(res.Body) > rc.GzipSize {
		data, err := EncodeGzip(res.Body)
		if err != nil {
			vlog.Errorf("encode gzip fail! requestid:%d, err %s\n", response.GetRequestID(), err.Error())
		} else {
			res.Header.SetGzip(true)
			res.Body = data
		}
	}
	if rc.Proxy {
		res.Header.SetProxy(true)
	}

	return res, nil
}

// ConvertToResponse convert protocol response to motan Response
func ConvertToResponse(response *Message, serialize motan.Serialization) (motan.Response, error) {
	mres := &motan.MotanResponse{}
	rc := mres.GetRPCContext(true)
	rc.Proxy = response.Header.IsProxy()
	mres.RequestID = response.Header.RequestID
	if response.Header.GetStatus() == Normal && len(response.Body) > 0 {
		if response.Header.IsGzip() {
			response.Body = DecodeGzipBody(response.Body)
			response.Header.SetGzip(false)
		}
		if !rc.Proxy && serialize == nil {
			return nil, errors.New("serialization is nil")
		}
		dv := &motan.DeserializableValue{Body: response.Body, Serialization: serialize}
		mres.Value = dv
	}
	if response.Header.GetStatus() == Exception && response.Metadata[MExceptionn] != "" {
		var exception *motan.Exception
		err := json.Unmarshal([]byte(response.Metadata[MExceptionn]), &exception)
		if err != nil {
			return nil, err
		}
		mres.Exception = exception
	}
	mres.Attachment = response.Metadata
	rc.OriginalMessage = response
	rc.Proxy = response.Header.IsProxy()
	return mres, nil
}

func BuildExceptionResponse(requestID uint64, errmsg string) *Message {
	header := BuildHeader(Res, false, defaultSerialize, requestID, Exception)
	msg := &Message{Header: header, Metadata: make(map[string]string)}
	msg.Metadata[MExceptionn] = errmsg
	return msg
}

func ExceptionToJSON(e *motan.Exception) string {
	errmsg, _ := json.Marshal(e)
	return string(errmsg)
}
