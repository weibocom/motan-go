package filter

import (
	"fmt"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/core"
	"math/rand"
	"testing"
)

func TestAttachmentReader_ForeachKey(t *testing.T) {
	att := TestAttachment{}
	att.SetAttachment("k1", "v1")
	att.SetAttachment("k2", "v2")

	r := AttachmentReader{attach: &att}

	n := 0
	r.ForeachKey(func(key, val string) error {
		defer func() { n++ }()
		assert.Equal(t, att[key], val)
		return nil
	})
	assert.Equal(t, len(att), n)
}

func TestAttachmentWriter_Set(t *testing.T) {
	cases :=
		[]struct {
			K, V string
		}{
			{"a", "b"},
			{"c", "d"},
			{"a", "h"},
			{"e", "f"},
		}

	att := TestAttachment{}

	w := AttachmentWriter{attach: &att}

	for _, e := range cases {
		w.Set(e.K, e.V)
		assert.Equal(t, e.V, att[e.K])
	}
}

// This test case needs to mock a lot of things:
//
//
//    * Tracer
//    * Request
//	  * Response
//    * caller
//
// But the infrastructure is not enough, So disable it temporary
func TestTracingFilter_Filter(t *testing.T) {
	req := core.MotanRequest{
		RequestID: 1,
		Attachment: map[string]string{
			"tid": "1",
			"id":  "2",
			"pid": "3",
		},
		Method:      "foo",
		ServiceName: "FooService",
		Arguments:   []interface{}{},
		MethodDesc:  "FooService.foo()",
		RPCContext:  &core.RPCContext{},
	}

	filterFunc := func(caller core.Caller, request core.Request) core.Response {
		assert.Equal(t, request.GetAttachment("tid"), "1")
		assert.Equal(t, request.GetAttachment("pid"), "2")
		return &MockResponse{}
	}

	referrer := Referrer{
		name: "foo referrer",
		call: filterFunc,
		url:  core.URL{Host: "1.2.3.4", Port: 8065, Group: "test-group"},
	}

	tracer := &MockTracer{}
	opentracing.SetGlobalTracer(tracer)

	mockFilter := MockFilter{
		filter: filterFunc,
	}
	filter := &TracingFilter{next: &mockFilter}
	filter.Filter(&referrer, &req)

}

type SpanContext struct {
	traceId, id, parentId string
}

func (c SpanContext) isParentFor(ctx *SpanContext) bool {
	if ctx == nil {
		return false
	} else if ctx.traceId != c.traceId {
		return false
	} else if ctx.parentId != c.id {
		return false
	} else {
		return true
	}
}

func (c *SpanContext) ForeachBaggageItem(handler func(k, v string) bool) {
	handler("tid", c.traceId)
	handler("id", c.id)
	handler("pid", c.parentId)
}

type MockSpan struct {
	name    string
	context SpanContext
	tags    map[string]interface{}
	logs    map[string]log.Field
	tracer  *MockTracer
	kind    string
}

func (*MockSpan) Finish() {

}

func (*MockSpan) FinishWithOptions(opts opentracing.FinishOptions) {

}

func (s *MockSpan) Context() opentracing.SpanContext {
	return &s.context
}

func (s *MockSpan) SetOperationName(operationName string) opentracing.Span {
	s.name = operationName
	return s
}

func (s *MockSpan) SetTag(key string, value interface{}) opentracing.Span {
	if s.tags == nil {
		s.tags = make(map[string]interface{})
	}
	s.tags[key] = value
	return s
}

func (s *MockSpan) LogFields(fields ...log.Field) {
	ensureLogField(s)
	for _, f := range fields {
		s.logs[f.Key()] = f
	}
}

func ensureLogField(s *MockSpan) {
	if s.logs == nil {
		s.logs = make(map[string]log.Field)
	}
}

func (s *MockSpan) LogKV(alternatingKeyValues ...interface{}) {
	ensureLogField(s)

}

func (s *MockSpan) SetBaggageItem(restrictedKey, value string) opentracing.Span {
	return s
}

func (s *MockSpan) BaggageItem(restrictedKey string) string {
	panic("implement me")
}

func (s *MockSpan) Tracer() opentracing.Tracer {
	return s.tracer
}

func (*MockSpan) LogEvent(event string) {
	panic("implement me")
}

func (*MockSpan) LogEventWithPayload(event string, payload interface{}) {
	panic("implement me")
}

func (*MockSpan) Log(data opentracing.LogData) {
	panic("implement me")
}

type MockTracer struct {
}

func (t *MockTracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	options := opentracing.StartSpanOptions{}
	for _, opt := range opts {
		opt.Apply(&options)
	}

	id := fmt.Sprintf("%x", rand.Uint64())
	var ctxt *SpanContext
	if options.References != nil && len(options.References) > 0 {
		ref := options.References[0].ReferencedContext
		if c, ok := ref.(*SpanContext); ok {
			ctxt = &SpanContext{traceId: c.traceId, parentId: c.id, id: id}
		}
	}
	if ctxt == nil {
		ctxt = &SpanContext{traceId: id, id: id}
	}

	return &MockSpan{context: *ctxt, name: operationName, tracer: t, tags: options.Tags}
}

func (*MockTracer) Inject(sm opentracing.SpanContext, format interface{}, carrier interface{}) error {
	switch format {
	case opentracing.TextMap:
		if w, ok := carrier.(opentracing.TextMapWriter); ok {
			sm.ForeachBaggageItem(func(k, v string) bool {
				w.Set(k, v)
				return true
			})
		}
	case opentracing.Binary:
	case opentracing.HTTPHeaders:
	}
	return nil
}

func (*MockTracer) Extract(format interface{}, carrier interface{}) (opentracing.SpanContext, error) {
	switch format {
	case opentracing.TextMap:
		if r, ok := (carrier).(opentracing.TextMapReader); ok {
			var id, traceId, parentId string
			r.ForeachKey(func(key, val string) error {
				switch key {
				case "tid":
					traceId = val
				case "id":
					id = val
				case "pid":
					parentId = val
				}
				return nil
			})
			return &SpanContext{traceId: traceId, id: id, parentId: parentId}, nil
		}
	}
	return nil, nil
}

type MockResponse struct {
}

func (*MockResponse) GetAttachment(key string) string {
	panic("implement me")
}

func (*MockResponse) GetAttachments() map[string]string {
	panic("implement me")
}

func (*MockResponse) GetException() *core.Exception {
	return nil
}

func (*MockResponse) GetProcessTime() int64 {
	panic("implement me")
}

func (*MockResponse) GetRPCContext(canCreate bool) *core.RPCContext {
	panic("implement me")
}

func (*MockResponse) GetRequestID() uint64 {
	panic("implement me")
}

func (*MockResponse) GetValue() interface{} {
	panic("implement me")
}

func (*MockResponse) ProcessDeserializable(toType interface{}) error {
	panic("implement me")
}

func (*MockResponse) SetAttachment(key string, value string) {
	panic("implement me")
}

func (*MockResponse) SetProcessTime(time int64) {
	panic("implement me")
}

type MockFilter struct {
	filter func(caller core.Caller, request core.Request) core.Response
}

func (*MockFilter) SetNext(nextFilter core.EndPointFilter) {
	panic("implement me")
}

func (*MockFilter) GetNext() core.EndPointFilter {
	panic("implement me")
}

func (f *MockFilter) Filter(caller core.Caller, request core.Request) core.Response {
	if f.filter != nil {
		return f.filter(caller, request)
	} else {
		panic("mock func 'filter' not set")
	}
}

func (*MockFilter) GetName() string {
	panic("implement me")
}

func (*MockFilter) NewFilter(url *core.URL) core.Filter {
	panic("implement me")
}

func (*MockFilter) HasNext() bool {
	panic("implement me")
}

func (*MockFilter) GetIndex() int {
	panic("implement me")
}

func (*MockFilter) GetType() int32 {
	panic("implement me")
}

type Referrer struct {
	name      string
	url       core.URL
	available bool

	call func(caller core.Caller, request core.Request) core.Response
}

func (r *Referrer) GetName() string {
	return r.name
}

func (r *Referrer) GetURL() *core.URL {
	return &r.url
}

func (r *Referrer) SetURL(url *core.URL) {
	r.url = *url
}

func (r *Referrer) IsAvailable() bool {
	return r.available
}

func (r *Referrer) Call(request core.Request) core.Response {
	if r.call != nil {
		return r.call(nil, request)
	} else {
		panic("call method not found")
	}
}

func (r *Referrer) Destroy() {
	r.available = false
}

func (r *Referrer) SetSerialization(s core.Serialization) {
	// DO NOTHING
}

func (r *Referrer) SetProxy(proxy bool) {
	// DO NOTHING
}

type TestAttachment map[string]string

func (t *TestAttachment) GetAttachments() map[string]string {
	return *t
}

func (t *TestAttachment) GetAttachment(key string) string {
	return (*t)[key]
}

func (t *TestAttachment) SetAttachment(key string, value string) {
	(*t)[key] = value
}
