package metrics

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/config"
	motan "github.com/weibocom/motan-go/core"
)

var (
	group   = "groupT"
	service = "serviceT"
)

func TestGetStatItem(t *testing.T) {
	ClearStatItems()
	// get
	si := GetStatItem(group, service)
	assert.Nil(t, si, "GetStatItem")
	assert.Equal(t, 0, StatItemSize(), "item size")

	// register
	si = GetOrRegisterStatItem(group, service)
	assert.NotNil(t, si, "GetOrRegisterStatItem")
	assert.Equal(t, group, si.GetGroup(), "StatItem group")
	assert.Equal(t, service, si.GetService(), "StatItem service")
	assert.Equal(t, 1, StatItemSize(), "item size")
	again := GetStatItem(group, service)
	assert.True(t, si == again, "get StatItem not the same one")
	si2 := GetOrRegisterStatItem(group+"2", service+"2")
	assert.NotNil(t, si2, "GetOrRegisterStatItem")
	assert.Equal(t, group+"2", si2.GetGroup(), "StatItem group")
	assert.Equal(t, service+"2", si2.GetService(), "StatItem service")
	assert.NotEqual(t, si, si2, "get StatItem is the same")
	assert.Equal(t, 2, StatItemSize(), "item size")

	// rm
	RMStatItem(group, service)
	si = GetStatItem(group, service)
	assert.Nil(t, si, "GetStatItem")
	si2 = GetStatItem(group+"2", service+"2")
	assert.NotNil(t, si2, "GetOrRegisterStatItem")

	// clear
	ClearStatItems()
	si = GetStatItem(group, service)
	assert.Nil(t, si, "clear not work")
	si2 = GetStatItem(group+"2", service+"2")
	assert.Nil(t, si2, "clear not work")

	// multi thread
	size := 50
	sia := make([]StatItem, size, size)
	var lock sync.Mutex
	for i := 0; i < size; i++ {
		j := i
		go func() {
			s := GetOrRegisterStatItem(group, service)
			lock.Lock()
			sia[j] = s
			lock.Unlock()
		}()
	}

	time.Sleep(10 * time.Millisecond)
	lock.Lock()
	for i := 0; i < size; i++ {
		assert.True(t, sia[i] == sia[0], "multi thread GetOrRegisterStatItem")
	}
	lock.Unlock()
}

func TestNewDefaultStatItem(t *testing.T) {
	si := NewStatItem(group, service)
	assert.NotNil(t, si, "NewStatItem")
	_, ok := si.(StatItem)
	assert.True(t, ok, "type not StatItem")
}

func TestDefaultStatItem(t *testing.T) {
	item := NewDefaultStatItem(group, service)
	assert.NotNil(t, item, "GetOrRegisterStatItem")
	assert.Equal(t, group, item.GetGroup(), "StatItem group")
	assert.Equal(t, service, item.GetService(), "StatItem service")
	item.AddCounter("c1", 123)
	item.AddHistograms("h1", 200)
	item.AddCounter("c2", 22)
	item.Remove("c2")
	snap := item.SnapshotAndClear()
	lastSnap := item.LastSnapshot()
	assert.True(t, snap == lastSnap, "last snapshot")
	c1 := snap.Count("c1")
	assert.Equal(t, int64(123), c1, "count")
	h1 := snap.P99("h1")
	assert.Equal(t, float64(200), h1, "histogram")
	count := 0
	snap.RangeKey(func(k string) {
		if k == "c2" {
			t.Error("should not has key:'c2'")
		}
		count++
	})
	assert.Equal(t, 2, count, "key size")
	item.SetReport(true)
	assert.True(t, item.IsReport(), "isReport")
	item.SetReport(false)
	assert.False(t, item.IsReport(), "isReport")
	snap.Clear()
	count = 0
	snap.RangeKey(func(k string) {
		count++
	})
	assert.Equal(t, 0, count, "key size")
}

func TestAddWriter(t *testing.T) {
	ClearStatItems()
	w := &mockWriter{}
	AddWriter("mock", w)
	nw := rp.writers["mock"]
	assert.NotNil(t, nw, "add writer fail")
	assert.True(t, w == nw, "add writer")

	// test sink
	rp.interval = 500 * time.Millisecond
	StartReporter(&motan.Context{Config: config.NewConfig()})
	AddCounter(group, service, "c1", 1)
	AddCounter(group, service, "c2", 2)
	AddHistograms(group, service, "h1", 100)
	AddHistograms(group, service, "h2", 200)
	AddGauge(group, service, "g1", 100)
	AddGauge(group, service, "g2", 200)

	time.Sleep(520 * time.Millisecond)
	assert.Equal(t, 1, len(w.GetSnapshot()), "writer snapshot size")
	assert.Equal(t, group, w.GetSnapshot()[0].GetGroup(), "snapshot group")
	assert.Equal(t, service, w.GetSnapshot()[0].GetService(), "snapshot group")
}

func TestStat(t *testing.T) {
	ClearStatItems()
	rp.processor = 5
	params := make(map[string]string)
	params[motan.ApplicationKey] = "testApplication"
	assert.Nil(t, motan.PanicStatFunc, "panic stat func")
	start = sync.Once{}
	StartReporter(&motan.Context{Config: config.NewConfig(), AgentURL: &motan.URL{Parameters: params}})
	// agent panic stat
	assert.NotNil(t, motan.PanicStatFunc, "panic stat func")

	// stat tests
	var d, total int64
	length := 1000
	for i := 0; i < length; i++ {
		AddCounter(group, service, "c1", 1)
		AddCounter(group, service, "c2", 2)
		AddHistograms(group, service, "h1", 100)
		AddHistograms(group, service, "h2", 200)
		d = rand.Int63n(1000)
		total += d
		AddHistograms(group, service, "h3", d)
		AddGauge(group, service, "g1", 100)
		AddGauge(group, service, "g2", 200)
	}
	time.Sleep(100 * time.Millisecond)
	item := GetStatItem(group, service)
	assert.NotNil(t, item, "item not exist")
	snap := item.SnapshotAndClear()
	// test counters
	assert.NotNil(t, snap, "snapshot not exist")
	assert.Equal(t, int64(length), snap.Count("c1"), "count")
	assert.Equal(t, int64(length*2), snap.Count("c2"), "count")

	// test histogram
	assert.NotNil(t, snap, "snapshot not exist")
	assert.Equal(t, int64(length), snap.Count("h1"), "count")
	assert.Equal(t, int64(length), snap.Count("h3"), "count")
	assert.Equal(t, int64(100), snap.Min("h1"), "min")
	assert.Equal(t, float64(100), snap.Mean("h1"), "mean")
	assert.Equal(t, int64(100), snap.Max("h1"), "max")
	assert.Equal(t, 100*int64(length), snap.Sum("h1"), "sum")
	assert.Equal(t, 200*int64(length), snap.Sum("h2"), "sum")
	assert.Equal(t, total, snap.Sum("h3"), "sum")
	assert.Equal(t, float64(total)/float64(length), snap.Mean("h3"), "mean")

	// pXX
	mean := snap.Mean("h3")
	p90 := snap.P90("h3")
	p95 := snap.P95("h3")
	p99 := snap.P99("h3")
	p999 := snap.P999("h3")
	assert.True(t, mean > 10, "pxx")
	assert.True(t, p90 > mean, "pxx")
	assert.True(t, p95 > p90, "pxx")
	assert.True(t, p99 > p95, "pxx")
	assert.True(t, p999 > p99, "pxx")
	assert.True(t, p999 < 1000, "pxx")

	// percentile
	p84 := snap.Percentile("h3", 0.84)
	p97 := snap.Percentile("h3", 0.97)
	assert.True(t, p84 > mean, "percentile")
	assert.True(t, p84 < p90, "percentile")
	assert.True(t, p90 < p97, "percentile")
	assert.True(t, p97 < p999, "percentile")

	// percentiles
	pa := []float64{0.9, 0.95, 0.99, 0.999, 0.5}
	pr := snap.Percentiles("h3", pa)
	assert.Equal(t, len(pa), len(pr), "return size")
	assert.Equal(t, p90, pr[0], "percentiles")
	assert.Equal(t, p95, pr[1], "percentiles")
	assert.Equal(t, p99, pr[2], "percentiles")
	assert.Equal(t, p999, pr[3], "percentiles")

	// test gauge
	assert.Equal(t, int64(100), snap.Value("g1"))
	assert.Equal(t, int64(200), snap.Value("g2"))
}

type mockWriter struct {
	lock      sync.RWMutex
	snapshots []Snapshot
}

func (m *mockWriter) Write(snapshots []Snapshot) error {
	m.lock.Lock()
	m.snapshots = snapshots
	m.lock.Unlock()
	return nil
}

func (m *mockWriter) GetSnapshot() []Snapshot {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.snapshots
}
