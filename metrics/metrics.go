package metrics

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/rcrowley/go-metrics"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	// stat event type
	eventCounter int32 = iota
	eventHistograms
)

const (
	elapseLess50ms  = ".Less50ms"
	elapseLess100ms = ".Less100ms"
	elapseLess200ms = ".Less200ms"
	elapseLess500ms = ".Less500ms"
	elapseMore500ms = ".More500ms"
	eventBufferSize = 1024 * 100

	// default value
	defaultEventProcessor = 1
	maxEventProcessor     = 50
	defaultSinkDuration   = 5 * time.Second
	defaultStatGroup      = "motanStat"
)

var (
	// NewStatItem is the factory func for StatItem
	NewStatItem = NewDefaultStatItem
	// escape chars for metrics key
	escapeChars = map[rune]bool{
		'.': true,
		'/': true}

	items     = make(map[string]StatItem, 64)
	itemsLock sync.RWMutex
	start     sync.Once
	rp        = &reporter{
		interval:  defaultSinkDuration,
		processor: defaultEventProcessor, //sink processor size
		eventBus:  make(chan *event, eventBufferSize),
		writers:   make(map[string]StatWriter),
		evtBuf:    &sync.Pool{New: func() interface{} { return new(event) }},
	}
)

type StatItem interface {
	SetService(service string)
	GetService() string
	SetGroup(group string)
	GetGroup() string
	AddCounter(key string, value int64)
	AddHistograms(key string, duration int64)
	Snapshot() Snapshot
	SnapshotAndClear() Snapshot
	LastSnapshot() Snapshot
	SetReport(b bool)
	IsReport() bool
	Remove(key string)
	Clear()
}

type Snapshot interface {
	StatItem
	Count(key string) int64
	Sum(key string) int64
	Max(key string) int64
	Mean(key string) float64
	Min(key string) int64
	P90(key string) float64
	P95(key string) float64
	P99(key string) float64
	P999(key string) float64
	Percentile(key string, v float64) float64
	Percentiles(key string, f []float64) []float64
	RangeKey(f func(k string))
	IsHistogram(key string) bool
	IsCounter(key string) bool
}

type StatWriter interface {
	Write(snapshots []Snapshot) error
}

func GetOrRegisterStatItem(group string, service string) StatItem {
	itemsLock.RLock()
	item := items[group+service]
	itemsLock.RUnlock()
	if item != nil {
		return item
	}
	itemsLock.Lock()
	item = items[group+service]
	if item == nil {
		item = NewStatItem(group, service)
		items[group+service] = item
	}
	itemsLock.Unlock()
	return item
}

func GetStatItem(group string, service string) StatItem {
	itemsLock.RLock()
	defer itemsLock.RUnlock()
	return items[group+service]
}

func NewDefaultStatItem(group string, service string) StatItem {
	return &DefaultStatItem{group: group, service: Escape(service), holder: &RegistryHolder{registry: metrics.NewRegistry()}, isReport: true}
}

func RMStatItem(group string, service string) {
	itemsLock.RLock()
	i := items[group+service]
	itemsLock.RUnlock()
	if i != nil {
		i.Clear()
		itemsLock.Lock()
		delete(items, group+service)
		itemsLock.Unlock()
	}
}

func ClearStatItems() {
	itemsLock.Lock()
	old := items
	items = make(map[string]StatItem, 64)
	itemsLock.Unlock()
	for _, item := range old {
		item.Clear()
	}
}

func RangeAllStatItem(f func(k string, v StatItem) bool) {
	itemsLock.RLock()
	defer itemsLock.RUnlock()
	var b bool
	for k, i := range items {
		b = f(k, i)
		if !b {
			return
		}
	}
}

func StatItemSize() int {
	itemsLock.RLock()
	defer itemsLock.RUnlock()
	return len(items)
}

func Escape(s string) string {
	return strings.Map(func(r rune) rune {
		if escapeChars[r] {
			return '_'
		}
		return r
	}, s)
}

func AddCounter(group string, service string, key string, value int64) {
	sendEvent(eventCounter, group, service, key, value)
}

func AddHistograms(group string, service string, key string, duration int64) {
	sendEvent(eventHistograms, group, service, key, duration)
}

func sendEvent(eventType int32, group string, service string, key string, value int64) {
	evt := rp.evtBuf.Get().(*event)
	evt.event = eventType
	evt.key = key
	evt.group = group
	evt.service = service
	evt.value = value
	select {
	case rp.eventBus <- evt:
	default:
		vlog.Warningln("metrics eventBus is full.")
	}
}

func ElapseTimeSuffix(t int64) string {
	switch {
	case t < 50:
		return elapseLess50ms
	case t < 100:
		return elapseLess100ms
	case t < 200:
		return elapseLess200ms
	case t < 500:
		return elapseLess500ms
	default:
		return elapseMore500ms
	}
}

type event struct {
	event   int32
	key     string
	group   string
	service string
	value   int64
}

type RegistryHolder struct {
	registry metrics.Registry
}

type DefaultStatItem struct {
	group        string
	service      string
	holder       *RegistryHolder
	isReport     bool
	lastSnapshot Snapshot
	lock         sync.Mutex
}

func (d *DefaultStatItem) getRegistry() metrics.Registry {
	return (*RegistryHolder)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&d.holder)))).registry
}

func (d *DefaultStatItem) SetService(service string) {
	d.service = service
}

func (d *DefaultStatItem) GetService() string {
	return d.service
}

func (d *DefaultStatItem) SetGroup(group string) {
	d.group = group
}

func (d *DefaultStatItem) GetGroup() string {
	return d.group
}

func (d *DefaultStatItem) AddCounter(key string, value int64) {
	c := d.getRegistry().Get(key)
	if c == nil {
		c = metrics.GetOrRegisterCounter(key, d.getRegistry())
	}
	c.(metrics.Counter).Inc(value)
}

func (d *DefaultStatItem) AddHistograms(key string, duration int64) {
	h := d.getRegistry().Get(key)
	if h == nil {
		h = metrics.GetOrRegisterHistogram(key, d.getRegistry(), metrics.NewExpDecaySample(1024, 0))
	}
	h.(metrics.Histogram).Update(duration)
}

func (d *DefaultStatItem) Snapshot() Snapshot {
	// TODO need real-time snapshot?
	return d.LastSnapshot()
}

func (d *DefaultStatItem) SnapshotAndClear() Snapshot {
	d.lock.Lock()
	defer d.lock.Unlock()
	old := atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&d.holder)), unsafe.Pointer(&RegistryHolder{registry: metrics.NewRegistry()}))
	d.lastSnapshot = &DefaultStatItem{group: d.group, service: d.service, isReport: d.isReport, holder: (*RegistryHolder)(old)}
	return d.lastSnapshot
}

func (d *DefaultStatItem) LastSnapshot() Snapshot {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.lastSnapshot
}

func (d *DefaultStatItem) SetReport(b bool) {
	d.isReport = b
}

func (d *DefaultStatItem) IsReport() bool {
	return d.isReport
}

func (d *DefaultStatItem) Remove(key string) {
	d.getRegistry().Unregister(key)
}

func (d *DefaultStatItem) Clear() {
	d.getRegistry().UnregisterAll()
}

func (d *DefaultStatItem) Count(key string) (i int64) {
	v := d.getRegistry().Get(key)
	if v != nil {
		switch m := v.(type) {
		case metrics.Counter:
			i = m.Count()
		case metrics.Histogram:
			i = m.Count()
		}
	}
	return i
}

func (d *DefaultStatItem) Sum(key string) int64 {
	v := d.getRegistry().Get(key)
	if h, ok := v.(metrics.Histogram); ok {
		return h.Sum()
	}
	return 0
}

func (d *DefaultStatItem) Max(key string) int64 {
	v := d.getRegistry().Get(key)
	if h, ok := v.(metrics.Histogram); ok {
		return h.Max()
	}
	return 0
}

func (d *DefaultStatItem) Mean(key string) float64 {
	v := d.getRegistry().Get(key)
	if h, ok := v.(metrics.Histogram); ok {
		return h.Mean()
	}
	return 0
}

func (d *DefaultStatItem) Min(key string) int64 {
	v := d.getRegistry().Get(key)
	if h, ok := v.(metrics.Histogram); ok {
		return h.Min()
	}
	return 0
}

func (d *DefaultStatItem) P90(key string) float64 {
	return d.Percentile(key, 0.9)
}

func (d *DefaultStatItem) P95(key string) float64 {
	return d.Percentile(key, 0.95)
}

func (d *DefaultStatItem) P99(key string) float64 {
	return d.Percentile(key, 0.99)
}

func (d *DefaultStatItem) P999(key string) float64 {
	return d.Percentile(key, 0.999)
}

func (d *DefaultStatItem) Percentile(key string, f float64) float64 {
	v := d.getRegistry().Get(key)
	if h, ok := v.(metrics.Histogram); ok {
		return h.Percentile(f)
	}
	return 0
}

// Percentiles : return value is nil while key not exist
func (d *DefaultStatItem) Percentiles(key string, f []float64) []float64 {
	v := d.getRegistry().Get(key)
	if h, ok := v.(metrics.Histogram); ok {
		return h.Percentiles(f)
	}
	return nil
}

func (d *DefaultStatItem) RangeKey(f func(k string)) {
	d.getRegistry().Each(func(s string, i interface{}) {
		f(s)
	})
}

func (d *DefaultStatItem) IsHistogram(key string) bool {
	_, ok := d.getRegistry().Get(key).(metrics.Histogram)
	return ok
}

func (d *DefaultStatItem) IsCounter(key string) bool {
	_, ok := d.getRegistry().Get(key).(metrics.Counter)
	return ok
}

type metric struct {
	Period    int
	Processor int
	Graphite  []graphite
}

func StartReporter(ctx *motan.Context) {
	start.Do(func() {
		var m metric
		err := ctx.Config.GetStruct("metrics", &m)
		if err != nil {
			vlog.Warningf("get metrics config fail. %s\n", err.Error())
		} else {
			if m.Period > 0 {
				rp.interval = time.Duration(m.Period) * time.Second
			}
			if m.Processor > 1 && m.Processor <= maxEventProcessor {
				rp.processor = m.Processor
			}
			for _, g := range m.Graphite {
				w := newGraphite(g.Host, g.Name, g.Port)
				AddWriter(g.Name, w)
			}
		}
		for i := 0; i < rp.processor; i++ {
			go rp.eventLoop()
		}
		go rp.sink()

		// panic stat when agent model
		if ctx.AgentURL != nil && ctx.AgentURL.Parameters[motan.ApplicationKey] != "" {
			motan.PanicStatFunc = func() {
				AddCounter(defaultStatGroup, ctx.AgentURL.Parameters[motan.ApplicationKey], "Panic", 1)
			}
		}
	})
}

func AddWriter(key string, sw StatWriter) {
	rp.addWriter(key, sw)
}

type reporter struct {
	eventBus    chan *event
	interval    time.Duration
	processor   int
	writers     map[string]StatWriter
	evtBuf      *sync.Pool
	writersLock sync.RWMutex
}

func (r *reporter) eventLoop() {
	for evt := range r.eventBus {
		r.processEvent(evt)
	}
}

func (r *reporter) addWriter(key string, sw StatWriter) {
	if key != "" && sw != nil {
		r.writersLock.Lock()
		defer r.writersLock.Unlock()
		r.writers[key] = sw
		vlog.Infof("add metrics StatWriter %s\n", key)
	}
}

func (r *reporter) processEvent(evt *event) {
	defer motan.HandlePanic(nil)
	item := GetOrRegisterStatItem(evt.group, evt.service)
	switch evt.event {
	case eventCounter:
		item.AddCounter(evt.key, evt.value)
	case eventHistograms:
		item.AddHistograms(evt.key, evt.value)
	}
}

func (r *reporter) sink() {
	ticker := time.NewTicker(r.interval)
	for range ticker.C {
		func() {
			defer motan.HandlePanic(nil)
			snap := r.snapshot() // must snapshot periodically whatever has writers or not
			if len(snap) > 0 && len(r.writers) > 0 {
				r.writersLock.RLock()
				defer r.writersLock.RUnlock()
				for name, writer := range r.writers {
					if err := writer.Write(snap); err != nil {
						vlog.Errorf("write metrics error. name:%s, err:%v\n", name, err)
					}
				}
			}
		}()
	}
}

func (r *reporter) snapshot() (snapshots []Snapshot) {
	l := StatItemSize()
	if l > 0 {
		snapshots = make([]Snapshot, 0, l)
		RangeAllStatItem(func(k string, v StatItem) bool {
			snapshots = append(snapshots, v.SnapshotAndClear())
			return true
		})
	}
	return snapshots
}
