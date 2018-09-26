package lb

import (
	"testing"

	motan "github.com/weibocom/motan-go/core"
)

func TestRoundrobinLB(t *testing.T) {
	endpoints := make([]motan.EndPoint, 0, 10)
	for i := 0; i < 10; i++ {
		if i > 2 && i < 6 {
			endpoints = append(endpoints, lbTestMockEndpoint{index: i, isAvail: false})
		} else {
			endpoints = append(endpoints, lbTestMockEndpoint{index: i, isAvail: true})
		}
	}
	roundrobinLb := &RoundrobinLB{endpoints: endpoints}
	for i := 0; i < 30; i++ {
		ep := roundrobinLb.Select(nil)
		if !ep.IsAvailable() || (ep.(lbTestMockEndpoint).index > 2 && ep.(lbTestMockEndpoint).index < 6) {
			t.Errorf("roundrobinLb select error, isAvailable=false: %v\n", ep)
		}
	}

	for i := 0; i < 30; i++ {
		eps := roundrobinLb.SelectArray(nil)
		if len(eps) != MaxSelectArraySize {
			t.Errorf("roundrobinLb selectArray error, the number is not satisfied: %v\n", eps)
			break
		}
		for _, ep := range eps {
			if ep.(lbTestMockEndpoint).index > 2 && ep.(lbTestMockEndpoint).index < 6 {
				t.Errorf("roundrobinLb selectArray error, isAvailable=false: %v\n", ep)
			}
		}
	}

}
