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
const (
	MotanMagic   = 0xf1f1
	HeaderLength = 13
)
const (
	M_path          = "M_p"
	M_method        = "M_m"
	M_exception     = "M_e"
	M_processTime   = "M_pt"
	M_methodDesc    = "M_md"
	M_group         = "M_g"
	M_proxyProtocol = "M_pp"
	M_version       = "M_v"
	M_module        = "M_mdu"
	M_source        = "M_s"
)

type Header struct {
	Magic          uint16
	MsgType        uint8
	Version_status uint8
	Serialize      uint8
	RequestId      uint64
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
	Json
	Msgpack
	Hprose
	Pb
	Simple
	GrpcJson
)

// message status
const (
	Normal = iota
	Exception
)

var (
	defaultSerialize = Simple
)

// proxy request header
func BuildRequestHeader(requestId uint64) *Header {
	return BuildHeader(Req, true, defaultSerialize, requestId, Normal)
}

func BuildResponseHeader(requestId uint64, msgStatus int) *Header {
	return BuildHeader(Res, false, defaultSerialize, requestId, msgStatus)
}

func BuildHeartbeat(requestId uint64, msgType int) *Message {
	request := &Message{
		Header:   BuildHeader(msgType, true, defaultSerialize, requestId, Normal),
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
	h.Version_status = (h.Version_status & 0x07) | (uint8(version) << 3 & 0xf8)
	return nil
}

func (h *Header) GetVersion() int {
	return int(h.Version_status >> 3 & 0x1f)
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
	h.Version_status = (h.Version_status & 0xf8) | (uint8(status) & 0x07)
	return nil
}

func (h *Header) GetStatus() int {
	return int(h.Version_status & 0x07)
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

// build header with common set
func BuildHeader(msgType int, proxy bool, serialize int, requestId uint64, msgStatus int) *Header {
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

	header := &Header{MotanMagic, mtype, status, serial, requestId}
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

// decode one message from buffer
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
	body := make([]byte, bodysize)
	if bodysize > 0 {
		body = reqbuf.Next(int(bodysize))
	}
	msg := &Message{header, metamap, body, Req}
	return msg
}

// decode one message from reader
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
	body := make([]byte, bodysize)
	if bodysize > 0 {
		body, err = readBytes(buf, int(bodysize))
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

// encodeGzip
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
	} else {
		return data, nil
	}

}

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
	} else {
		return data, nil
	}
}

func ConvertToRequest(request *Message, serialize motan.Serialization) (motan.Request, error) {
	motanRequest := &motan.MotanRequest{Arguments: make([]interface{}, 0)}
	motanRequest.RequestId = request.Header.RequestId
	motanRequest.ServiceName = request.Metadata[M_path]
	motanRequest.Method = request.Metadata[M_method]
	motanRequest.MethodDesc = request.Metadata[M_methodDesc]
	motanRequest.Attachment = request.Metadata
	rc := motanRequest.GetRpcContext(true)
	rc.OriginalMessage = request
	rc.Proxy = request.Header.IsProxy()
	if request.Body != nil && len(request.Body) > 0 {
		if request.Header.IsGzip() {
			request.Body = DecodeGzipBody(request.Body)
			request.Header.SetGzip(false)
		}
		if serialize == nil {
			return nil, errors.New("serialization is nil!")
		}
		dv := &motan.DeserializableValue{Body: request.Body, Serialization: serialize}
		motanRequest.Arguments = []interface{}{dv}

	}

	return motanRequest, nil
}

func ConvertToReqMessage(request motan.Request, serialize motan.Serialization) (*Message, error) {
	rc := request.GetRpcContext(false)
	if rc != nil && rc.Proxy && rc.OriginalMessage != nil {
		if msg, ok := rc.OriginalMessage.(*Message); ok {
			msg.Header.SetProxy(true)
			return msg, nil
		}
	}

	haeder := BuildHeader(Req, false, serialize.GetSerialNum(), request.GetRequestId(), Normal)
	req := &Message{Header: haeder, Metadata: make(map[string]string)}

	if len(request.GetArguments()) > 0 {
		if serialize == nil {
			return nil, errors.New("serialization is nil.")
		}
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
	req.Metadata[M_path] = request.GetServiceName()
	req.Metadata[M_method] = request.GetMethod()
	if request.GetAttachment(M_proxyProtocol) == "" {
		req.Metadata[M_proxyProtocol] = "motan2"
	}

	if request.GetMethodDesc() != "" {
		req.Metadata[M_methodDesc] = request.GetMethodDesc()
	}
	return req, nil

}

func ConvertToResMessage(response motan.Response, serialize motan.Serialization) (*Message, error) {
	rc := response.GetRpcContext(false)

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
		response.SetAttachment(M_exception, ExceptionToJson(response.GetException()))
	} else {
		msgType = Normal
	}
	res.Header = BuildHeader(Res, false, serialize.GetSerialNum(), response.GetRequestId(), msgType)
	if response.GetValue() != nil {
		if serialize == nil {
			return nil, errors.New("serialization is nil.")
		}
		b, err := serialize.Serialize(response.GetValue())
		if err != nil {
			return nil, err
		}
		res.Body = b
	}
	res.Metadata = response.GetAttachments()

	if rc != nil {
		if rc.GzipSize > 0 && len(res.Body) > rc.GzipSize {
			data, err := EncodeGzip(res.Body)
			if err != nil {
				vlog.Errorf("encode gzip fail! requestid:%d, err %s\n", response.GetRequestId(), err.Error())
			} else {
				res.Header.SetGzip(true)
				res.Body = data
			}
		}
		if rc.Proxy {
			res.Header.SetProxy(true)
		}
	}

	res.Header.SetSerialize(serialize.GetSerialNum())
	return res, nil
}

func ConvertToResponse(response *Message, serialize motan.Serialization) (motan.Response, error) {
	mres := &motan.MotanResponse{}
	rc := mres.GetRpcContext(true)

	mres.RequestId = response.Header.RequestId
	if response.Header.GetStatus() == Normal && len(response.Body) > 0 {
		if response.Header.IsGzip() {
			response.Body = DecodeGzipBody(response.Body)
			response.Header.SetGzip(false)
		}
		if serialize == nil {
			return nil, errors.New("serialization is nil.")
		}
		dv := &motan.DeserializableValue{Body: response.Body, Serialization: serialize}
		mres.Value = dv
	}
	if response.Header.GetStatus() == Exception && response.Metadata[M_exception] != "" {
		var exception *motan.Exception
		err := json.Unmarshal([]byte(response.Metadata[M_exception]), &exception)
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

func BuildExceptionResponse(requestId uint64, errmsg string) *Message {
	header := BuildHeader(Res, false, defaultSerialize, requestId, Exception)
	msg := &Message{Header: header, Metadata: make(map[string]string)}
	msg.Metadata[M_exception] = errmsg
	return msg
}

func ExceptionToJson(e *motan.Exception) string {
	errmsg, _ := json.Marshal(e)
	return string(errmsg)
}
