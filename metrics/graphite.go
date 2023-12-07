package metrics

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/weibocom/motan-go/log"
)

const (
	messageMaxLen = 5 * 1024
)

var (
	sla = map[string]float64{
		"p50":   0.5,
		"p75":   0.75,
		"p95":   0.95,
		"p99":   0.99,
		"p999":  0.999,
		"p9999": 0.9999}
	minKeyLength = 3
)

type graphite struct {
	Host    string
	Port    int
	Name    string
	localIP string
	lock    *sync.Mutex //using pointer avoid shadow copy, cause lock issue
	conn    net.Conn
}

func getUDPConn(ip string, port int) net.Conn {
	conn, err := net.Dial("udp", net.JoinHostPort(ip, strconv.Itoa(port)))
	if err != nil {
		vlog.Warningf("Open graphite conn failed. err:%s", err.Error())
		return nil
	}
	return conn
}

func newGraphite(ip, pool string, port int) *graphite {
	return &graphite{
		Host: ip,
		Port: port,
		Name: pool,
		lock: &sync.Mutex{},
		conn: getUDPConn(ip, port),
	}
}

func (g *graphite) Write(snapshots []Snapshot) error {
	g.lock.Lock()
	defer g.lock.Unlock()

	if g.conn = getUDPConn(g.Host, g.Port); g.conn == nil {
		return errors.New("open graphite conn failed")
	}

	defer func() {
		if g.conn != nil {
			g.conn.Close()
		}
	}()

	if g.localIP == "" {
		g.localIP = strings.Replace(strings.Split(g.conn.LocalAddr().String(), ":")[0], ".", "_", -1)
	}

	messages := GenGraphiteMessages(g.localIP, snapshots)
	for _, message := range messages {
		if message != "" {
			vlog.MetricsLog("\n" + message)
			if _, err := g.conn.Write([]byte(message)); err != nil {
				vlog.Warningln("Write graphite error, reconnect. err:", err.Error())
				g.conn.Close()
				if g.conn = getUDPConn(g.Host, g.Port); g.conn == nil {
					return errors.New("open graphite conn failed")
				}
			}
		}
	}
	return nil
}

func GenGraphiteMessages(localIP string, snapshots []Snapshot) []string {
	messages := make([]string, 0, 16)
	var buf bytes.Buffer
	buf.Grow(messageMaxLen)

	for _, snap := range snapshots {
		if snap.IsReport() {
			snap.RangeKey(func(k string) {
				var segment string
				pni := strings.SplitN(k, KeyDelimiter, minKeyLength)
				if len(pni) < minKeyLength {
					return
				}
				escapedService := snap.GetEscapedService()
				escapedGroup := snap.GetEscapedGroup()
				if snap.IsHistogram(k) { //histogram
					for slaK, slaV := range sla {
						segment += fmt.Sprintf("%s.%s.%s.byhost.%s.%s.%s.%s:%.2f|kv\n",
							pni[0], pni[1], escapedGroup, localIP, escapedService, pni[2], slaK, snap.Percentile(k, slaV))
					}
					segment += fmt.Sprintf("%s.%s.%s.byhost.%s.%s.%s.%s:%.2f|ms\n",
						pni[0], pni[1], escapedGroup, localIP, escapedService, pni[2], "avg_time", snap.Mean(k))
				} else if snap.IsCounter(k) { //counter
					segment = fmt.Sprintf("%s.%s.%s.byhost.%s.%s.%s:%d|c\n",
						pni[0], pni[1], escapedGroup, localIP, escapedService, pni[2], snap.Count(k))
				} else { // gauge
					segment = fmt.Sprintf("%s.%s.%s.byhost.%s.%s.%s:%d|kv\n",
						pni[0], pni[1], escapedGroup, localIP, escapedService, pni[2], snap.Value(k))
				}
				if buf.Len() > 0 && buf.Len()+len(segment) > messageMaxLen {
					messages = append(messages, buf.String())
					buf = bytes.Buffer{}
					buf.Grow(messageMaxLen)
				}
				buf.WriteString(segment)
			})
		}
	}

	messages = append(messages, buf.String())
	return messages
}
