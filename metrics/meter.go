package metrics

import (
	"git.intra.weibo.com/openapi_rd/motan-go/metrics"
	"github.com/rcrowley/go-metrics"
	"sync"
	"sync/atomic"
	"time"
)

// go-metrics中的Meter的Rate相关计算的都是指数移动变化率(http://blog.sina.com.cn/s/blog_5069fdde0100g4ua.html),
// 会隐藏一些数据指标抖动的细节,导致一些故障不易发现.因此重新实现一个Meter,计算数据指标即刻变化率.

const (
	meterDuration = 5e9
)

// MeterSnapshot is a read-only copy of another Meter.
type meterSnapshot struct {
	count                          int64
	rate1, rate5, rate15, rateMean float64
}

// Count returns the count of events at the time the snapshot was taken.
func (m *meterSnapshot) Count() int64 { return m.count }

// Mark panics.
func (*meterSnapshot) Mark(n int64) {
	panic("Mark called on a MeterSnapshot")
}

// Rate1 returns the one-minute average rate of events per second at the
// time the snapshot was taken.
func (m *meterSnapshot) Rate1() float64 { return m.rate1 }

// Rate5 returns the five-minute average rate of events per second at
// the time the snapshot was taken.
func (m *meterSnapshot) Rate5() float64 { return m.rate5 }

// Rate15 returns the fifteen-minute average rate of events per second
// at the time the snapshot was taken.
func (m *meterSnapshot) Rate15() float64 { return m.rate15 }

// RateMean returns the meter's mean rate of events per second at the time the
// snapshot was taken.
func (m *meterSnapshot) RateMean() float64 { return m.rateMean }

// Snapshot returns the snapshot.
func (m *meterSnapshot) Snapshot() metrics.Meter { return m }

type qpsMeter struct {
	lock      sync.RWMutex
	snapshot  *meterSnapshot
	a1        *average
	startTime time.Time
}

func getOrRegisterMeter(name string, r metrics.Registry) metrics.Meter {
	if nil == r {
		r = metrics.DefaultRegistry
	}
	return r.GetOrRegister(name, newQPSMeter).(metrics.Meter)
}

func newQPSMeter() *qpsMeter {
	m := &qpsMeter{
		snapshot:  &meterSnapshot{},
		startTime: time.Now(),
		a1:        &average{},
	}

	arbiter.Lock()
	defer arbiter.Unlock()
	arbiter.meters = append(arbiter.meters, m)
	if !arbiter.started {
		arbiter.started = true
		go arbiter.tick()
	}
	return m
}

// Count returns the number of events recorded.
func (m *qpsMeter) Count() int64 {
	m.lock.RLock()
	count := m.snapshot.count
	m.lock.RUnlock()
	return count
}

// Mark records the occurrence of n events.
func (m *qpsMeter) Mark(n int64) {
	m.lock.Lock()
	m.snapshot.count += n
	m.a1.update(n)
	m.lock.Unlock()
}

// Rate1 returns the one-minute average rate of events per second.
func (m *qpsMeter) Rate1() float64 {
	m.lock.RLock()
	rate1 := m.snapshot.rate1
	m.lock.RUnlock()
	return rate1
}

// Rate5 returns the five-minute average rate of events per second.
func (m *qpsMeter) Rate5() float64 {
	m.lock.RLock()
	rate5 := m.snapshot.rate5
	m.lock.RUnlock()
	return rate5
}

// Rate15 returns the fifteen-minute average rate of events per second.
func (m *qpsMeter) Rate15() float64 {
	m.lock.RLock()
	rate15 := m.snapshot.rate15
	m.lock.RUnlock()
	return rate15
}

// RateMean returns the meter's mean rate of events per second.
func (m *qpsMeter) RateMean() float64 {
	m.lock.RLock()
	rateMean := m.snapshot.rateMean
	m.lock.RUnlock()
	return rateMean
}

// Snapshot returns a read-only copy of the meter.
func (m *qpsMeter) Snapshot() metrics.Meter {
	m.lock.RLock()
	snapshot := *m.snapshot
	m.lock.RUnlock()
	return &snapshot
}

// should run with write lock held on m.lock
func (m *qpsMeter) updateSnapshot() {
	snapshot := m.snapshot
	snapshot.rate1 = m.a1.Rate()
	snapshot.rate5 = snapshot.rate1
	snapshot.rate15 = snapshot.rate1
	snapshot.rateMean = float64(snapshot.count) / time.Since(m.startTime).Seconds()
}

func (m *qpsMeter) tick() {
	m.lock.Lock()
	m.a1.tick()
	m.updateSnapshot()
	m.lock.Unlock()
}

type ticker interface {
	tick()
}

type meterArbiter struct {
	sync.RWMutex
	started bool
	meters  []ticker
	ticker  *time.Ticker
}

var arbiter = meterArbiter{ticker: time.NewTicker(meterDuration)}

// Ticks meters on the scheduled interval
func (ma *meterArbiter) tick() {
	for range ma.ticker.C {
		ma.tickMeters()
	}
}

func (ma *meterArbiter) tickMeters() {
	ma.RLock()
	for _, meter := range ma.meters {
		meter.tick()
	}
	ma.RUnlock()
}

type average struct {
	uncounted int64 // /!\ this should be the first member to ensure 64-bit alignment
	rate      float64
	mutex     sync.Mutex
}

func (a *average) Rate() float64 {
	a.mutex.Lock()
	rate := a.rate * float64(1e9)
	a.mutex.Unlock()
	return rate
}

// Tick ticks the clock to update the moving average.  It assumes it is called
// every five seconds.
func (a *average) tick() {

	count := atomic.LoadInt64(&a.uncounted)
	atomic.AddInt64(&a.uncounted, -count)
	instantRate := float64(count) / float64(meterDuration)
	a.mutex.Lock()
	a.rate = instantRate
	a.mutex.Unlock()
}

func (a *average) update(n int64) {
	atomic.AddInt64(&(a.uncounted), n)
}
