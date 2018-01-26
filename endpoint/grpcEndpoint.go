package endpoint

import (
	"strconv"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
	"golang.org/x/net/context"
	grpc "google.golang.org/grpc"
	metadata "google.golang.org/grpc/metadata"
)

type GrpcEndPoint struct {
	url      *motan.URL
	grpcConn *grpc.ClientConn
	proxy    bool
}

const (
	GRPCSerialNum = 1
)

func (g *GrpcEndPoint) Initialize() {
	grpcconn, err := grpc.Dial((g.url.Host + ":" + strconv.Itoa((int)(g.url.Port))), grpc.WithInsecure(), grpc.WithCodec(&agentCodec{}))
	if err != nil {
		vlog.Errorf("connect to grpc fail! url:%s, err:%s\n", g.url.GetIdentity(), err.Error())
	}
	g.grpcConn = grpcconn
}

func (g *GrpcEndPoint) Destroy() {
	if g.grpcConn != nil {
		vlog.Infof("grpc endpoint %s will destroyed", g.url.GetAddressStr())
		g.grpcConn.Close()
	}
}

func (g *GrpcEndPoint) SetProxy(proxy bool) {
	g.proxy = proxy
}

func (g *GrpcEndPoint) SetSerialization(s motan.Serialization) {}

func (g *GrpcEndPoint) Call(request motan.Request) motan.Response {
	t := time.Now().UnixNano()
	var in []byte
	if dv, ok := request.GetArguments()[0].(motan.DeserializableValue); ok {
		in = dv.Body
	} else if ba, ok := request.GetArguments()[0].([]byte); ok {
		in = ba
	} else {
		vlog.Errorf("can not process argument in grpc endpoint. argument:%v\n", request.GetArguments()[0])
		return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "grpc argument must be []byte", ErrType: motan.ServiceException})
	}
	out := new(OutMsg)

	var header, trailer metadata.MD
	md := metadata.New(request.GetAttachments())
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	err := grpc.Invoke(ctx, "/"+request.GetServiceName()+"/"+request.GetMethod(), in, out, g.grpcConn, grpc.Header(&header), grpc.Trailer(&trailer))
	// type MD map[string][]string
	resp := &motan.MotanResponse{Attachment: make(map[string]string)}
	resp.RequestID = request.GetRequestID()
	resp.ProcessTime = int64((time.Now().UnixNano() - t) / 1000000)
	rc := resp.GetRPCContext(true)
	rc.Serialized = true
	rc.SerializeNum = GRPCSerialNum

	// @TODO add receiving header and trailers into attachment
	if err != nil {
		errcode := int(grpc.Code(err))
		//@TODO ErrTYpe
		resp.Exception = &motan.Exception{ErrCode: errcode, ErrMsg: grpc.ErrorDesc(err), ErrType: errcode}
		return resp
	}
	resp.Value = (*out).buf
	return resp
}

func (g *GrpcEndPoint) GetName() string {
	return "grpcEndpoint"
}

func (g *GrpcEndPoint) GetURL() *motan.URL {
	return g.url
}

func (g *GrpcEndPoint) SetURL(url *motan.URL) {
	g.url = url
}

func (g *GrpcEndPoint) IsAvailable() bool {
	//TODO endpoint 是否可用
	return true
}

type agentCodec struct{}

type OutMsg struct {
	buf []byte
}

func (agentCodec) Marshal(v interface{}) ([]byte, error) {
	var err error
	rs := v.([]byte)
	return rs, err
}

func (agentCodec) Unmarshal(data []byte, v interface{}) error {
	var err error
	om := v.(*OutMsg)
	om.buf = data
	return err
}

func (agentCodec) String() string {
	return "agent"
}
