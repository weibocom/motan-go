package lb

import (
	motan "github.com/weibocom/motan-go/core"
	"testing"
)

func TestRandomLB(t *testing.T) {
	endpoints := make([]motan.EndPoint, 0, 10)
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			endpoints = append(endpoints, lbTestMockEndpoint{index: i, isAvail: false})
		} else {
			endpoints = append(endpoints, lbTestMockEndpoint{index: i, isAvail: true})
		}
	}
	randomLb := &RandomLB{endpoints: endpoints}
	for i := 0; i < 30; i++ {
		ep := randomLb.Select(nil)
		if !ep.IsAvailable() || ep.(lbTestMockEndpoint).index%2 == 0 {
			t.Errorf("randomlb select error, isAvailable=false: %v\n", ep)
		}
	}

	for i := 0; i < 30; i++ {
		eps := randomLb.SelectArray(nil)
		if len(eps) != MaxSelectArraySize {
			t.Errorf("randomlb selectArray error, the number is not satisfied: %v\n", eps)
			break
		}
		for _, ep := range eps {
			if ep.(lbTestMockEndpoint).index%2 == 0 {
				t.Errorf("randomlb selectArray error, filter isAvailable: %v\n", ep)
			}
		}
	}

}
