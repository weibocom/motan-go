package registry

import (
	"fmt"
	"testing"

	motan "github.com/weibocom/motan-go/core"
)

func TestGetDirectRegistey(t *testing.T) {
	defaultExtFactory := &motan.DefaultExtentionFactory{}
	defaultExtFactory.Initialize()
	RegistDefaultRegistry(defaultExtFactory)
	dirUrl := &motan.Url{Protocol: "direct", Host: "localhost", Port: 8001}
	registry := defaultExtFactory.GetRegistry(dirUrl)
	dirRegistry, _ := registry.(*DirectRegistry)
	fmt.Printf("dirurl: %+v, registry's url: %+v", dirUrl, dirRegistry.url)
	if dirUrl != dirRegistry.url {
		t.Fatalf("direct registry url not correct. dirurl: %+v, registry's url: %+v", dirUrl, dirRegistry.url)
	}
}

func TestDirectDiscover(t *testing.T) {
	regUrl := &motan.Url{Host: "127.0.0.1", Port: 8000}
	registry := &DirectRegistry{url: regUrl}
	u1 := &motan.Url{Protocol: "motan", Host: "10.210.230.10", Port: 8999}
	urls := registry.Discover(u1)
	if len(urls) != 1 {
		t.Fatalf("discover size should be 1. size: %d", len(urls))
	}
	if urls[0].Protocol != "motan" || urls[0].Host != "127.0.0.1" || urls[0].Port != 8000 {
		t.Fatalf("discover not correct. url: %+v", urls[0])
	}

	u2 := &motan.Url{Protocol: "grpc", Host: "10.210.230.20", Port: 8888}
	urls = registry.Discover(u2)
	if len(urls) != 1 {
		t.Fatalf("discover size should be 1. size: %d", len(urls))
	}
	fmt.Printf("discover url: %+v, org url: %+v", urls[0], u2)
	if urls[0].Protocol != "grpc" || urls[0].Host != "127.0.0.1" || urls[0].Port != 8000 {
		t.Fatalf("discover not correct. url: %+v", urls[0])
	}

}

func TestMultiAddress(t *testing.T) {
	params := make(map[string]string)
	params["address"] = "127.0.0.1:8002;10.210.235.1:8003;127.0.0.1:8005"
	regUrl := &motan.Url{Parameters: params}
	registry := &DirectRegistry{url: regUrl}
	urlParams := make(map[string]string)
	urlParams["codec"] = "xxx"
	urlParams["group"] = "test"
	u1 := &motan.Url{Protocol: "motan", Host: "10.210.230.10", Port: 8999, Parameters: urlParams}
	urls := registry.Discover(u1)
	for i, u := range urls {
		fmt.Printf("u: %v \n", u)
		if u.Host != registry.urls[i].Host || u.Port != registry.urls[i].Port {
			t.Fatalf("discover not correct. url: %+v", u)
		}
		for k, v := range u.Parameters {
			if v != u1.Parameters[k] {
				t.Fatalf("discover not correct. parameters not same . k: %s, v: %s, realv: %s", k, u1.Parameters[k], v)
			}
		}
	}
}
