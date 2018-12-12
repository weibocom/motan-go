package core

import (
	"fmt"
	"testing"
	"time"
)

func TestNewPinger(t *testing.T) {
	pinger, err := NewPinger("127.0.0.1", 5, 5*time.Second, 56, false)
	if err != nil {
		t.Errorf(err.Error())
	}
	pinger.Ping()
	fmt.Printf("total count: %d\n", len(pinger.Rtts))
	for _, rtt := range pinger.Rtts {
		fmt.Println(rtt)
	}
}
