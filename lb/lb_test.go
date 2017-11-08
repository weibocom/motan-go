package lb

import (
	"strconv"
	"testing"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
)

func TestWeightedLbWraper(t *testing.T) {
	url := &motan.URL{Parameters: make(map[string]string)}
	url.Parameters[motan.Lbkey] = Roundrobin
	defaultExtFactory := &motan.DefaultExtentionFactory{}
	defaultExtFactory.Initialize()
	RegistDefaultLb(defaultExtFactory)
	lb := defaultExtFactory.GetLB(url)
	wlbw, ok := lb.(*WeightedLbWraper)
	if !ok {
		t.Errorf("lb type not WeightedLbWraper, lb: %v\n", lb)
	}

	//test weight ratio
	weight := "group0:20,group1:30,group2:50"
	wlbw.SetWeight(weight)
	endpoints := make([]motan.EndPoint, 0, 20)
	for i := 0; i < 3; i++ {
		group := "group" + strconv.Itoa(i)
		for j := 0; j < 5; j++ {
			endpoints = append(endpoints, &endpoint.MockEndpoint{URL: &motan.URL{Port: 1000 + 1000*i + j, Group: group}})
		}
	}
	wlbw.OnRefresh(endpoints)
	refers, ok := wlbw.refers.(*weightedRefers)
	if !ok {
		t.Errorf("refers type not weightedRefers, lb: %v\n", lb)
	}
	if refers.groupWeight["group0"] != 2 || refers.groupWeight["group1"] != 3 || refers.groupWeight["group2"] != 5 {
		t.Errorf("weight not correct. %+v\n", refers.groupWeight)
	}
	if refers.ringSize != 10 {
		t.Errorf("ringsize not correct. %d\n", refers.ringSize)
	}
	for k, v := range refers.groupLb {
		lb := v.(*RoundrobinLB)
		if len(lb.endpoints) != 5 {
			t.Errorf("lb endpoint size not correct. group:%s, lb: %+v\n", k, v)
		}
	}
	var g0, g1, g2 int
	for _, v := range refers.weightRing {
		switch v {
		case "group0":
			g0 = g0 + 1
		case "group1":
			g1 = g1 + 1
		case "group2":
			g2 = g2 + 1
		}
	}
	if g0 != 2 || g1 != 3 || g2 != 5 {
		t.Errorf("weight ring not correct. g0:%d, g1: %d, g2:%d\n", g0, g1, g2)
	}

	request := &motan.MotanRequest{}
	// test lb
	for i := 1; i < 21; i++ { // the first index of weightring used for select is 1
		ep := wlbw.Select(request)
		rg := refers.weightRing[i%len(refers.weightRing)]
		if ep.GetURL().Group != rg {
			t.Errorf("ep group not same with weight ring. group:%s, ring: %s\n", ep.GetURL().Group, rg)
		}
	}

	//test no weight
	wlbw.SetWeight("")
	wlbw.OnRefresh(endpoints)
	_, ok = wlbw.refers.(*singleGroupRefers)
	if !ok {
		t.Errorf("refers type not singleGroupRefers, lb: %v\n", lb)
	}
	//repeat
	wlbw.OnRefresh(endpoints)

}

func TestLBSelect(t *testing.T) {
	endpoints := make([]motan.EndPoint, 0, 5)
	for i := 0; i < 5; i++ {
		if i == 2 {
			endpoints = append(endpoints, lbTestMockEndpoint{index: i, isAvail: false})
		} else {
			endpoints = append(endpoints, lbTestMockEndpoint{index: i, isAvail: true})
		}
	}

	eps := selectArrayFromIndex(endpoints, 1)
	if len(eps) != MaxSelectArraySize {
		t.Errorf("selectArrayFromIndex filter isAvailable error: %v\n", eps)
	}

	for i := 0; i < 20; i++ {
		index, ep := selectOneAtRandom(endpoints)
		if !ep.IsAvailable() || index == 2 {
			t.Errorf("selectOneAtRandom filter isAvailable error: %v\n", eps)
		}
	}

}

type lbTestMockEndpoint struct {
	*endpoint.MockEndpoint
	isAvail bool
	index   int
}

func (e lbTestMockEndpoint) IsAvailable() bool {
	return e.isAvail
}

func (e lbTestMockEndpoint) setAvailable(isAvail bool) {
	e.isAvail = isAvail
}
