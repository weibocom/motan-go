package registry

import (
	"fmt"
	"testing"

	motan "github.com/weibocom/motan-go/core"
	"strconv"
)

func TestGetMeshRegistry(t *testing.T) {
	defaultExtFactory := &motan.DefaultExtentionFactory{}
	defaultExtFactory.Initialize()
	RegistDefaultRegistry(defaultExtFactory)
	regURL := &motan.URL{Protocol: "mesh", Host: "localhost", Port: 8001}
	regURL.PutParam(motan.ProxyRegistryKey, "direct://localhost:8001")
	regURL.PutParam(motan.MeshPortKey, strconv.Itoa(defaultMeshPort))
	registry := defaultExtFactory.GetRegistry(regURL)
	meshRegistry, _ := registry.(*MeshRegistry)
	fmt.Printf("mesh url: %v, registry's url: %v", regURL, meshRegistry.url)
	if regURL != meshRegistry.url {
		t.Fatalf("mesh registry url not correct. mesh url: %v, registry's url: %v", regURL, meshRegistry.url)
	}
}

func TestMeshRegistry_Discover(t *testing.T) {
	registryHost := "localhost"
	regURL := &motan.URL{Host: registryHost, Port: 8002}
	regURL.PutParam(motan.ProxyRegistryKey, "direct://localhost:9982")
	regURL.PutParam(motan.MeshPortKey, strconv.Itoa(defaultMeshPort))
	meshRegistry := &MeshRegistry{url: regURL}
	meshRegistry.Initialize()
	u1 := &motan.URL{Protocol: "motan2", Host: "localhost", Port: 8999}
	urls := meshRegistry.Discover(u1)
	if len(urls) != 1 {
		t.Fatalf("discover size should be 1. size: %d", len(urls))
	}
	if urls[0].Protocol != "motan2" || urls[0].Host != registryHost || urls[0].Port != defaultMeshPort {
		t.Fatalf("discover not correct. url: %+v", urls[0])
	}
}
