package metrics

import (
	"github.com/weibocom/motan-go/metrics/sampler"
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
	eventGauge
)

const (
	elapseLess50ms  = ".Less50ms"
	elapseLess100ms = ".Less100ms"
	elapseLess200ms = ".Less200ms"
	elapseLess500ms = ".Less500ms"
	elapseMore500ms = ".More500ms"
	eventBufferSize = 1024 * 100

	// default value
	defaultEventProcessor     = 1
	maxEventProcessor         = 50
	defaultSinkDuration       = 5 * time.Second
	defaultStatusSamplePeriod = 30 * time.Second

	KeyDelimiter           = ":"
	DefaultStatGroup       = "motan-stat"
	DefaultStatService     = "status"
	DefaultStatRole        = "motan-agent"
	DefaultStatApplication = "unknown"

	DefaultRuntimeErrorApplication      = "runtime-error"
	DefaultRuntimeCircuitBreakerGroup   = "circuit-breaker"
	DefaultRuntimeCircuitBreakerService = "endpoint"
)

var (
	metricsKeyBuilderBufferSize = 64
	// NewStatItem is the factory func for StatItem
	NewStatItem = NewDefaultStatItem
	items       = make(map[string]StatItem, 64)
	itemsLock   sync.RWMutex
	start       sync.Once
	rp          = &reporter{
		interval:  defaultSinkDuration,
		processor: defaultEventProcessor, //sink processor size
		eventBus:  make(chan *event, eventBufferSize),
		writers:   make(map[string]StatWriter),
		eventPool: &sync.Pool{New: func() interface{} {
			return &event{}
		}},
	}
	escapeCache sync.Map
)

type StatItem interface {
	SetService(service string)
	GetService() string
	GetEscapedService() string
	SetGroup(group string)
	GetGroup() string
	GetEscapedGroup() string
	GetGroupSuffix() string
	AddCounter(key string, value int64)
	AddHistograms(key string, duration int64)
	AddGauge(key string, value int64)
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
	Value(key string) int64
	RangeKey(f func(k string))
	IsHistogram(key string) bool
	IsCounter(key string) bool
	IsGauge(key string) bool
}

type StatWriter interface {
	Write(snapshots []Snapshot) error
}

func GetOrRegisterStatItem(group, groupSuffix string, service string) StatItem {
	k := group + groupSuffix + service
	item := safeGet(k)
	if item != nil {
		return item
	}
	return safePutAbsent(k, group, groupSuffix, service)
}

func GetStatItem(group, groupSuffix string, service string) StatItem {
	itemsLock.RLock()
	defer itemsLock.RUnlock()
	return items[group+groupSuffix+service]
}

// NewDefaultStatItem create a new statistic item, you should escape input parameter before call this function
func NewDefaultStatItem(group, groupSuffix string, service string) StatItem {
	return &DefaultStatItem{group: group, groupSuffix: groupSuffix, service: service, holder: &RegistryHolder{registry: metrics.NewRegistry()}, isReport: true}
}

func RMStatItem(group, groupSuffix string, service string) {
	k := group + groupSuffix + service
	item := safeGet(k)
	if item != nil {
		safeDelete(k)
		item.Clear()
	}
}

func ClearStatItems() {
	old := safeExchangeNew()
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

func safeGet(k string) StatItem {
	itemsLock.RLock()
	defer itemsLock.RUnlock()
	return items[k]
}

func safePutAbsent(k, group, groupSuffix, service string) StatItem {
	itemsLock.Lock()
	defer itemsLock.Unlock()
	item := items[k]
	if item == nil {
		item = NewStatItem(group, groupSuffix, service)
		items[k] = item
	}
	return item
}

func safeDelete(k string) {
	itemsLock.Lock()
	defer itemsLock.Unlock()
	delete(items, k)
}

func safeExchangeNew() map[string]StatItem {
	itemsLock.Lock()
	defer itemsLock.Unlock()
	old := items
	items = make(map[string]StatItem, 64)
	return old
}

// Escape the string avoid invalid graphite key
func Escape(s string) string {
	if v, ok := escapeCache.Load(s); ok {
		return v.(string)
	}
	v := strings.Map(func(char rune) rune {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || (char == '-') {
			return char
		}
		return '_'
	}, s)
	escapeCache.Store(s, v)
	return v
}

func AddCounter(group string, service string, key string, value int64) {
	sendEvent(eventCounter, group, service, key, value)
}

func AddHistograms(group string, service string, key string, duration int64) {
	sendEvent(eventHistograms, group, service, key, duration)
}

func AddGauge(group string, service string, key string, value int64) {
	sendEvent(eventGauge, group, service, key, value)
}

func AddGaugeWithKeys(group, groupSuffix string, service string, keys []string, keySuffix string, value int64) {
	sendEventWithKeys(eventGauge, group, groupSuffix, service, keys, keySuffix, value)
}

// AddCounterWithKeys arguments: group & groupSuffix & service &  keys elements & keySuffix is text without escaped
func AddCounterWithKeys(group, groupSuffix string, service string, keys []string, keySuffix string, value int64) {
	sendEventWithKeys(eventCounter, group, groupSuffix, service, keys, keySuffix, value)
}

// AddHistogramsWithKeys arguments: group & groupSuffix & service &  keys elements & keySuffix is text without escaped
func AddHistogramsWithKeys(group, groupSuffix string, service string, keys []string, suffix string, duration int64) {
	sendEventWithKeys(eventHistograms, group, groupSuffix, service, keys, suffix, duration)
}

func sendEvent(eventType int32, group string, service string, key string, value int64) {
	sendEventWithKeys(eventType, group, "", service, []string{key}, "", value)
}

func sendEventWithKeys(eventType int32, group, groupSuffix string, service string, keys []string, suffix string, value int64) {
	evt := rp.eventPool.Get().(*event)
	evt.event = eventType
	evt.keys = keys
	evt.group = group
	evt.service = service
	evt.value = value
	evt.keySuffix = suffix
	evt.groupSuffix = groupSuffix
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

func RegisterStatusSampleFunc(key string, sf func() int64) {
	sampler.RegisterStatusSampleFunc(key, sf)
}

func sampleStatus(application string) {
	defer motan.HandlePanic(nil)
	sampler.RangeDo(func(key string, value sampler.StatusSampler) bool {
		AddGaugeWithKeys(DefaultStatGroup, "", DefaultStatService,
			[]string{DefaultStatRole, application, key}, "", value.Sample())
		return true
	})
}

func startSampleStatus(application string) {
	go func() {
		ticker := time.NewTicker(defaultStatusSamplePeriod)
		defer ticker.Stop()
		for range ticker.C {
			sampleStatus(application)
		}
	}()
}

type event struct {
	event       int32
	keys        []string
	keySuffix   string
	group       string
	groupSuffix string
	service     string
	value       int64
}

// reset used to reset the event object before put it back
// to event objects pool
func (s *event) reset() {
	s.event = 0
	s.keys = s.keys[:0]
	s.keySuffix = ""
	s.group = ""
	s.service = ""
	s.value = 0
	s.groupSuffix = ""
}

// getMetricKey get the metrics key when add metrics data into metrics object,
// the key split by : used to when send data to graphite
func (s *event) getMetricKey() string {
	keyBuilder := motan.AcquireBytesBuffer(metricsKeyBuilderBufferSize)
	defer motan.ReleaseBytesBuffer(keyBuilder)
	l := len(s.keys)
	for idx, k := range s.keys {
		keyBuilder.WriteString(Escape(k))
		if idx < l-1 {
			keyBuilder.WriteString(":")
		}
	}
	if s.keySuffix != "" {
		keyBuilder.WriteString(s.keySuffix)
	}
	return string(keyBuilder.Bytes())
}

type RegistryHolder struct {
	registry metrics.Registry
}

type DefaultStatItem struct {
	group        string
	groupSuffix  string
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

// GetEscapedService return the escaped service used as graphite key
func (d *DefaultStatItem) GetEscapedService() string {
	return Escape(d.service)
}

func (d *DefaultStatItem) SetGroup(group string) {
	d.group = group
}

func (d *DefaultStatItem) GetGroup() string {
	return d.group
}

func (d *DefaultStatItem) GetGroupSuffix() string {
	return d.groupSuffix
}

// GetEscapedGroup return the escaped group used as graphite key
func (d *DefaultStatItem) GetEscapedGroup() string {
	return Escape(d.group)
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

func (d *DefaultStatItem) AddGauge(key string, value int64) {
	c := d.getRegistry().Get(key)
	if c == nil {
		c = metrics.GetOrRegisterGauge(key, d.getRegistry())
	}
	c.(metrics.Gauge).Update(value)
}

func (d *DefaultStatItem) Snapshot() Snapshot {
	// TODO need real-time snapshot?
	return d.LastSnapshot()
}

// SnapshotAndClear acquires Snapshot(ReadonlyStatItem), and it calculates metrics without locker, higher performance.
func (d *DefaultStatItem) SnapshotAndClear() Snapshot {
	d.lock.Lock()
	defer d.lock.Unlock()
	old := atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&d.holder)), unsafe.Pointer(&RegistryHolder{registry: metrics.NewRegistry()}))
	d.lastSnapshot = &ReadonlyStatItem{
		group:          d.group,
		groupSuffix:    d.groupSuffix,
		service:        d.service,
		holder:         (*RegistryHolder)(old),
		isReport:       d.isReport,
		cache:          map[string]interface{}{},
		buildCacheLock: &sync.RWMutex{},
	}
	return d.lastSnapshot
}

// SnapshotAndClearV0 Using SnapshotAndClear instead.
// Deprecated.
// Because of Snapshot(DefaultStatItem) calculates metrics will call locker to do that,
// cause low performance
func (d *DefaultStatItem) SnapshotAndClearV0() Snapshot {
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

func (d *DefaultStatItem) Value(key string) int64 {
	v := d.getRegistry().Get(key)
	if g, ok := v.(metrics.Gauge); ok {
		return g.Value()
	}
	return 0
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

func (d *DefaultStatItem) IsGauge(key string) bool {
	_, ok := d.getRegistry().Get(key).(metrics.Gauge)
	return ok
}

type ReadonlyStatItem struct {
	group          string
	groupSuffix    string
	service        string
	holder         *RegistryHolder
	isReport       bool
	cache          map[string]interface{}
	buildCacheLock *sync.RWMutex
}

func (d *ReadonlyStatItem) getRegistry() metrics.Registry {
	return (*RegistryHolder)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&d.holder)))).registry
}

func (d *ReadonlyStatItem) getCache(key string) interface{} {
	d.buildCacheLock.RLock()
	if v, ok := d.cache[key]; ok {
		d.buildCacheLock.RUnlock()
		return v
	}
	d.buildCacheLock.RUnlock()

	d.buildCacheLock.Lock()
	defer d.buildCacheLock.Unlock()

	if v, ok := d.cache[key]; ok {
		return v
	}

	v := d.getRegistry().Get(key)
	if v == nil {
		return nil
	}
	var val interface{}
	switch h := v.(type) {
	case metrics.Counter:
		val = h.Snapshot()
	case metrics.Gauge:
		val = h.Snapshot()
	case metrics.Histogram:
		val = h.Snapshot()
	case metrics.Timer:
		val = h.Snapshot()
	case metrics.Meter:
		val = h.Snapshot()
	}
	d.cache[key] = val
	return val
}

func (d *ReadonlyStatItem) SetService(service string) {
	panic("action not supported")
}

func (d *ReadonlyStatItem) GetService() string {
	return d.service
}

// GetEscapedService return the escaped service used as graphite key
func (d *ReadonlyStatItem) GetEscapedService() string {
	return Escape(d.service)
}

func (d *ReadonlyStatItem) SetGroup(group string) {
	panic("action not supported")
}

func (d *ReadonlyStatItem) GetGroup() string {
	return d.group
}

func (d *ReadonlyStatItem) GetGroupSuffix() string {
	return d.groupSuffix
}

// GetEscapedGroup return the escaped group used as graphite key
func (d *ReadonlyStatItem) GetEscapedGroup() string {
	return Escape(d.group)
}

func (d *ReadonlyStatItem) AddCounter(key string, value int64) {
	panic("action not supported")
}

func (d *ReadonlyStatItem) AddHistograms(key string, duration int64) {
	panic("action not supported")
}

func (d *ReadonlyStatItem) AddGauge(key string, value int64) {
	panic("action not supported")
}

func (d *ReadonlyStatItem) Snapshot() Snapshot {
	return d.LastSnapshot()
}

func (d *ReadonlyStatItem) SnapshotAndClear() Snapshot {
	panic("action not supported")
}

func (d *ReadonlyStatItem) LastSnapshot() Snapshot {
	panic("action not supported")
}

func (d *ReadonlyStatItem) SetReport(b bool) {
	panic("action not supported")
}

func (d *ReadonlyStatItem) IsReport() bool {
	return d.isReport
}

func (d *ReadonlyStatItem) Remove(key string) {
	panic("action not supported")
}

func (d *ReadonlyStatItem) Clear() {
	panic("action not supported")
}

func (d *ReadonlyStatItem) Count(key string) (i int64) {
	v := d.getCache(key)
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

func (d *ReadonlyStatItem) Sum(key string) int64 {
	v := d.getCache(key)
	if h, ok := v.(metrics.Histogram); ok {
		return h.Sum()
	}
	return 0
}

func (d *ReadonlyStatItem) Max(key string) int64 {
	v := d.getCache(key)
	if h, ok := v.(metrics.Histogram); ok {
		return h.Max()
	}
	return 0
}

func (d *ReadonlyStatItem) Mean(key string) float64 {
	v := d.getCache(key)
	if h, ok := v.(metrics.Histogram); ok {
		return h.Mean()
	}
	return 0
}

func (d *ReadonlyStatItem) Min(key string) int64 {
	v := d.getCache(key)
	if h, ok := v.(metrics.Histogram); ok {
		return h.Min()
	}
	return 0
}

func (d *ReadonlyStatItem) P90(key string) float64 {
	return d.Percentile(key, 0.9)
}

func (d *ReadonlyStatItem) P95(key string) float64 {
	return d.Percentile(key, 0.95)
}

func (d *ReadonlyStatItem) P99(key string) float64 {
	return d.Percentile(key, 0.99)
}

func (d *ReadonlyStatItem) P999(key string) float64 {
	return d.Percentile(key, 0.999)
}

func (d *ReadonlyStatItem) Percentile(key string, f float64) float64 {
	v := d.getCache(key)
	if h, ok := v.(metrics.Histogram); ok {
		return h.Percentile(f)
	}
	return 0
}

// Percentiles : return value is nil while key not exist
func (d *ReadonlyStatItem) Percentiles(key string, f []float64) []float64 {
	v := d.getCache(key)
	if h, ok := v.(metrics.Histogram); ok {
		return h.Percentiles(f)
	}
	return nil
}

func (d *ReadonlyStatItem) Value(key string) int64 {
	v := d.getCache(key)
	if g, ok := v.(metrics.Gauge); ok {
		return g.Value()
	}
	return 0
}

func (d *ReadonlyStatItem) RangeKey(f func(k string)) {
	d.getRegistry().Each(func(s string, i interface{}) {
		f(s)
	})
}

func (d *ReadonlyStatItem) IsHistogram(key string) bool {
	_, ok := d.getCache(key).(metrics.Histogram)
	return ok
}

func (d *ReadonlyStatItem) IsCounter(key string) bool {
	_, ok := d.getCache(key).(metrics.Counter)
	return ok
}

func (d *ReadonlyStatItem) IsGauge(key string) bool {
	_, ok := d.getCache(key).(metrics.Gauge)
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
			vlog.Warningf("get metrics config fail:%s, use default config:{Period:%s, Processor:%d, Graphite:[]}", err.Error(), defaultSinkDuration, defaultEventProcessor)
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
		if ctx.AgentURL != nil {
			application := ctx.AgentURL.GetParam(motan.ApplicationKey, DefaultStatApplication)
			motan.PanicStatFunc = func() {
				keys := []string{DefaultStatRole, application, "panic"}
				AddCounterWithKeys(DefaultStatGroup, "", DefaultStatService,
					keys, ".total_count", 1)
			}
			startSampleStatus(application)
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
	eventPool   *sync.Pool
	writersLock sync.RWMutex
}

func (r *reporter) eventLoop() {
	for evt := range r.eventBus {
		r.processEvent(evt)
		// clean the event object before put it back
		evt.reset()
		r.eventPool.Put(evt)
	}
}

func (r *reporter) addWriter(key string, sw StatWriter) {
	if key != "" && sw != nil {
		r.writersLock.Lock()
		defer r.writersLock.Unlock()
		r.writers[key] = sw
		vlog.Infof("add metrics StatWriter %s", key)
	}
}

func (r *reporter) processEvent(evt *event) {
	defer motan.HandlePanic(nil)
	item := GetOrRegisterStatItem(evt.group, evt.groupSuffix, evt.service)
	key := evt.getMetricKey()
	switch evt.event {
	case eventCounter:
		item.AddCounter(key, evt.value)
	case eventHistograms:
		item.AddHistograms(key, evt.value)
	case eventGauge:
		item.AddGauge(key, evt.value)
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
						vlog.Errorf("write metrics error. name:%s, err:%v", name, err)
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
