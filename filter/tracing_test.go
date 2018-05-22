package filter

import (
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
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
		[]struct{
			K, V string
		} {
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
func testTracingFilter_Filter(t *testing.T) {
	req := core.MotanRequest{
		RequestID:   1,
		Attachment:  make(map[string]string),
		Method:      "foo",
		ServiceName: "FooService",
		Arguments:   []interface{}{},
		MethodDesc:  "FooService.foo()",
		RPCContext:  &core.RPCContext{},
	}

	referrer := Referrer{
		name: "foo referrer",
		call: func(request core.Request) core.Response {
			return nil
		},
		url: core.URL{Host: "1.2.3.4", Port: 8065, Group: "test-group"},
	}

	tracer := mocktracer.New()
	opentracing.SetGlobalTracer(tracer)

	filter := &TracingFilter{}
	filter.Filter(&referrer, &req)

}

type Referrer struct {
	name      string
	url       core.URL
	available bool

	call func(request core.Request) core.Response
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
		return r.call(request)
	} else {
		return nil
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
