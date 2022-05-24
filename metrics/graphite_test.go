package metrics

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	role         = "role"
	application  = "application"
	methodPrefix = "method"
	localhost    = "localhost"
	keyPrefix    = fmt.Sprintf("%s:%s:%s", role, application, methodPrefix) // key include three parts >> role + ":" + application + ":" + method
)

func Test_graphite_Write(t *testing.T) {
	server := &udpServer{port: "3456"}
	go server.start()
	time.Sleep(100 * time.Millisecond)
	g := newGraphite("127.0.0.1", "test pool", 3456)
	item := NewStatItem(group, service)
	item.AddCounter(keyPrefix+"c1", 1)
	item.AddHistograms(keyPrefix+" h1", 100)
	err := g.Write([]Snapshot{item.SnapshotAndClear()})
	if err != nil {
		fmt.Printf("write graphit fail. err:%s\n", err)
	}
	time.Sleep(100 * time.Millisecond)
	server.stop()
	assert.Equal(t, 589, server.data.Len(), "send data size")

	// illegal udp connection address
	g = newGraphite("1.1.1.1", "test pool 2", 1234)
	item.AddCounter(keyPrefix+"c1", 1)
	item.AddHistograms(keyPrefix+" h1", 100)
	server.data.Reset()
	assert.Equal(t, 0, server.data.Len(), "illegal udp address data size")
	err = g.Write([]Snapshot{item.SnapshotAndClear()})
	assert.Equal(t, nil, err, "illegal udp address")
	assert.Equal(t, 0, server.data.Len(), "illegal udp address data size")
}

func TestGenGraphiteMessages(t *testing.T) {
	item1 := NewDefaultStatItem(group, service)

	// counter message
	item1.AddCounter(keyPrefix+"c1", 1)
	messages := GenGraphiteMessages(localhost, []Snapshot{item1.SnapshotAndClear()})
	assert.Equal(t, 1, len(messages), "message size")
	expect := fmt.Sprintf("%s.%s.%s.byhost.%s.%s.%s:%d|c\n", role, application, group, localhost, service, methodPrefix+"c1", 1)
	assert.Equal(t, expect, messages[0], "counter message")

	// histogram message
	item1.AddHistograms(keyPrefix+"h1", 100)
	messages = GenGraphiteMessages(localhost, []Snapshot{item1.SnapshotAndClear()})
	assert.Equal(t, 1, len(messages), "message size")
	for slaK := range sla {
		assert.True(t, strings.Contains(messages[0], fmt.Sprintf("%s.%s.%s.byhost.%s.%s.%s.%s:%.2f|kv\n",
			role, application, group, localhost, service, methodPrefix+"h1", slaK, float32(100))), "histogram message")
	}
	assert.True(t, strings.Contains(messages[0], fmt.Sprintf("%s.%s.%s.byhost.%s.%s.%s.%s:%.2f|ms\n",
		role, application, group, localhost, service, methodPrefix+"h1", "avg_time", float32(100))), "histogram message")

	// multi items
	item2 := NewDefaultStatItem(group+"2", service+"2")
	item3 := NewDefaultStatItem(group+"3", service+"3")
	item3.SetReport(false) // item3 not report

	length := 10
	for i := 0; i < length; i++ {
		item1.AddCounter(keyPrefix+"c1", 1)
		item1.AddHistograms(keyPrefix+"h1", 100)
		item2.AddCounter(keyPrefix+"c2", 1)
		item2.AddHistograms(keyPrefix+"h2", 100)
		item3.AddCounter(keyPrefix+"c3", 1)
		item3.AddHistograms(keyPrefix+"h3", 100)
	}

	snapshots := make([]Snapshot, 0, 10)
	snapshots = append(snapshots, item1.SnapshotAndClear())
	snapshots = append(snapshots, item2.SnapshotAndClear())
	snapshots = append(snapshots, item3.SnapshotAndClear())

	messages = GenGraphiteMessages(localhost, snapshots)
	assert.Equal(t, 1, len(messages), "message size")
	assert.True(t, strings.Contains(messages[0], "c1"), "message")
	assert.True(t, strings.Contains(messages[0], "h1"), "message")
	assert.True(t, strings.Contains(messages[0], "c2"), "message")
	assert.True(t, strings.Contains(messages[0], "h2"), "message")
	assert.False(t, strings.Contains(messages[0], "c3"), "message")
	assert.False(t, strings.Contains(messages[0], "h3"), "message")

	// large message
	var buf bytes.Buffer
	buf.WriteString(keyPrefix)
	for i := 0; i < messageMaxLen/10; i++ {
		buf.WriteString("1111111111")
	}
	item1.AddCounter(buf.String(), 1)
	item1.AddCounter(buf.String()+"c2", 1)
	item1.AddHistograms(keyPrefix+"h1", 100)
	messages = GenGraphiteMessages(localhost, []Snapshot{item1.SnapshotAndClear()})
	assert.Equal(t, 3, len(messages), "message size")
}

type udpServer struct {
	port    string
	stopped bool
	data    bytes.Buffer
	lock    sync.Mutex
}

func (u *udpServer) start() {
	ServerAddr, err := net.ResolveUDPAddr("udp", ":"+u.port)
	if err != nil {
		fmt.Printf("upd listen fail, Err:%s\n", err.Error())
		return
	}
	socket, err := net.ListenUDP("udp", ServerAddr)
	if err != nil {
		fmt.Printf("upd listen fail, Err:%s\n", err.Error())
		return
	}
	defer socket.Close()
	for {
		if u.isStop() {
			break
		}
		data := make([]byte, 4096)
		read, _, err := socket.ReadFromUDP(data)
		if err != nil {
			fmt.Printf("read fail, Err:%s\n", err.Error())
			continue
		}
		println(string(data[:read]))
		u.data.Write(data[:read])
	}
}

func (u *udpServer) stop() {
	u.lock.Lock()
	u.stopped = true
	u.lock.Unlock()
}

func (u *udpServer) isStop() bool {
	u.lock.Lock()
	defer u.lock.Unlock()
	return u.stopped
}
