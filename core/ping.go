package core

import (
	"math/rand"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/weibocom/motan-go/log"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	timeLength       = 8
	protocolICMP     = 1
	protocolIPv6ICMP = 58
)

func NewPinger(addr string, count int, timeout time.Duration, size int, privileged bool) (*Pinger, error) {
	ipAddr, err := net.ResolveIPAddr("ip", addr)
	if err != nil {
		return nil, err
	}
	pinger := &Pinger{
		ipAddr: ipAddr,
		addr:   addr,

		Interval: 1 * time.Millisecond,
		Count:    count,
		id:       rand.Intn(0xffff),
		Size:     timeLength + size,
		Timeout:  timeout,

		done: make(chan bool),
	}

	if isIPv4(ipAddr.IP) {
		pinger.icmpType = ipv4.ICMPTypeEcho
		pinger.proto = protocolICMP
		pinger.ipv4 = true
	} else if isIPv6(ipAddr.IP) {
		pinger.icmpType = ipv6.ICMPTypeEchoRequest
		pinger.proto = protocolIPv6ICMP
		pinger.ipv4 = false
	}

	if privileged {
		pinger.dst = ipAddr
		pinger.netProto = "ip4:icmp"
		if !pinger.ipv4 {
			pinger.netProto = "ip6:ipv6-icmp"
		}
	} else {
		pinger.dst = &net.UDPAddr{IP: ipAddr.IP, Zone: ipAddr.Zone}
		pinger.netProto = "udp4"
		if !pinger.ipv4 {
			pinger.netProto = "udp6"
		}
	}

	return pinger, nil
}

type Pinger struct {
	Interval    time.Duration
	Timeout     time.Duration
	Count       int
	PacketsSent int
	PacketsRecv int
	Rtts        []time.Duration
	Size        int

	done     chan bool
	ipAddr   *net.IPAddr
	addr     string
	netProto string
	dst      net.Addr
	icmpType icmp.Type
	proto    int
	ipv4     bool
	id       int

	sequence int
}

type packet struct {
	bytes  []byte
	nBytes int
}

func (p *Pinger) IPAddr() *net.IPAddr {
	return p.ipAddr
}

func (p *Pinger) Addr() string {
	return p.addr
}

func (p *Pinger) Ping() error {
	conn, err := p.listen()
	if err != nil {
		return err
	}

	defer conn.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()

	recv := make(chan *packet, 5)
	go p.recvICMP(conn, recv, &wg)

	err = p.sendICMP(conn)
	if err != nil {
		vlog.Errorf("Send ping packet error: %s", err.Error())
	}

	timeout := time.NewTicker(p.Timeout)
	interval := time.NewTicker(p.Interval)
	defer timeout.Stop()
	defer interval.Stop()

	for {
		select {
		case <-p.done:
			return nil
		case <-timeout.C:
			close(p.done)
			return errors.New("Ping timed out")
		case <-interval.C:
			if p.Count > 0 && p.PacketsSent == p.Count {
				break
			}
			err = p.sendICMP(conn)
			if err != nil {
				vlog.Errorln(err.Error())
			}
		case r := <-recv:
			err := p.processPacket(r)
			if err != nil {
				close(p.done)
				return err
			}
			if p.Count > 0 && p.PacketsRecv >= p.Count {
				close(p.done)
				return nil
			}
		}
	}
}

func (p *Pinger) sendICMP(conn *icmp.PacketConn) error {
	buffer := NewBytesBuffer(p.Size)
	buffer.WriteUint64(uint64(time.Now().UnixNano()))
	for i := 0; i < p.Size-timeLength; i++ {
		buffer.WriteByte(1)
	}

	pingMessage := &icmp.Message{
		Type: p.icmpType, Code: 0,
		Body: &icmp.Echo{
			ID:   p.id,
			Seq:  p.sequence,
			Data: buffer.Bytes(),
		},
	}
	bytes, err := pingMessage.Marshal(nil)
	if err != nil {
		return err
	}

	for {
		if _, err := conn.WriteTo(bytes, p.dst); err != nil {
			if netErr, ok := err.(*net.OpError); ok && netErr.Err == syscall.ENOBUFS {
				continue
			} else {
				vlog.Infoln("Send icmp packet error: " + err.Error())
			}
		}
		p.PacketsSent++
		p.sequence++
		break
	}
	return nil
}

func (p *Pinger) recvICMP(conn *icmp.PacketConn, recv chan<- *packet, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-p.done:
			return
		default:
			bytes := make([]byte, 512)
			conn.SetReadDeadline(time.Now().Add(time.Second))
			n, _, err := conn.ReadFrom(bytes)
			if err != nil {
				if netErr, ok := err.(*net.OpError); ok {
					if netErr.Timeout() {
						// Read timeout
						continue
					} else {
						close(p.done)
						vlog.Errorf("Read icmp response error: %s", err.Error())
						return
					}
				}
			}

			recv <- &packet{bytes: bytes, nBytes: n}
		}
	}
}

func (p *Pinger) processPacket(recv *packet) error {
	var m *icmp.Message
	var err error
	if m, err = icmp.ParseMessage(p.proto, recv.bytes[:recv.nBytes]); err != nil {
		return errors.New("Error parsing icmp message")
	}

	if m.Type != ipv4.ICMPTypeEchoReply && m.Type != ipv6.ICMPTypeEchoReply {
		// Not an echo reply, ignore it
		return nil
	}

	// Check if reply from same ID
	body := m.Body.(*icmp.Echo)
	if body.ID != p.id {
		return nil
	}

	switch pkt := m.Body.(type) {
	case *icmp.Echo:
		p.PacketsRecv++
		buffer := CreateBytesBuffer(pkt.Data)
		nano, err := buffer.ReadUint64()
		if err != nil {
			vlog.Infoln("Response packet body error: " + err.Error())
		} else {
			rtt := time.Since(time.Unix(int64(nano)/int64(time.Second), int64(nano)%int64(time.Second)))
			p.Rtts = append(p.Rtts, rtt)
		}
	default:
		return errors.Errorf("Invalid ICMP echo reply. Body type: %T, %s", pkt, pkt)
	}

	return nil
}

func (p *Pinger) listen() (*icmp.PacketConn, error) {
	conn, err := icmp.ListenPacket(p.netProto, "")
	if err != nil {
		vlog.Errorf("Error listening for ICMP packets: %s", err.Error())
		close(p.done)
		return nil, err
	}
	return conn, nil
}

func isIPv4(ip net.IP) bool {
	return len(ip.To4()) == net.IPv4len
}

func isIPv6(ip net.IP) bool {
	return len(ip) == net.IPv6len
}
