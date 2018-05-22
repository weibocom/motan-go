package filter

import (
	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	"github.com/weibocom/motan-go/core"
);

// TracingFilter用来拦截所有的RPC请求，包括接收的请求和发出的请求
//
//    1. 接收的请求，当前作为服务端，此时caller应该为core.Provider类型
//    2. 发出的请求，当前作为客户端，此时caller应该为core.Endpoint类型
//
// 正常情况下，假设有一个调用链为
//
//               (1)                 (2)
//    caller -----------> worker -----------> dependents
//             [span1]             [span2]
//
// 此时span1为span2的parent。
//
// 那么将trace功能设置在agent之后，服务不再需要做trace的事情，只需要将trace相关数据透传。如下：
//
//
//                                agent
//                              +-------+
//                      span1   |       |  span1
//                   *----------+- in <-+---------- user
//                   |          |       |
//                   V          |       |
//             | -------        |       |
//   pass-thru | service        |       |
//             V -------        |       |
//                   |          |       |
//                   |  span1   |       |  span2
//                   *----------+> out -+---------> dep
//                              |       |
//                              +-------+
//
// 该功能是会影响现有功能的，影响包括两方面
//
//    1. 如果业务已经有了trace功能，那么对外调用的时候，它就已经用了合适的span信息了。
//       该Filter对对外调用的trace信息的更改，会影响trace的正确性。
//    2. 如果业务已经有了trace功能，业务会记录一遍trace相关的信息，而agent也会记录一遍
//       会导致trace信息重复
//
type TracingFilter struct {
	next core.EndPointFilter
}

func (t *TracingFilter) SetNext(nextFilter core.EndPointFilter) {
	t.next = nextFilter
}

func (t *TracingFilter) GetNext() core.EndPointFilter {
	return t.next
}

func (cf *TracingFilter) Filter(caller core.Caller, request core.Request) core.Response {
	switch caller.(type) {
	case core.Provider:
		return cf.filterForProvider(caller.(core.Provider), request)
	case core.EndPoint:
		return cf.filterForClient(caller.(core.EndPoint), request)
	default:
		return caller.Call(request)
	}
}

func (cf *TracingFilter) filterForClient(caller core.EndPoint, request core.Request) core.Response {
	sc, err := ot.GlobalTracer().Extract(ot.TextMap, AttachmentReader{attach: request})
	var span ot.Span
	if err == ot.ErrSpanContextNotFound {
		// 如果请求中没有span信息，则创建一个根span
		span = ot.StartSpan(spanName(&request), ext.SpanKindRPCClient)
	} else {
		// 如果请求中有span信息，则创建该span对应的一个子span
		span = ot.StartSpan(spanName(&request), ot.ChildOf(sc), ext.SpanKindRPCClient)
	}
	defer span.Finish()

	span.SetTag("ca", core.GetLocalIP())
	span.SetTag("peer.ipv4", caller.GetURL().Host)
	span.SetTag("peer.port", caller.GetURL().Port)

	ot.GlobalTracer().Inject(span.Context(), ot.TextMap, AttachmentWriter{attach: request})

	defer recoverFromPanic(&span)

	var response = callNext(cf, caller, request)

	if response.GetException() != nil {
		span.SetTag("error", true)
		span.LogFields(log.Int("error.kind", response.GetException().ErrType))
		span.LogFields(log.String("message", response.GetException().ErrMsg))
	}
	return response
}

func (cf *TracingFilter) filterForProvider(caller core.Provider, request core.Request) core.Response {
	sc, _ := ot.GlobalTracer().Extract(ot.TextMap, AttachmentReader{attach: request})
	var span ot.Span
	// 如果请求中没有span信息，则创建一个根span
	// 如果请求中有span信息，则创建该span对应的一个子span
	span = ot.StartSpan(spanName(&request), ext.RPCServerOption(sc))
	defer span.Finish()

	span.SetTag("ca", core.GetLocalIP())

	ot.GlobalTracer().Inject(span.Context(), ot.TextMap, AttachmentWriter{attach: request})

	defer recoverFromPanic(&span)

	response := callNext(cf, caller, request)

	if response.GetException() != nil {
		span.SetTag("error", true)
		span.LogFields(log.Int("error.kind", response.GetException().ErrType))
		span.LogFields(log.String("message", response.GetException().ErrMsg))
	}
	return response
}

func callNext(cf *TracingFilter, caller core.Caller, request core.Request) core.Response {
	var response core.Response
	if next := cf.GetNext(); next != nil {
		response = next.Filter(caller, request)
	} else {
		response = caller.Call(request)
	}
	return response
}

func recoverFromPanic(span *ot.Span) {
	if r := recover(); r != nil {
		(*span).SetTag("error", true)
		(*span).LogFields(log.Int("error.kind", core.ServiceException))
		(*span).LogFields(log.Object("message", r))
		panic(r)
	}
}

func spanName(request *core.Request) string {
	return (*request).GetServiceName() + "." + (*request).GetMethod()
}

func (*TracingFilter) GetName() string {
	return "TracingFilter"
}

func (t *TracingFilter) NewFilter(url *core.URL) core.Filter {
	return t
}

func (t *TracingFilter) HasNext() bool {
	return t.next != nil
}

func (*TracingFilter) GetIndex() int {
	return 2
}

func (*TracingFilter) GetType() int32 {
	return core.EndPointFilterType
}

type AttachmentReader struct {
	attach core.Attachment
}

func (a *AttachmentReader) ForeachKey(handler func(key, val string) error) error {
	att := a.attach.GetAttachments()
	if att == nil {
		return nil
	}

	for k, v := range att {
		err := handler(k, v)
		if err != nil {
			return err
		}
	}

	return nil
}

type AttachmentWriter struct {
	attach core.Attachment
}

func (a *AttachmentWriter) Set(key, val string) {
	a.attach.SetAttachment(key, val)
}
