package protocol

import (
	"bufio"
	"encoding/binary"
	"errors"
	motan "github.com/weibocom/motan-go/core"
	vlog "github.com/weibocom/motan-go/log"
	"io"
	"time"
)

const (
	InnerMotanMagic   = 0xf0f0
	V1L1HeaderLength  = 16 // first level header length
	V1AllHeaderLength = 32 // two levels header length
)

const (
	versionV1         = 0x01
	versionV1Compress = 0x02 // v1 压缩版本，已逐步废弃
)

// object stream type
const (
	OS_MAGIC   uint16 = 0xaced
	OS_VERSION uint16 = 0x0005
	GZIP_MAGIC        = 0x1f8b

	TC_ARRAY         = 0x75
	TC_CLASSDESC     = 0x72
	TC_ENDBLOCKDATA  = 0x78
	TC_NULL          = 0x70
	TC_REFERENCE     = 0x71
	TC_BLOCKDATA     = 0x77
	TC_BLOCKDATALONG = 0x7A
)

// hessian2 const
const (
	BC_STRING_DIRECT  = 0x00
	STRING_DIRECT_MAX = 0x1f
	BC_STRING_SHORT   = 0x30
	STRING_SHORT_MAX  = 0x3ff
)

// v1 flags
const (
	FLAG_REQUEST             = 0x00
	FLAG_RESPONSE            = 0x01
	FLAG_RESPONSE_VOID       = 0x03
	FLAG_RESPONSE_EXCEPTION  = 0x05
	FLAG_RESPONSE_ATTACHMENT = 0x07
	FLAG_OTHER               = 0xFF
)

const (
	HEARTBEAT_INTERFACE_NAME  = "com.weibo.api.motan.rpc.heartbeat"
	HEARTBEAT_METHOD_NAME     = "heartbeat"
	HEARTBEAT_RESPONSE_STRING = HEARTBEAT_METHOD_NAME
)

const MAX_BLOCK_SIZE = 1024

// base binary arrays
var (
	baseV1HeartbeatReq     = []byte{241, 241, 0, 0, 24, 44, 223, 176, 126, 80, 0, 96, 0, 0, 0, 78, 240, 240, 1, 0, 24, 44, 223, 176, 126, 80, 0, 96, 0, 0, 0, 62, 172, 237, 0, 5, 119, 56, 0, 33, 99, 111, 109, 46, 119, 101, 105, 98, 111, 46, 97, 112, 105, 46, 109, 111, 116, 97, 110, 46, 114, 112, 99, 46, 104, 101, 97, 114, 116, 98, 101, 97, 116, 0, 9, 104, 101, 97, 114, 116, 98, 101, 97, 116, 0, 4, 118, 111, 105, 100, 0, 0, 0, 0}
	baseV1HeartbeatRes     = []byte{241, 241, 0, 1, 24, 44, 226, 160, 201, 25, 2, 108, 0, 0, 0, 81, 240, 240, 1, 1, 24, 44, 226, 160, 201, 25, 2, 108, 0, 0, 0, 65, 172, 237, 0, 5, 119, 26, 0, 0, 0, 0, 0, 0, 0, 0, 0, 16, 106, 97, 118, 97, 46, 108, 97, 110, 103, 46, 83, 116, 114, 105, 110, 103, 117, 114, 0, 2, 91, 66, 172, 243, 23, 248, 6, 8, 84, 224, 2, 0, 0, 120, 112, 0, 0, 0, 10, 9, 104, 101, 97, 114, 116, 98, 101, 97, 116}
	hessian2HeartbeatBytes = []byte{9, 104, 101, 97, 114, 116, 98, 101, 97, 116}
	baseV1ExceptionRes     = []byte{241, 241, 0, 1, 24, 46, 120, 216, 224, 128, 0, 1, 0, 0, 1, 70, 240, 240, 1, 5, 24, 46, 120, 216, 224, 128, 0, 1, 0, 0, 1, 54}
	baseV1ResObjectStream  = []byte{172, 237, 0, 5, 119, 61, 0, 0, 0, 0, 0, 0, 0, 2, 0, 51, 99, 111, 109, 46, 119, 101, 105, 98, 111, 46, 97, 112, 105, 46, 109, 111, 116, 97, 110, 46, 101, 120, 99, 101, 112, 116, 105, 111, 110, 46, 77, 111, 116, 97, 110, 83, 101, 114, 118, 105, 99, 101, 69, 120, 99, 101, 112, 116, 105, 111, 110, 117, 114, 0, 2, 91, 66, 172, 243, 23, 248, 6, 8, 84, 224, 2, 0, 0, 120, 112}
	h_exceptionDesc        = []byte{67, 48, 51, 99, 111, 109, 46, 119, 101, 105, 98, 111, 46, 97, 112, 105, 46, 109, 111, 116, 97, 110, 46, 101, 120, 99, 101, 112, 116, 105, 111, 110, 46, 77, 111, 116, 97, 110, 83, 101, 114, 118, 105, 99, 101, 69, 120, 99, 101, 112, 116, 105, 111, 110, 150, 8, 101, 114, 114, 111, 114, 77, 115, 103, 13, 100, 101, 116, 97, 105, 108, 77, 101, 115, 115, 97, 103, 101, 5, 99, 97, 117, 115, 101, 13, 109, 111, 116, 97, 110, 69, 114, 114, 111, 114, 77, 115, 103, 10, 115, 116, 97, 99, 107, 84, 114, 97, 99, 101, 20, 115, 117, 112, 112, 114, 101, 115, 115, 101, 100, 69, 120, 99, 101, 112, 116, 105, 111, 110, 115, 96}
	h_motanErrorMsgDesc    = []byte{67, 48, 43, 99, 111, 109, 46, 119, 101, 105, 98, 111, 46, 97, 112, 105, 46, 109, 111, 116, 97, 110, 46, 101, 120, 99, 101, 112, 116, 105, 111, 110, 46, 77, 111, 116, 97, 110, 69, 114, 114, 111, 114, 77, 115, 103, 147, 6, 115, 116, 97, 116, 117, 115, 9, 101, 114, 114, 111, 114, 99, 111, 100, 101, 7, 109, 101, 115, 115, 97, 103, 101, 97}
)

var (
	ErrWrongMotanVersion    = errors.New("unsupported motan version")
	ErrOSVersion            = errors.New("object stream magic number or version not correct")
	ErrUnsupported          = errors.New("unsupported object stream type")
	ErrNotHasV1Msg          = errors.New("not has MotanV1Message")
	ErrParseV1MsgAttachment = errors.New("parse MotanV1Message attachment fail")
)

type MotanV1Message struct {
	OriginBytes    []byte
	V1InnerVersion byte
	Flag           byte
	Rid            uint64
	InnerLength    int
	objStream      *simpleObjectStream //only for request encode
}

func DecodeMotanV1Request(msg *MotanV1Message) (motan.Request, error) {
	objStream, err := newObjectStream(msg)
	if err != nil {
		return nil, err
	}
	err = objStream.parseReq()
	if err != nil {
		return nil, err
	}
	msg.objStream = objStream
	request := &motan.MotanRequest{
		RequestID:   msg.Rid,
		ServiceName: objStream.service,
		Method:      objStream.method,
		MethodDesc:  objStream.paramDesc,
		Attachment:  objStream.attachment}
	ctx := request.GetRPCContext(true)
	ctx.OriginalMessage = msg
	ctx.IsMotanV1 = true
	return request, nil
}

func EncodeMotanV1Request(req motan.Request, sendId uint64) ([]byte, error) {
	if msg, ok := req.GetRPCContext(true).OriginalMessage.(*MotanV1Message); ok {
		return encodeMotanV1(msg, req.GetAttachments(), sendId), nil
	}
	return nil, ErrNotHasV1Msg
}

func DecodeMotanV1Response(msg *MotanV1Message) (motan.Response, error) {
	objStream, err := newObjectStream(msg)
	if err != nil {
		return nil, err
	}
	err = objStream.parseRes()
	if err != nil {
		return nil, err
	}
	response := &motan.MotanResponse{
		RequestID:   msg.Rid,
		ProcessTime: objStream.processTime,
		Value:       objStream.value,
		Attachment:  objStream.attachment}
	if objStream.hasException {
		if objStream.cName == "com.weibo.api.motan.exception.MotanBizException" { // biz exception
			response.Exception = &motan.Exception{ErrCode: 500, ErrMsg: "v1: has biz exception", ErrType: motan.BizException}
		} else {
			response.Exception = &motan.Exception{ErrCode: 500, ErrMsg: "v1: has exception, class:" + objStream.cName, ErrType: motan.ServiceException}
		}
		vlog.Warningf("v1 exception message: %s", objStream.errMsg)
	}
	ctx := response.GetRPCContext(true)
	ctx.OriginalMessage = msg
	ctx.IsMotanV1 = true
	return response, nil
}

func EncodeMotanV1Response(res motan.Response) ([]byte, error) {
	if msg, ok := res.GetRPCContext(true).OriginalMessage.(*MotanV1Message); ok {
		writeV1Rid(msg.OriginBytes, res.GetRequestID()) //replace sendId with rid
		return msg.OriginBytes, nil
	} else if res.GetException() != nil {
		return BuildV1ExceptionResponse(res.GetRequestID(), "build v1 exception res. org err:"+res.GetException().ErrMsg), nil
	}
	return nil, ErrNotHasV1Msg
}

func ReadV1Message(buf *bufio.Reader, maxContentLength int) (*MotanV1Message, time.Time, error) {
	temp, err := buf.Peek(V1L1HeaderLength)
	start := time.Now() // record time when starting to read data
	if err != nil {
		return nil, start, err
	}
	length := V1L1HeaderLength + int(binary.BigEndian.Uint32(temp[12:]))
	if length < V1AllHeaderLength || length > V1L1HeaderLength+maxContentLength {
		vlog.Errorf("content length over the limit. size:%d", length-V1L1HeaderLength)
		return nil, start, ErrOverSize
	}
	ori := make([]byte, length, length)
	_, err = io.ReadAtLeast(buf, ori, length)
	if err != nil {
		return nil, start, err
	}
	mn := binary.BigEndian.Uint16(ori[16:18])
	if mn != InnerMotanMagic {
		vlog.Errorf("wrong v1 inner magic num:%d", mn)
		return nil, start, ErrMagicNum
	}
	v1InnerVersion := ori[18]
	flag := ori[19]
	rid := binary.BigEndian.Uint64(ori[20:28])
	innerLength := int(binary.BigEndian.Uint32(ori[28:32]))
	if innerLength+V1AllHeaderLength != length {
		vlog.Errorf("inner content length not correct. size:%d, inner length:%d", length-V1L1HeaderLength, innerLength)
		return nil, start, ErrWrongSize
	}
	msg := &MotanV1Message{OriginBytes: ori, V1InnerVersion: v1InnerVersion,
		Flag: flag, Rid: rid, InnerLength: innerLength}
	return msg, start, nil
}

func IsV1HeartbeatReq(req motan.Request) bool {
	if req != nil && req.GetServiceName() == HEARTBEAT_INTERFACE_NAME &&
		req.GetMethod() == HEARTBEAT_METHOD_NAME {
		return true
	}
	return false
}

func IsV1HeartbeatRes(res motan.Response) bool {
	if res != nil {
		if str, ok := res.GetValue().(string); ok {
			return str == HEARTBEAT_RESPONSE_STRING
		}
	}
	return false
}

func BuildV1HeartbeatReq(rid uint64) []byte {
	bytes := make([]byte, len(baseV1HeartbeatReq))
	copy(bytes, baseV1HeartbeatReq)
	writeV1Rid(bytes, rid)
	return bytes
}

func BuildV1HeartbeatRes(rid uint64) []byte {
	bytes := make([]byte, len(baseV1HeartbeatRes))
	copy(bytes, baseV1HeartbeatRes)
	writeV1Rid(bytes, rid)
	return bytes
}

func BuildV1ExceptionResponse(rid uint64, errMsg string) []byte {
	var result []byte
	var byteArrayLengthPos int
	if errMsg != "" {
		buf := motan.NewBytesBuffer(350 + len(errMsg))
		// write v1 header
		buf.Write(baseV1ExceptionRes)
		writeV1Rid(buf.Bytes(), rid)

		// write object stream
		buf.Write(baseV1ResObjectStream)
		byteArrayLengthPos = buf.GetWPos()
		buf.SetWPos(byteArrayLengthPos + 4) // skip byte array length

		// write hessian bytes of exception
		buf.Write(h_exceptionDesc)
		buf.Write([]byte("NNN")) // (three) null fields
		buf.Write(h_motanErrorMsgDesc)
		buf.Write([]byte{201, 247, 212, 39, 17}) // status && error code
		writeHessianString(errMsg, buf)
		buf.Write([]byte("NN")) // null fields
		wpos := buf.GetWPos()

		// set byte array length
		length := wpos - byteArrayLengthPos - 4
		buf.SetWPos(byteArrayLengthPos)
		buf.WriteUint32(uint32(length))

		// set l2 header length
		length = wpos - 32
		buf.SetWPos(28) // l2 header length pos
		buf.WriteUint32(uint32(length))

		// set l1 header length
		length = wpos - 16
		buf.SetWPos(12) // l1 header length pos
		buf.WriteUint32(uint32(length))

		buf.SetWPos(wpos)
		return buf.Bytes()
	}
	return result
}

func encodeMotanV1(msg *MotanV1Message, att *motan.StringMap, rid uint64) []byte {
	stream := msg.objStream
	// re-encode attachment
	attBytes := encodeMotanV1Attachment(att, msg.V1InnerVersion)
	var preSize int
	if stream.isGzip {
		preSize = len(stream.bytes) + len(attBytes)
	} else {
		preSize = len(msg.OriginBytes) + len(attBytes)
	}
	buf := motan.NewBytesBuffer(preSize)
	// write header
	buf.Write(msg.OriginBytes[:V1AllHeaderLength])
	buf.Write(stream.bytes[:4]) // object stream magic num

	// write body
	if stream.argSize == 0 { // no params
		writeBlock(buf, stream.sBlock.bytes[:stream.attachmentPos], attBytes) // write service info  && attachments
	} else {
		writeBlock(buf, stream.sBlock.bytes)                   // write service info
		buf.Write(stream.bytes[stream.argStart:stream.argEnd]) // write params
		writeBlock(buf, attBytes)                              // write attachments
	}

	// re-write rid and length
	ret := buf.Bytes()
	writeV1Rid(ret, rid) // replace rid with sendId for send
	writeV1InnerLength(ret, uint32(len(ret)-V1AllHeaderLength))
	return ret
}

// att should not nil
func encodeMotanV1Attachment(att *motan.StringMap, v1InnerVersion byte) []byte {
	size := att.Len()
	buf := motan.NewBytesBuffer(25 * size)
	// write size
	if v1InnerVersion == versionV1Compress {
		buf.WriteUint16(uint16(size))
	} else {
		buf.WriteUint32(uint32(size))
	}
	if size > 0 {
		// write KV
		att.Range(func(k, v string) bool {
			writeUtfStr(k, buf)
			writeUtfStr(v, buf)
			return true
		})
	}
	return buf.Bytes()
}

func writeUtfStr(str string, buf *motan.BytesBuffer) {
	buf.WriteUint16(uint16(len(str)))
	buf.Write([]byte(str))
}

func writeV1Rid(v1Msg []byte, rid uint64) {
	if len(v1Msg) >= V1AllHeaderLength {
		index := 4 // l1 header rid
		binary.BigEndian.PutUint64(v1Msg[index:index+8], rid)
		index = 20 // l2 header rid
		binary.BigEndian.PutUint64(v1Msg[index:index+8], rid)
	}
}

func writeV1InnerLength(v1Msg []byte, innerLength uint32) {
	if len(v1Msg) >= V1AllHeaderLength {
		index := 12 // l1 header length
		binary.BigEndian.PutUint32(v1Msg[index:index+4], innerLength+16)
		index = 28 // l2 header length
		binary.BigEndian.PutUint32(v1Msg[index:index+4], innerLength)
	}
}

func writeBlock(buf *motan.BytesBuffer, bs ...[]byte) {
	var tRemain int // total remain
	for _, b := range bs {
		tRemain += len(b)
	}
	first := true
	lastBlock := false
	blockPos := 0 // block pos
	wPos := 0     // bytes pos
	var ws int    // write size
	for _, b := range bs {
		if lastBlock { // last block
			buf.Write(b)
			tRemain -= len(b)
		} else {
			r := len(b) // current bytes remain
			for r > 0 {
				if first || blockPos >= MAX_BLOCK_SIZE { // new block
					writeBlockHeader(buf, minInt(tRemain, MAX_BLOCK_SIZE))
					blockPos = 0 // reset block pos for new block
					if tRemain <= MAX_BLOCK_SIZE {
						lastBlock = true
					}
					if first {
						first = false
					}
				}
				ws = minInt(r, MAX_BLOCK_SIZE-blockPos) // should write size
				if ws > 0 {
					wPos = len(b) - r
					buf.Write(b[wPos : wPos+ws])
					r -= ws
					blockPos += ws
					tRemain -= ws
				}
			}
		}
	}
}

func writeBlockHeader(buf *motan.BytesBuffer, l int) {
	if l <= 0xFF {
		buf.WriteByte(TC_BLOCKDATA)
		buf.WriteByte(byte(l))
	} else {
		buf.WriteByte(TC_BLOCKDATALONG)
		buf.WriteUint32(uint32(l))
	}
}

func minInt(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

type simpleObjectStream struct {
	bytes          []byte
	pos            int
	len            int
	flag           byte
	parsed         bool
	v1InnerVersion byte
	isGzip         bool
	attachmentPos  int    // attachment position in sBlock if no params
	sBlock         *block // service info block
	aBlock         *block // attachment block. maybe nil
	argStart       int    // args start index in bytes
	argEnd         int    // args end index in bytes

	// for request
	service    string
	method     string
	paramDesc  string
	argSize    int
	attachment *motan.StringMap

	// for response
	processTime  int64
	hasException bool
	cName        string
	errMsg       string
	value        interface{}
}

func newObjectStream(msg *MotanV1Message) (*simpleObjectStream, error) {
	if msg.V1InnerVersion == 0x02 && len(msg.OriginBytes) > 34 &&
		binary.BigEndian.Uint16(msg.OriginBytes[32:34]) == GZIP_MAGIC { // motan v1compress协议版本
		decodedBytes, err := DecodeGzip(msg.OriginBytes[32:])
		if err != nil {
			return nil, err
		}
		return &simpleObjectStream{bytes: decodedBytes, flag: msg.Flag, len: len(decodedBytes), v1InnerVersion: msg.V1InnerVersion, isGzip: true}, nil
	}
	return &simpleObjectStream{bytes: msg.OriginBytes[V1AllHeaderLength:], flag: msg.Flag, len: len(msg.OriginBytes) - V1AllHeaderLength, v1InnerVersion: msg.V1InnerVersion}, nil
}

func (s *simpleObjectStream) parseReq() error {
	err := s.checkObjectStream()
	if err != nil {
		return err
	}
	// service infos
	infos := make([]string, 3)
	for i := 0; i < 3; i++ {
		infos[i], err = s.readUtfFromBlock(s.sBlock)
		if err != nil {
			return err
		}
		if i == 0 && infos[0] == "1" { // v1 compress method info
			var methodSign string
			methodSign, err = s.readUtfFromBlock(s.sBlock)
			if err != nil {
				return err
			}
			infos[0] = "v1compressMethodSign"
			infos[1] = methodSign
			infos[2] = ""
			break
		}
	}
	s.service = infos[0]
	s.method = infos[1]
	s.paramDesc = infos[2]

	// arguments.
	if s.sBlock.remain() == 0 { //has args, should skip arguments bytes
		s.argStart = s.pos
		checkNext := true
		var t byte
		for checkNext {
			s.argSize++
			t, err = s.peekType()
			if err != nil {
				return err
			}
			switch t {
			case TC_NULL:
				s.pos++
			case TC_ARRAY:
				_, err = s.skipByteArray(false)
			case TC_BLOCKDATA, TC_BLOCKDATALONG: // arguments end
				s.argEnd = s.pos
				s.aBlock, err = s.getBlock() // attachment block
				checkNext = false
				s.argSize--
			default:
				vlog.Errorf("unsupported object stream type, type:%d", t)
				return ErrUnsupported
			}
			if err != nil {
				return err
			}
		}
	} else {
		s.attachmentPos = s.sBlock.pos
	}
	if s.sBlock.remain() > 0 {
		err = s.readAttachments(s.sBlock)
	} else if s.aBlock != nil {
		err = s.readAttachments(s.aBlock)
	} else {
		return ErrParseV1MsgAttachment
	}
	if err != nil {
		return err
	}
	s.parsed = true
	return nil
}

func (s *simpleObjectStream) parseRes() error {
	err := s.checkObjectStream()
	if err != nil {
		return err
	}

	s.processTime, err = s.readLongFromBlock(s.sBlock)
	if err != nil {
		return err
	}
	switch s.flag {
	case FLAG_RESPONSE_EXCEPTION:
		s.hasException = true
		s.cName, _ = s.readUtfFromBlock(s.sBlock) // parse exception class name
		//It cannot be fully deserialized, so try to convert it into a string to provide more information
		s.errMsg = string(s.bytes)
	case FLAG_RESPONSE:
		s.cName, err = s.readUtfFromBlock(s.sBlock)
		if err == nil && s.cName == "java.lang.String" {
			var c []byte
			c, err = s.skipByteArray(true)
			if err != nil {
				vlog.Warningf("parse string response value fail. err:%v", err)
			} else if len(c) == len(hessian2HeartbeatBytes) {
				//check hessian2 heartbeat string
				isHeartbeat := true
				for i := 0; i < len(hessian2HeartbeatBytes); i++ {
					if c[i] != hessian2HeartbeatBytes[i] {
						isHeartbeat = false
						break
					}
				}
				if isHeartbeat {
					s.value = HEARTBEAT_RESPONSE_STRING
				}
			}
		}
	default: // No further parsing required
		break
	}
	s.parsed = true
	return nil
}

func (s *simpleObjectStream) checkObjectStream() error {
	if s.remain() < 4 {
		vlog.Errorf("length is wrong v1 request type, flag:%d", s.flag)
		return ErrOSVersion
	}
	// check stream magic number and version
	if binary.BigEndian.Uint16(s.bytes[:2]) != OS_MAGIC || binary.BigEndian.Uint16(s.bytes[2:4]) != OS_VERSION {
		vlog.Errorf("wrong v1 request type, flag:%d", s.flag)
		return ErrOSVersion
	}
	s.pos += 4
	block, err := s.getBlock()
	if err != nil {
		return err
	}
	s.sBlock = block
	return nil
}

func (s *simpleObjectStream) readUtf() (string, error) {
	l, err := s.readInt16()
	if err != nil {
		return "", err
	}
	var str string
	if l > 0 {
		if s.remain() < int(l) {
			return str, io.EOF
		}
		str = string(s.bytes[s.pos : s.pos+int(l)])
		s.pos += int(l)
	}
	return str, nil
}

func (s *simpleObjectStream) readUtfFromBlock(block *block) (string, error) {
	l, err := s.readInt16FromBlock(block)
	if err != nil {
		return "", err
	}
	var str string
	if l > 0 {
		if block.remain() < int(l) {
			return str, io.EOF
		}
		str = string(block.bytes[block.pos : block.pos+int(l)])
		block.pos += int(l)
	}
	return str, nil
}

func (s *simpleObjectStream) readInt16() (int16, error) {
	if s.remain() < 2 {
		return 0, io.EOF
	}
	i := int16(binary.BigEndian.Uint16(s.bytes[s.pos : s.pos+2]))
	s.pos += 2
	return i, nil
}

func (s *simpleObjectStream) readInt16FromBlock(block *block) (int16, error) {
	if block.remain() < 2 {
		return 0, io.EOF
	}
	i := int16(binary.BigEndian.Uint16(block.bytes[block.pos : block.pos+2]))
	block.pos += 2
	return i, nil
}

func (s *simpleObjectStream) readInt() (int, error) {
	if s.remain() < 4 {
		return 0, io.EOF
	}
	i := int(binary.BigEndian.Uint32(s.bytes[s.pos : s.pos+4]))
	s.pos += 4
	return i, nil
}

func (s *simpleObjectStream) readIntFromBlock(block *block) (int, error) {
	if block.remain() < 4 {
		return 0, io.EOF
	}
	i := int(binary.BigEndian.Uint32(block.bytes[block.pos : block.pos+4]))
	block.pos += 4
	return i, nil
}

func (s *simpleObjectStream) readLong() (int64, error) {
	if s.remain() < 8 {
		return 0, io.EOF
	}
	i := int64(binary.BigEndian.Uint64(s.bytes[s.pos : s.pos+8]))
	s.pos += 8
	return i, nil
}

func (s *simpleObjectStream) readLongFromBlock(block *block) (int64, error) {
	if block.remain() < 8 {
		return 0, io.EOF
	}
	i := int64(binary.BigEndian.Uint64(block.bytes[block.pos : block.pos+8]))
	block.pos += 8
	return i, nil
}

// read attachment from block。
func (s *simpleObjectStream) readAttachments(block *block) (err error) {
	var size int
	if s.v1InnerVersion == versionV1Compress {
		var size16 int16
		size16, err = s.readInt16FromBlock(block)
		size = int(size16)
	} else {
		size, err = s.readIntFromBlock(block)
	}
	if err != nil {
		vlog.Errorf("read v1 attachment size fail. err:%v", err)
		return err
	}
	attachments := motan.NewStringMap(DefaultMetaSize)
	if size > 0 { // has attachments
		for i := 0; i < size; i++ {
			var k, v string
			k, err = s.readUtfFromBlock(block)
			if err != nil {
				vlog.Errorf("read v1 attachment key fail. err:%v", err)
				return err
			}
			v, err = s.readUtfFromBlock(block)
			if err != nil {
				vlog.Errorf("read v1 attachment value fail. err:%v", err)
				return err
			}
			attachments.Store(k, v)
		}
	}
	s.attachment = attachments
	return nil
}

// skip byte array(serialized object)
// the return content bytes will be nil if array length is 0, so check len(bytes) before use it
func (s *simpleObjectStream) skipByteArray(withContent bool) ([]byte, error) {
	if s.remain() < 10 {
		return nil, io.EOF
	}
	t := s.bytes[s.pos+1] // inner type
	switch t {
	case TC_CLASSDESC:
		if s.remain() < 23 { // class desc (19) + array length(4)
			vlog.Errorf("object stream: not enough bytes for parse TC_CLASSDESC, need size > 19, remain:%d", s.remain())
			return nil, io.EOF
		}
		s.pos += 19 // skip TC_CLASSDESC
	case TC_REFERENCE:
		s.pos += 6 // skip TC_REFERENCE
	default:
		vlog.Errorf("unsupported object stream type in skipByteArray, type:%d", t)
		return nil, ErrUnsupported
	}
	length, err := s.readInt()
	if err != nil {
		return nil, err
	}
	var content []byte
	if length > 0 {
		if withContent {
			content = make([]byte, length, length)
			copy(content, s.bytes[s.pos:s.pos+length])
		}
		s.pos += length
	}
	return content, nil
}

// get data bytes from blocks
func (s *simpleObjectStream) getBlock() (*block, error) {
	if s.remain() < 2 {
		return nil, io.EOF
	}
	t := s.bytes[s.pos]
	s.pos++
	b := &block{}
	var l int
	switch t {
	case TC_BLOCKDATA:
		l = int(s.bytes[s.pos])
		s.pos++
	case TC_BLOCKDATALONG:
		if s.remain() < 4 {
			return nil, io.EOF
		}
		l = int(binary.BigEndian.Uint32(s.bytes[s.pos : s.pos+4]))
		s.pos += 4
	default:
		vlog.Errorf("unsupported object stream type, type:%d", t)
		return nil, ErrUnsupported
	}

	if s.remain() < l {
		return nil, io.EOF
	}
	b.bytes = s.bytes[s.pos:(s.pos + l)]
	s.pos += l
	if l == MAX_BLOCK_SIZE {
		nt, err := s.peekType()
		if err == nil && (nt == TC_BLOCKDATA || nt == TC_BLOCKDATALONG) { // has next block
			buf := motan.NewBytesBuffer(MAX_BLOCK_SIZE * 2)
			buf.Write(b.bytes) // first block
			err = s.appendBlockBytes(buf)
			if err != nil {
				return nil, err
			}
			b.bytes = buf.Bytes()
		}
	}
	return b, nil
}

func (s *simpleObjectStream) appendBlockBytes(buf *motan.BytesBuffer) error {
	checkNext := true
	var l int
	for checkNext {
		if s.remain() < 1 { // no more data
			return nil
		}
		t := s.bytes[s.pos]
		s.pos++
		switch t {
		case TC_BLOCKDATA:
			l = int(s.bytes[s.pos])
			s.pos++
		case TC_BLOCKDATALONG:
			if s.remain() < 4 {
				return io.EOF
			}
			l = int(binary.BigEndian.Uint32(s.bytes[s.pos : s.pos+4]))
			s.pos += 4
		default: // no more blocks
			return nil
		}
		if s.remain() < l {
			return io.EOF
		}
		buf.Write(s.bytes[s.pos:(s.pos + l)])
		s.pos += l
		if l != MAX_BLOCK_SIZE {
			checkNext = false
		}
	}
	return nil
}

func (s *simpleObjectStream) peekType() (byte, error) {
	if s.remain() < 1 {
		return 0, io.EOF
	}
	return s.bytes[s.pos], nil
}

func (s *simpleObjectStream) remain() int {
	return s.len - s.pos
}

type block struct {
	bytes []byte
	pos   int
}

func (b block) len() int {
	return len(b.bytes)
}

func (b block) remain() int {
	return len(b.bytes) - b.pos
}

func writeHessianString(str string, buf *motan.BytesBuffer) {
	// write length
	length := len(str)
	if length <= STRING_DIRECT_MAX {
		buf.WriteByte(byte(BC_STRING_DIRECT + len(str)))
	} else if length <= STRING_SHORT_MAX {
		buf.WriteByte(byte(BC_STRING_SHORT + (length >> 8)))
		buf.WriteByte(byte(length))
	} else {
		buf.WriteByte('S')
		buf.WriteByte(byte(length >> 8))
		buf.WriteByte(byte(length))
	}
	buf.Write([]byte(str))
}
