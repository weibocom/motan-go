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
	"strings"

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
	MotanMagic   = 0xf1f1
	HeaderLength = 13
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
	buf = new(bytes.Buffer)
	var metabuf bytes.Buffer
	var meta map[string]string
	meta = msg.Metadata
	for k, v := range meta {
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

	body := msg.Body
	bodysize := int32(len(body))
	binary.Write(buf, binary.BigEndian, msg.Header)
	binary.Write(buf, binary.BigEndian, metasize)
	if metasize > 0 {
		binary.Write(buf, binary.BigEndian, metabuf.Bytes()[:metasize])
	}

	binary.Write(buf, binary.BigEndian, bodysize)
	if bodysize > 0 {
		binary.Write(buf, binary.BigEndian, body)
	}
	return buf
}

// Decode decode one message from buffer
func Decode(reqbuf *bytes.Buffer) *Message {
	header := &Header{}
	binary.Read(reqbuf, binary.BigEndian, header)
	metasize := readInt32(reqbuf)
	metamap := make(map[string]string)
	if metasize > 0 {
		metadata := string(reqbuf.Next(int(metasize)))
		values := strings.Split(metadata, "\n")
		for i := 0; i < len(values); i++ {
			key := values[i]
			i++
			metamap[key] = values[i]
		}

	}
	bodysize := readInt32(reqbuf)
	var body []byte
	if bodysize > 0 {
		body = reqbuf.Next(int(bodysize))
	} else {
		body = make([]byte, 0)
	}
	msg := &Message{header, metamap, body, Req}
	return msg
}

// DecodeFromReader decode one message from reader
func DecodeFromReader(buf *bufio.Reader) (msg *Message, err error) {
	header := &Header{}
	err = binary.Read(buf, binary.BigEndian, header)
	if err != nil {
		return nil, err
	}

	metasize := readInt32(buf)
	metamap := make(map[string]string)
	if metasize > 0 {
		metadata, err := readBytes(buf, int(metasize))
		if err != nil {
			return nil, err
		}
		values := strings.Split(string(metadata), "\n")
		for i := 0; i < len(values); i++ {
			key := values[i]
			i++
			metamap[key] = values[i]
		}

	}
	bodysize := readInt32(buf)
	var body []byte
	if bodysize > 0 {
		body, err = readBytes(buf, int(bodysize))
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

func readInt32(buf io.Reader) int32 {
	var i int32
	binary.Read(buf, binary.BigEndian, &i)
	return i
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
		if rc.Proxy { // put ungzip body to arguments if proxy
			motanRequest.Arguments = []interface{}{request.Body}
		} else { // put DeserializableValue to arguments if not proxy
			if serialize == nil {
				return nil, errors.New("serialization is nil")
			}
			dv := &motan.DeserializableValue{Body: request.Body, Serialization: serialize}
			motanRequest.Arguments = []interface{}{dv}
		}
	}

	return motanRequest, nil
}

// ConvertToReqMessage convert motan Request to protocol request
func ConvertToReqMessage(request motan.Request, serialize motan.Serialization) (*Message, error) {
	rc := request.GetRPCContext(false)
	if rc != nil && rc.Proxy && rc.OriginalMessage != nil {
		if msg, ok := rc.OriginalMessage.(*Message); ok {
			msg.Header.SetProxy(true)
			return msg, nil
		}
	}

	var header *Header
	if rc.Proxy {
		header = BuildHeader(Req, false, rc.SerializeNum, request.GetRequestID(), Normal)
	} else {
		if serialize == nil {
			return nil, errors.New("serialization is nil")
		}
		header = BuildHeader(Req, false, serialize.GetSerialNum(), request.GetRequestID(), Normal)
	}
	req := &Message{Header: header, Metadata: make(map[string]string)}

	if len(request.GetArguments()) > 0 {

		b, err := serialize.SerializeMulti(request.GetArguments())
		if err != nil {
			return nil, err
		}
		req.Body = b
	}
	req.Metadata = request.GetAttachments()
	if rc != nil {
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
	}
	req.Header.SetSerialize(serialize.GetSerialNum())
	req.Metadata[MPath] = request.GetServiceName()
	req.Metadata[MMethod] = request.GetMethod()
	if request.GetAttachment(MProxyProtocol) == "" {
		req.Metadata[MProxyProtocol] = "motan2"
	}

	if request.GetMethodDesc() != "" {
		req.Metadata[MMethodDesc] = request.GetMethodDesc()
	}
	return req, nil

}

// ConvertToResMessage convert motan Response to protocol response
func ConvertToResMessage(response motan.Response, serialize motan.Serialization) (*Message, error) {
	rc := response.GetRPCContext(false)

	if rc != nil && rc.Proxy && rc.OriginalMessage != nil {
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

	if rc.Proxy {
		res.Header = BuildHeader(Res, false, rc.SerializeNum, response.GetRequestID(), msgType)
		res.Header.SetProxy(true)
		if response.GetValue() != nil {
			if b, ok := response.GetValue().([]byte); ok {
				res.Body = b
			} else {
				vlog.Warningf("convert response value fail in proxy type! value not []byte. v:%v\n", response.GetValue())
			}
		}
	} else {
		if serialize == nil {
			return nil, errors.New("serialization is nil")
		}
		res.Header = BuildHeader(Res, false, serialize.GetSerialNum(), response.GetRequestID(), msgType)
		if response.GetValue() != nil {
			b, err := serialize.Serialize(response.GetValue())
			if err != nil {
				return nil, err
			}
			res.Body = b
		}
	}

	res.Metadata = response.GetAttachments()

	if rc != nil {
		if rc.GzipSize > 0 && len(res.Body) > rc.GzipSize {
			data, err := EncodeGzip(res.Body)
			if err != nil {
				vlog.Errorf("encode gzip fail! requestid:%d, err %s\n", response.GetRequestID(), err.Error())
			} else {
				res.Header.SetGzip(true)
				res.Body = data
			}
		}
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
		if rc.Proxy {
			mres.Value = response.Body
		} else {
			if serialize == nil {
				return nil, errors.New("serialization is nil")
			}
			dv := &motan.DeserializableValue{Body: response.Body, Serialization: serialize}
			mres.Value = dv
		}
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
