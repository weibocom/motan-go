package core

import (
	"math/rand"
	"sync"
	"time"
)

var (
	// TracePolicy is trace policy for mesh request, this func is called by each request, trace will enable if this func return a TraceContext
	TracePolicy TracePolicyFunc = NoTrace

	// RandomTraceBase is random base for RandomTrace
	RandomTraceBase        = 10
	MaxTraceSize    uint64 = 10000

	once   sync.Once
	holder *traceHolder
)

type TracePolicyFunc func(rid uint64, ext *StringMap) *TraceContext

// NoTrace : not trace. default trace policy.
func NoTrace(rid uint64, ext *StringMap) *TraceContext {
	return nil
}

// RandomTrace : trace ratio is 1/DefaultRandomTraceBase
func RandomTrace(rid uint64, ext *StringMap) *TraceContext {
	n := rand.Intn(RandomTraceBase)
	if n == 0 {
		return NewTraceContext(rid)
	}
	return nil
}

// AlwaysTrace : trace every request unless the tracecontext size over MaxTraceSize.
func AlwaysTrace(rid uint64, ext *StringMap) *TraceContext {
	return NewTraceContext(rid)
}

type traceHolder struct {
	tcs  []*TraceContext
	max  uint64
	size uint64
	lock sync.Mutex
}

func (t *traceHolder) newTrace(rid uint64) *TraceContext {
	if t.size > t.max {
		return nil
	}
	t.lock.Lock()
	defer t.lock.Unlock()
	if t.size <= t.max {
		tc := &TraceContext{Rid: rid,
			ReqSpans: make([]*Span, 0, 16),
			ResSpans: make([]*Span, 0, 16),
			Values:   make(map[string]interface{}, 16)}
		t.tcs = append(t.tcs, tc)
		t.size++
		return tc
	}
	return nil
}

type TraceContext struct {
	Rid      uint64                 `json:"requestid"`
	Addr     string                 `json:"address"`
	ReqSpans []*Span                `json:"request_spans"`
	ResSpans []*Span                `json:"response_spans"`
	Values   map[string]interface{} `json:"values"`
	lock     sync.Mutex
}

type Span struct {
	Name     string    `json:"name"`
	Addr     string    `json:"address"`
	Time     time.Time `json:"time"`
	Duration int64     `json:"duration"`
}

// NewTraceContext : create a new TraceContext and hold to holder. it will return nil, if TraceContext size of holder is over MaxTraceSize.
func NewTraceContext(rid uint64) *TraceContext {
	once.Do(func() {
		holder = &traceHolder{tcs: make([]*TraceContext, 0, 512), max: MaxTraceSize}
	})
	return holder.newTrace(rid)
}

// PutReqSpan : put a trace Span at request phase
func (t *TraceContext) PutReqSpan(span *Span) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.ReqSpans = append(t.ReqSpans, span)
}

// PutResSpan : put a trace Span at response phase
func (t *TraceContext) PutResSpan(span *Span) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.ResSpans = append(t.ResSpans, span)
}

// GetTraceContexts get && remove all TraceContext in holder, and create a new TraceContext holder.
func GetTraceContexts() []*TraceContext {
	temp := holder
	holder = &traceHolder{tcs: make([]*TraceContext, 0, 1024), max: MaxTraceSize}
	if temp != nil {
		return temp.tcs
	}
	return nil
}
