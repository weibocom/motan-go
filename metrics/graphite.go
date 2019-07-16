package metrics

import (
	"bytes"
	"fmt"
	"net"
	"strconv"
	"strings"

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
	localAddr    net.Addr
)

type graphite struct {
	Host    string
	Port    int
	Name    string
	localIP string
}

func newGraphite(ip, pool string, port int) *graphite {
	return &graphite{
		Host: ip,
		Port: port,
		Name: pool,
	}
}

func (g *graphite) Write(snapshots []Snapshot) error {
	dial := net.Dialer{LocalAddr: localAddr}
	conn, err := dial.Dial("udp", net.JoinHostPort(g.Host, strconv.Itoa(g.Port)))
	if err != nil {
		vlog.Warningf("open graphite conn fail. err:%s", err.Error())
		return err
	}
	defer conn.Close()
	if localAddr == nil {
		localAddr = conn.LocalAddr()
	}
	if g.localIP == "" {
		g.localIP = strings.Replace(strings.Split(conn.LocalAddr().String(), ":")[0], ".", "_", -1)
	}

	messages := GenGraphiteMessages(g.localIP, snapshots)
	for _, message := range messages {
		if message != "" {
			vlog.MetricsLog("\n" + message)
			_, err = conn.Write([]byte(message))
			if err != nil {
				return err
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
				if snap.IsHistogram(k) { //histogram
					for slaK, slaV := range sla {
						segment += fmt.Sprintf("%s.%s.%s.byhost.%s.%s.%s.%s:%.2f|kv\n",
							pni[0], pni[1], snap.GetGroup(), localIP, snap.GetService(), pni[2], slaK, snap.Percentile(k, slaV))
					}
					segment += fmt.Sprintf("%s.%s.%s.byhost.%s.%s.%s.%s:%.2f|ms\n",
						pni[0], pni[1], snap.GetGroup(), localIP, snap.GetService(), pni[2], "avg_time", snap.Mean(k))
				} else { //counter
					segment = fmt.Sprintf("%s.%s.%s.byhost.%s.%s.%s:%d|c\n",
						pni[0], pni[1], snap.GetGroup(), localIP, snap.GetService(), pni[2], snap.Count(k))
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
