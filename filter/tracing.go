package filter

import (
	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	"github.com/weibocom/motan-go/core"
);

const (
	IN_BOUND_CALL  = iota
	OUT_BOUND_CALL
)

type CallData struct {
	Caller   core.Caller
	Request  core.Request
	Response core.Response

	// If any error occurred during this call, response not get
	Error interface{}
	// the call direction, please refer to IN_BOUND_CALL and OUT_BOUND_CALL
	Direction uint32
}

// If this function is set, when a call is made, the function will be called
var CustomRecordingFunc func(span *ot.Span, data CallData)

// TracingFilter is designed to support OpenTracing, thus we make use of
// tracing capability many tracing systems (such as zipkin, etc.)
//
// As described by OpenTracing, for a single call from client to server, both sides will start a span,
// with the server side span to be child of the client side. Described as following
//
//                       call
//          caller ---------------> callee
//              [span1]      [span2]
//
// here [span1] is parent of [span2].
//
// When this filter is applied, it will filter both the incoming and
// outgoing requests to record trace information. The following diagram is a demonstration.
//
//                            filter
//                         +---------+
//                         |         |              [span1]
//          [span2]  *-----+-- in <--+--------------------- | user |
//                   |     |         |
//                   V     |         |
//             | -------   |         |
//   pass-thru | service   |         |
//    [span2]  V -------   |         |
//                   |     |         |
//                   |     |         |  [span3]
//          [span2]  *-----+-> out --+--------------------> | dep  |
//                         |         |
//                         +---------+
//
// When the filter receives an incoming request, it will:
//
//     1. extract span context from request (will get [span1])
//     2. start a child span of the extracted span ([span2], child of [span1])
//     3. forward the request with [span2] to the service
//
// Then the service may make an outgoing request to some dependent services,
// it should pass-through the span information ([span2]).
// The filter will receive the outgoing request with [span2], then it will.
//
//     1. extract span context from the outgoing request (it should the [span2])
//     2. start a child span of the extracted span ([span3], child of [span2])
//     3. forward the request with [span3] to the dependent service
//
// So here
//
//              (parent)        (parent)
//      [span1] <------ [span2] <------ [span3]
//
// NOTE:
//
// The tracing capability should not be duplicated, because duplicated tracing will start more than one subsequent span,
// then there will be some unwanted spans in the result.
//
// So the TracingFilter should not be applied more than once.
// and if an existing trace work has been done by the service itself, the TracingFilter should not be used.
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
		// If the request doesn't contain information of a span, then create a root span
		span = ot.StartSpan(spanName(&request), ext.SpanKindRPCClient)
	} else {
		// If the request has contained information of a span, create a child span of the existing span
		span = ot.StartSpan(spanName(&request), ot.ChildOf(sc), ext.SpanKindRPCClient)
	}
	defer span.Finish()

	span.SetTag(string(ext.PeerHostIPv4), caller.GetURL().Host)
	span.SetTag(string(ext.PeerPort), caller.GetURL().Port)
	span.SetTag(string(ext.PeerService), "motan")

	ot.GlobalTracer().Inject(span.Context(), ot.TextMap, AttachmentWriter{attach: request})

	defer handleIfPanic(span)

	defer customRecordError(span, caller, request, OUT_BOUND_CALL)
	var response = callNext(cf, caller, request)

	if CustomRecordingFunc != nil {
		defer CustomRecordingFunc(&span, CallData{Caller: caller, Request: request, Response: response, Error: nil, Direction: OUT_BOUND_CALL})
	}

	if ex := response.GetException(); ex != nil {
		span.SetTag(string(ext.Error), true)
		span.LogFields(log.Int("error.kind", ex.ErrType))
		span.LogFields(log.String("message", ex.ErrMsg))
	}

	return response
}

func customRecordError(span ot.Span, caller core.Caller, request core.Request, direction uint32) {
	if r := recover(); r != nil && CustomRecordingFunc != nil {
		defer CustomRecordingFunc(&span, CallData{Caller: caller, Request: request, Response: nil, Error: r, Direction: direction})
	}
}

func (cf *TracingFilter) filterForProvider(caller core.Provider, request core.Request) core.Response {
	sc, _ := ot.GlobalTracer().Extract(ot.TextMap, AttachmentReader{attach: request})
	var span ot.Span
	// If there is no span information in the request,
	// Just create a root span
	// If a span exists in the request, then start a child span of the existing span
	span = ot.StartSpan(spanName(&request), ext.RPCServerOption(sc))
	defer span.Finish()

	span.SetTag("ca", core.GetLocalIP())
	remoteHost := request.GetAttachment(core.HostKey)
	span.SetTag(string(ext.PeerHostIPv4), remoteHost)

	ot.GlobalTracer().Inject(span.Context(), ot.TextMap, AttachmentWriter{attach: request})

	defer handleIfPanic(span)

	defer customRecordError(span, caller, request, IN_BOUND_CALL)
	response := callNext(cf, caller, request)
	if CustomRecordingFunc != nil {
		defer CustomRecordingFunc(&span, CallData{Caller: caller, Request: request, Response: response, Error: nil, Direction: IN_BOUND_CALL})
	}

	if ex := response.GetException(); ex != nil {
		span.SetTag(string(ext.Error), true)
		span.LogFields(log.Int("error.kind", ex.ErrType))
		span.LogFields(log.String("message", ex.ErrMsg))
	}

	return response
}

func callNext(cf *TracingFilter, caller core.Caller, request core.Request) core.Response {
	var response core.Response
	if next := cf.GetNext(); next != nil {
		response = next.Filter(caller, request)
	}
	return response
}

func handleIfPanic(span ot.Span) {
	if r := recover(); r != nil {
		span.SetTag(string(ext.Error), true)
		span.LogFields(log.Int("error.kind", core.ServiceException))
		span.LogFields(log.Object("message", r))
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
	return &TracingFilter{}
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

// AttachmentReader is used to read the Attachment.
// use value type, to decrease the number of escaped variables
type AttachmentReader struct {
	attach core.Attachment
}

func (a AttachmentReader) ForeachKey(handler func(key, val string) error) error {
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

func (a AttachmentWriter) Set(key, val string) {
	a.attach.SetAttachment(key, val)
}
