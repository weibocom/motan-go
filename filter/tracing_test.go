package filter

import (
	"fmt"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/core"
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

func TestTracingFilter_FilterOutgoingRequst(t *testing.T) {
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

	outgoing := func(caller core.Caller, request core.Request) core.Response {
		assert.Equal(t, "1", request.GetAttachment("tid"))
		assert.Equal(t, "2", request.GetAttachment("pid"))
		assert.Equal(t, "a", request.GetAttachment("id"))
		return &MockResponse{}
	}

	referrer := Referrer{
		name: "foo referrer",
		call: outgoing,
		url:  core.URL{Host: "1.2.3.4", Port: 8065, Group: "test-group"},
	}

	tracer := &MockTracer{lastid: 10}
	opentracing.SetGlobalTracer(tracer)

	mockFilter := MockFilter{
		filter: outgoing,
	}

	previous := len(tracer.spans)

	filter := &TracingFilter{next: &mockFilter}
	filter.Filter(&referrer, &req)

	current := len(tracer.spans)
	assert.Equal(t, previous+1, current)

	if span, ok := tracer.spans[current-1].(*MockSpan); ok {
		assert.Equal(t, "1", span.context.traceId)
		assert.Equal(t, "2", span.context.parentId)
		assert.Equal(t, "a", span.context.id)
		assert.Equal(t, ext.SpanKindRPCClientEnum, span.tags[string(ext.SpanKind)])
		assert.True(t, span.finished)
	}
}

func TestTracingFilter_FilterIncomingRequst(t *testing.T) {
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

	outgoing := func(caller core.Caller, request core.Request) core.Response {
		// check the request passed to next filter
		assert.Equal(t, "1", request.GetAttachment("tid"))
		assert.Equal(t, "2", request.GetAttachment("id"))
		assert.Equal(t, "3", request.GetAttachment("pid"))
		return &MockResponse{}
	}

	provider := Provider{
		url: &core.URL{Host: "1.2.3.4", Port: 8065, Group: "test-group"},
	}

	tracer := &MockTracer{lastid: 10}
	opentracing.SetGlobalTracer(tracer)

	mockFilter := MockFilter{
		filter: outgoing,
	}

	previous := len(tracer.spans)

	filter := &TracingFilter{next: &mockFilter}
	filter.Filter(&provider, &req)

	current := len(tracer.spans)
	assert.Equal(t, previous+1, current)

	if span, ok := tracer.spans[current-1].(*MockSpan); ok {
		// check recorded span
		assert.Equal(t, "1", span.context.traceId)
		assert.Equal(t, "2", span.context.id)
		assert.Equal(t, "3", span.context.parentId)
		assert.Contains(t, span.tags, string(ext.SpanKind))
		assert.Equal(t, ext.SpanKindRPCServerEnum, span.tags[string(ext.SpanKind)])
		assert.True(t, span.finished)
	}
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
	name     string
	context  SpanContext
	tags     map[string]interface{}
	logs     map[string]log.Field
	tracer   *MockTracer
	kind     string
	finished bool
}

func (s *MockSpan) Finish() {
	s.finished = true
}

func (s *MockSpan) FinishWithOptions(opts opentracing.FinishOptions) {
	s.finished = true
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
	spans  []opentracing.Span
	lastid uint64
}

func (t *MockTracer) id() uint64 {
	defer func() { t.lastid++ }()
	return t.lastid
}

func (t *MockTracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	options := opentracing.StartSpanOptions{}
	for _, opt := range opts {
		opt.Apply(&options)
	}

	var ctxt *SpanContext
	if options.References != nil && len(options.References) > 0 {
		ref := options.References[0].ReferencedContext

		if c, ok := ref.(*SpanContext); ok {
			if options.Tags[string(ext.SpanKind)] == ext.SpanKindRPCServerEnum {
				ctxt = &SpanContext{traceId: c.traceId, parentId: c.parentId, id: c.id}
			} else {
				id := fmt.Sprintf("%x", t.id())
				ctxt = &SpanContext{traceId: c.traceId, parentId: c.id, id: id}
			}
		}
	}
	if ctxt == nil {
		id := fmt.Sprintf("%x", t.id())
		ctxt = &SpanContext{traceId: id, id: id}
	}

	span := &MockSpan{context: *ctxt, name: operationName, tracer: t, tags: options.Tags, finished: false}

	t.spans = append(t.spans, span)

	return span
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

type Provider struct {
	available bool
	handler   func(request core.Request) core.Response
	url       *core.URL
}

func (p *Provider) SetService(s interface{}) {
	if f, ok := s.(func(request core.Request) core.Response); ok {
		p.handler = f
	}
}

func (p *Provider) GetURL() *core.URL {
	return p.url
}

func (p *Provider) SetURL(url *core.URL) {
	p.url = url
}

func (p *Provider) IsAvailable() bool {
	return p.available
}

func (p *Provider) Call(request core.Request) core.Response {
	return p.handler(request)
}

func (p *Provider) Destroy() {
	p.available = false
}

func (*Provider) GetPath() string {
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
