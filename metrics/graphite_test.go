package metrics

import (
	"fmt"
	"testing"

	"bytes"

	"strings"

	"net"

	"time"

	"github.com/stretchr/testify/assert"
)

var (
	role         = "role"
	application  = "application"
	methodPerfix = "method"
	localhost    = "localhost"
	keyPerfix    = fmt.Sprintf("%s:%s:%s", role, application, methodPerfix) // key include three parts >> role + ":" + application + ":" + method

)

func Test_graphite_Write(t *testing.T) {
	server := &udpserver{port: 3456}
	go server.start()
	g := newGraphite("127.0.0.1", "testpool", 3456)
	item := NewStatItem(group, service)
	item.AddCounter(keyPerfix+"c1", 1)
	item.AddHistograms(keyPerfix+" h1", 100)
	g.Write([]Snapshot{item.SnapshotAndClear()})
	time.Sleep(100 * time.Millisecond)
	server.stop()
	assert.Equal(t, 589, server.data.Len(), "send data size")
}

func TestGenGraphiteMessages(t *testing.T) {
	item1 := NewDefaultStatItem(group, service)

	// counter message
	item1.AddCounter(keyPerfix+"c1", 1)
	messages := GenGraphiteMessages(localhost, []Snapshot{item1.SnapshotAndClear()})
	assert.Equal(t, 1, len(messages), "message size")
	expect := fmt.Sprintf("%s.%s.%s.byhost.%s.%s.%s:%d|c\n", role, application, group, localhost, service, methodPerfix+"c1", 1)
	assert.Equal(t, expect, messages[0], "counter message")

	// histogram message
	item1.AddHistograms(keyPerfix+"h1", 100)
	messages = GenGraphiteMessages(localhost, []Snapshot{item1.SnapshotAndClear()})
	assert.Equal(t, 1, len(messages), "message size")
	for slak := range sla {
		assert.True(t, strings.Contains(messages[0], fmt.Sprintf("%s.%s.%s.byhost.%s.%s.%s.%s:%.2f|kv\n",
			role, application, group, localhost, service, methodPerfix+"h1", slak, float32(100))), "histogram message")
	}
	assert.True(t, strings.Contains(messages[0], fmt.Sprintf("%s.%s.%s.byhost.%s.%s.%s.%s:%.2f|ms\n",
		role, application, group, localhost, service, methodPerfix+"h1", "avg_time", float32(100))), "histogram message")

	// multi items
	item2 := NewDefaultStatItem(group+"2", service+"2")
	item3 := NewDefaultStatItem(group+"3", service+"3")
	item3.SetReport(false) // item3 not report

	length := 10
	for i := 0; i < length; i++ {
		item1.AddCounter(keyPerfix+"c1", 1)
		item1.AddHistograms(keyPerfix+"h1", 100)
		item2.AddCounter(keyPerfix+"c2", 1)
		item2.AddHistograms(keyPerfix+"h2", 100)
		item3.AddCounter(keyPerfix+"c3", 1)
		item3.AddHistograms(keyPerfix+"h3", 100)
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
	buf.WriteString(keyPerfix)
	for i := 0; i < messageMaxLen/10; i++ {
		buf.WriteString("mmmmmmmmmm")
	}
	item1.AddCounter(buf.String(), 1)
	item1.AddCounter(buf.String()+"c2", 1)
	item1.AddHistograms(keyPerfix+"h1", 100)
	messages = GenGraphiteMessages(localhost, []Snapshot{item1.SnapshotAndClear()})
	assert.Equal(t, 3, len(messages), "message size")
}

type udpserver struct {
	port   int
	stoped bool
	data   bytes.Buffer
}

func (u *udpserver) start() {
	socket, err := net.ListenUDP("udp4", &net.UDPAddr{
		IP:   net.IPv4(0, 0, 0, 0),
		Port: u.port,
	})
	if err != nil {
		fmt.Printf("upd listen fail, Err:%s\n", err.Error())
		return
	}
	defer socket.Close()

	for {
		if u.stoped {
			break
		}
		data := make([]byte, 4096)
		read, _, err := socket.ReadFromUDP(data)
		if err != nil {
			fmt.Printf("read fail, Err:%s\n", err.Error())
			continue
		}
		u.data.Write(data[:read])
	}
}

func (u *udpserver) stop() {
	u.stoped = true
}
