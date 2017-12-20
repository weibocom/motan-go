package server

import (
	"strings"
	"bufio"
	"errors"
	"net"
	"strconv"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	mpro "github.com/weibocom/motan-go/protocol"
)

type MotanServer struct {
	URL           *motan.URL
	handler       motan.MessageHandler
	listener      net.Listener
	extFactory    motan.ExtentionFactory
	serialization motan.Serialization
	proxy         bool
}

func (m *MotanServer) Open(block bool, proxy bool, handler motan.MessageHandler, extFactory motan.ExtentionFactory) error {
	lis, err := net.Listen("tcp", ":"+strconv.Itoa(int(m.URL.Port)))
	if err != nil {
		vlog.Errorf("listen port:%d fail. err: %v\n", m.URL.Port, err)
		return err
	}
	m.listener = lis
	m.handler = handler
	m.extFactory = extFactory
	m.proxy = proxy
	m.serialization = motan.GetSerialization(m.URL, m.extFactory)
	vlog.Infof("motan server is started. port:%d\n", m.URL.Port)
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
	if err != nil {
		vlog.Errorf("motan server destroy fail.url %v, err :%s\n", m.URL, err.Error())
	} else {
		vlog.Infof("motan server destroy sucess.url %v\n", m.URL)
	}
}

func (m *MotanServer) run() {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			vlog.Errorf("motan server accept from port %v fail. err:%s\n", m.listener.Addr(), err.Error())
		} else {

			go m.handleConn(conn)
		}
	}
}

func (m *MotanServer) handleConn(conn net.Conn) {
	defer func() {
		if err := recover(); err != nil {
			vlog.Errorln("connection encount error! ", err)
		}
		conn.Close()
	}()
	buf := bufio.NewReader(conn)
	for {
		request, err := mpro.DecodeFromReader(buf)
		if err != nil {
			if err.Error() != "EOF" {
				vlog.Warningf("decode motan message fail! con:%s\n.", conn.RemoteAddr().String())
			}
			break
		}
		go m.processReq(request, conn)
	}
}

func (m *MotanServer) processReq(request *mpro.Message, conn net.Conn) {
	defer func() {
		if err := recover(); err != nil {
			vlog.Errorln("Motanserver processReq error! ", err)
		}
	}()
	request.Header.SetProxy(m.proxy)

	var res *mpro.Message
	if request.Header.IsHeartbeat() {
		res = mpro.BuildHeartbeat(request.Header.RequestID, mpro.Res)
	} else {
		var mres motan.Response
		serialization := m.extFactory.GetSerialization("", request.Header.GetSerialize())
		req, err := mpro.ConvertToRequest(request, serialization)

		var ip string = getRemoteIp(conn.RemoteAddr().String())
		var headers map[string]string = req.GetAttachments()
		headers["host"] = ip

		req.GetRPCContext(true).ExtFactory = m.extFactory
		if err != nil {
			vlog.Errorf("motan server convert to motan request fail. rid :%d, service: %s, method:%s,err:%s\n", request.Header.RequestID, request.Metadata[mpro.MPath], request.Metadata[mpro.MMethod], err.Error())
			mres = motan.BuildExceptionResponse(request.Header.RequestID, &motan.Exception{ErrCode: 500, ErrMsg: "deserialize fail. method:" + request.Metadata[mpro.MMethod], ErrType: motan.ServiceException})
		} else {
			mres = m.handler.Call(req)
			//TOOD oneway
		}
		if mres != nil {
			mres.GetRPCContext(true).Proxy = m.proxy
			res, err = mpro.ConvertToResMessage(mres, serialization)
		} else {
			err = errors.New("handler call return nil")
		}

		if err != nil {
			res = mpro.BuildExceptionResponse(request.Header.RequestID, mpro.ExceptionToJSON(&motan.Exception{ErrCode: 500, ErrMsg: "convert to response fail.", ErrType: motan.ServiceException}))
		}
	}
	resbuf := res.Encode()
	conn.Write(resbuf.Bytes())
}

func getRemoteIp(adress string) string {
	var ip string
	var index int = strings.IndexAny(adress,":")
	if( index > 0){
		s:= adress
		ip = string(s[:index])
	}else{
		ip = adress;
	}
	return ip
}