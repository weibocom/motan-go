package server

import (
	"bufio"
	"errors"
	"net"
	"strconv"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	mpro "github.com/weibocom/motan-go/protocol"
)

type MotanServer struct {
	Url           *motan.Url
	handler       motan.MessageHandler
	listener      net.Listener
	extFactory    motan.ExtentionFactory
	serialization motan.Serialization
	proxy         bool
}

func (m *MotanServer) Open(block bool, proxy bool, handler motan.MessageHandler, extFactory motan.ExtentionFactory) error {
	lis, err := net.Listen("tcp", ":"+strconv.Itoa(int(m.Url.Port)))
	if err != nil {
		vlog.Errorf("listen port:%d fail. err: %v\n", m.Url.Port, err)
		return err
	}
	m.listener = lis
	m.handler = handler
	m.extFactory = extFactory
	m.proxy = proxy
	m.serialization = motan.GetSerialization(m.Url, m.extFactory)
	vlog.Infof("motan server is started. port:%d\n", m.Url.Port)
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

func (m *MotanServer) GetUrl() *motan.Url {
	return m.Url
}

func (m *MotanServer) SetUrl(url *motan.Url) {
	m.Url = url
}

func (m *MotanServer) GetName() string {
	return "motan2"
}

func (m *MotanServer) Destroy() {
	err := m.listener.Close()
	if err != nil {
		vlog.Errorf("motan server destroy fail.url %v, err :%s\n", m.Url, err.Error())
	} else {
		vlog.Infof("motan server destroy sucess.url %v\n", m.Url)
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
		res = mpro.BuildHeartbeat(request.Header.RequestId, mpro.Res)
	} else {
		var mres motan.Response
		serialization := m.extFactory.GetSerialization("", request.Header.GetSerialize())
		req, err := mpro.ConvertToRequest(request, serialization)
		req.GetRpcContext(true).ExtFactory = m.extFactory
		if err != nil {
			vlog.Errorf("motan server convert to motan request fail. rid :%d, service: %s, method:%s,err:%s\n", request.Header.RequestId, request.Metadata[mpro.M_path], request.Metadata[mpro.M_method], err.Error())
			mres = motan.BuildExceptionResponse(request.Header.RequestId, &motan.Exception{ErrCode: 500, ErrMsg: "deserialize fail. method:" + request.Metadata[mpro.M_method], ErrType: motan.ServiceException})
		} else {
			mres = m.handler.Call(req)
			//TOOD oneway
		}
		if mres != nil {
			mres.GetRpcContext(true).Proxy = m.proxy
			res, err = mpro.ConvertToResMessage(mres, serialization)
		} else {
			err = errors.New("handler call return nil!")
		}

		if err != nil {
			res = mpro.BuildExceptionResponse(request.Header.RequestId, mpro.ExceptionToJson(&motan.Exception{ErrCode: 500, ErrMsg: "convert to response fail.", ErrType: motan.ServiceException}))
		}
	}
	resbuf := res.Encode()
	conn.Write(resbuf.Bytes())
}
