package core

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExtFactory(t *testing.T) {
	ext := DefaultExtensionFactory{}
	ext.Initialize()
	ext.RegistExtHa("test", newHa)
	ext.RegistExtHa("test2", newHa)
	if len(ext.haFactories) != 2 {
		t.Errorf("ext regitry fail.%+v", ext.haFactories)
	}

	ext.RegistExtLb("test", newlb)
	ext.RegistExtFilter("test", newFilter)
	ext.RegistExtRegistry("test", newRegistry)
	ext.RegistExtEndpoint("test", newEp)
	ext.RegistExtProvider("test", newProvider)
	ext.RegistExtServer("test", newServer)
	ext.RegistryExtMessageHandler("test", newMsHandler)
	ext.RegistryExtSerialization("test", 0, newSerial)
}

func TestMotanRequest_Clone(t *testing.T) {
	request := &MotanRequest{}
	for i := 0; i < 20; i++ {
		request.SetAttachment("tttttttttttttttt"+strconv.Itoa(i), "tttttttttttttttt"+strconv.Itoa(i))
	}
	parnell := 2
	iteration := 1000
	requestBuffer := make(chan Request, parnell*iteration)
	group := &sync.WaitGroup{}
	group.Add(parnell)
	for i := 0; i < parnell; i++ {
		go func(i int) {
			for j := 0; j < iteration; j++ {
				cloneRequest := request.Clone().(Request)
				testValue := i*iteration + j
				cloneRequest.SetAttachment("test", strconv.Itoa(testValue))
				requestBuffer <- cloneRequest
			}
			group.Done()
		}(i)
	}
	group.Wait()
	count := 0
	resultMap := make(map[string]string, parnell*iteration)
	for {
		select {
		case r := <-requestBuffer:
			resultMap[r.GetAttachment("test")] = r.GetAttachment("test")
			count++
		}
		if count == parnell*iteration {
			break
		}
	}
	if len(resultMap) != parnell*iteration {
		t.Fatalf("maybe some duplicate requests, except: %d, actual: %d", parnell*iteration, len(resultMap))
	}
}

func TestServiceInGroup(t *testing.T) {
	registry := &mockRegistry{}
	registry.url = &URL{Protocol: "mock", Host: "testHost", Port: 0}
	assert.True(t, ServiceInGroup(registry, "testGroup", "testService"))
	assert.False(t, ServiceInGroup(registry, "testGroup", "testNoneExistService"))
}

func TestServiceInGroupParallel(t *testing.T) {
	registry := &mockRegistry{}
	registry.url = &URL{Protocol: "mock", Host: "testHost", Port: 0}
	registryGroupServiceInfoMaxCacheTime = 10 * time.Millisecond
	for i := 0; i < 10000; i++ {
		assert.True(t, ServiceInGroup(registry, "testGroup", "testService"))
		assert.False(t, ServiceInGroup(registry, "testGroup", "testNoneExistService"))
	}
}

func BenchmarkServiceInGroup(b *testing.B) {
	registry := &mockRegistry{}
	registry.url = &URL{Protocol: "mock", Host: "testHost", Port: 0}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ServiceInGroup(registry, "testGroup", "testService")
		}
	})

}

func newHa(url *URL) HaStrategy {
	return nil
}

func newlb(url *URL) LoadBalance {
	return nil
}

func newFilter() Filter {
	return nil
}

func newRegistry(url *URL) Registry {
	return nil
}

func newEp(url *URL) EndPoint {
	return nil
}

func newProvider(url *URL) Provider {
	return nil
}

func newServer(url *URL) Server {
	return nil
}

func newMsHandler() MessageHandler {
	return nil
}

func newSerial() Serialization {
	return nil
}

type mockRegistry struct {
	url *URL
}

func (r *mockRegistry) GetName() string {
	return "mockRegistry"
}

func (r *mockRegistry) GetURL() *URL {
	return r.url
}

func (r *mockRegistry) SetURL(url *URL) {
	r.url = url
}

func (r *mockRegistry) Subscribe(url *URL, listener NotifyListener) {
	panic("implement me")
}

func (r *mockRegistry) Unsubscribe(url *URL, listener NotifyListener) {
	panic("implement me")
}

func (r *mockRegistry) Discover(url *URL) []*URL {
	panic("implement me")
}

func (r *mockRegistry) Register(serverURL *URL) {
	panic("implement me")
}

func (r *mockRegistry) UnRegister(serverURL *URL) {
	panic("implement me")
}

func (r *mockRegistry) Available(serverURL *URL) {
	panic("implement me")
}

func (r *mockRegistry) Unavailable(serverURL *URL) {
	panic("implement me")
}

func (r *mockRegistry) GetRegisteredServices() []*URL {
	panic("implement me")
}

func (r *mockRegistry) StartSnapshot(conf *SnapshotConf) {
	panic("implement me")
}

func (r *mockRegistry) DiscoverAllServices(group string) ([]string, error) {
	return []string{"testService"}, nil
}

func (r *mockRegistry) DiscoverAllGroups() ([]string, error) {
	return []string{"testService"}, nil
}
