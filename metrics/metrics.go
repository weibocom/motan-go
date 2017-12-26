package metrics

import (
	"sync"
	"sync/atomic"
	"time"
	"fmt"

	"github.com/weibocom/motan-go/config"
	"github.com/weibocom/motan-go/log"

	"github.com/rcrowley/go-metrics"
)

const (
	eventCounter int32 = iota
	eventMeter
	eventTimer
	eventGauge
	eventHistograms
)

const (
	ElapseLess50ms  = "Less50ms"
	ElapseLess100ms = "Less100ms"
	ElapseLess200ms = "Less200ms"
	ElapseLess500ms = "Less500ms"
	ElapseMore500ms = "More500ms"
	eventBufferSize = 1024 * 100
)

type metric struct {
	Period   int
	Graphite []graphite
}

type event struct {
	event int32
	key   string
	value int64
}

type reporter struct {
	eventBus chan *event
	interval time.Duration
	stopping int32
	registry metrics.Registry
	writers  map[string]statWriter
	samples  map[string]metrics.Sample
	evtBuf   *sync.Pool
}

func (r *reporter) Sample(key string) metrics.Sample {
	if s, ok := r.samples[key]; ok && s != nil {
		return s
	}
	r.samples[key] = metrics.NewExpDecaySample(1028, 0)
	return r.samples[key]
}

var (
	sinkDuration = time.Second * 5
	reg          = &reporter{
		registry: metrics.NewRegistry(),
		stopping: 0,
		samples:  map[string]metrics.Sample{},
		eventBus: make(chan *event, eventBufferSize),
		writers:  make(map[string]statWriter),
		evtBuf:   &sync.Pool{New: func() interface{} { return new(event) }},
	}
)

func Run(cfg *config.Config) error {

	var m metric
	err := cfg.GetStruct("metrics", &m)
	if err != nil {
		vlog.Errorln(err)
		return err
	}
	if m.Period >= 0 {
		sinkDuration = time.Duration(m.Period) * time.Second
	}

	for _, g := range m.Graphite {
		w, err := getWriter(&g)
		if err != nil {
			return err
		}
		reg.writers[g.Name] = w
	}
	go reg.eventLoop()
	return nil
}

func ElapseTimeString(t int64) string {
	switch {
	case t < 50:
		return ElapseLess50ms
	case t < 100:
		return ElapseLess100ms
	case t < 200:
		return ElapseLess200ms
	case t < 500:
		return ElapseLess500ms
	default:
		return ElapseMore500ms
	}
}

func getWriter(g *graphite) (statWriter, error) {
	return newGraphite(g.Host, g.Name, g.Port), nil

}

func (r *reporter) eventLoop() {

	if atomic.LoadInt32(&r.stopping) == 1 {
		return
	}

	ticker := time.NewTicker(sinkDuration)

	for {
		select {
		case evt := <-r.eventBus:
			r.processEvent(evt)
		case <-ticker.C:
			r.sink()
		}
	}
}

func (r *reporter) processEvent(evt *event) {
	switch evt.event {
	case eventCounter:
		metrics.GetOrRegisterCounter(evt.key, r.registry).Inc(evt.value)
	case eventMeter:
		getOrRegisterMeter(evt.key, r.registry).Mark(evt.value)
	case eventTimer:
		metrics.GetOrRegisterTimer(evt.key, r.registry).Update(time.Duration(evt.value))
	case eventHistograms:
		metrics.GetOrRegisterHistogram(evt.key, r.registry, r.Sample(evt.key)).Update(evt.value)

	}
}

func (r *reporter) sink() {

	snap := r.snapshot()
	if snap == nil {
		return
	}

	for name, writer := range r.writers {
		if err := writer.Write(snap); err != nil {
			vlog.Errorln("metrics writer %s error : %v", name, err)
			break
		}
	}

}

func AddCounter(key string, value int64) {
	evt := reg.evtBuf.Get().(*event)
	evt.event = eventCounter
	evt.key = key
	evt.value = value
	select {
	case reg.eventBus <- evt:
	default:
		vlog.Warningln("metrics eventBus is full.")
	}
}
func AddMeter(key string, value int64) {
	evt := reg.evtBuf.Get().(*event)
	evt.event = eventMeter
	evt.key = key
	evt.value = value
	select {
	case reg.eventBus <- evt:
	default:
		vlog.Warningln("metrics eventBus is full.")
	}
}

func AddTimer(key string, duration int64) {
	evt := reg.evtBuf.Get().(*event)
	evt.event = eventTimer
	evt.key = key
	evt.value = duration
	select {
	case reg.eventBus <- evt:
	default:
		vlog.Warningln("metrics eventBus is full.")
	}
}

func AddHistograms(key string, duration int64) {
	evt := reg.evtBuf.Get().(*event)
	evt.event = eventHistograms
	evt.key = key
	evt.value = duration
	select {
	case reg.eventBus <- evt:
	default:
		vlog.Warningln("metrics eventBus is full.")
	}
}

func (r *reporter) snapshot() metrics.Registry {
	hasElement := false
	snap := metrics.NewRegistry()

	r.registry.Each(func(key string, i interface{}) {
		switch m := i.(type) {
		case metrics.Counter:
			snap.Register(key, m.Snapshot())
			m.Clear()
		case metrics.Gauge:
			snap.Register(key, m.Snapshot())
		case metrics.GaugeFloat64:
			snap.Register(key, m.Snapshot())
		case metrics.Histogram:
			snap.Register(key, m.Snapshot())
			m.Clear()
		case metrics.Meter:
			snap.Register(key, m.Snapshot())
		case metrics.Timer:
			snap.Register(key, m.Snapshot())
		}
		hasElement = true
	})
	r.registry.UnregisterAll()
	if !hasElement {
		return nil
	}
	return snap
}
