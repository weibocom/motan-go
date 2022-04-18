package core

import (
	"strconv"
	"sync"
	"testing"

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
	ext.RegisterExtPipe(newPipe)
}

func TestRegistLocalProvider(t *testing.T) {
	service := "testService"
	tp := &TestProvider{}
	RegistLocalProvider(service, tp)
	assert.Equal(t, tp, GetLocalProvider(service))
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

func TestGetAllGroups(t *testing.T) {
	registry := newMockRegistry()
	discoverErrorRegistry := newDiscoverErrorRegistry()
	assert.Equal(t, GetAllGroups(registry)[0], "testGroup")
	assert.True(t, len(GetAllGroups(discoverErrorRegistry)) == 0)
}

func TestGetAllGroupsParallel(t *testing.T) {
	registry := newMockRegistry()
	goNum := 5
	wg := &sync.WaitGroup{}
	wg.Add(goNum)
	for i := 0; i < goNum; i++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				assert.Equal(t, GetAllGroups(registry)[0], "testGroup")
			}
		}()
	}
	wg.Wait()
}

func TestServiceInGroup(t *testing.T) {
	registry := newMockRegistry()
	assert.True(t, ServiceInGroup(registry, "testGroup", "testService"))
	assert.False(t, ServiceInGroup(registry, "testGroup", "testNoneExistService"))
	discoverErrorRegistry := newDiscoverErrorRegistry()
	assert.False(t, ServiceInGroup(discoverErrorRegistry, "testGroup", "testService"))
}

func TestServiceInGroupParallel(t *testing.T) {
	registry := newMockRegistry()
	goNum := 5
	wg := &sync.WaitGroup{}
	wg.Add(goNum)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				assert.True(t, ServiceInGroup(registry, "testGroup", "testService"))
				assert.False(t, ServiceInGroup(registry, "testGroup", "testNoneExistService"))
			}
		}()
	}
	wg.Wait()
}

func BenchmarkServiceInGroup(b *testing.B) {
	registry := newMockRegistry()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ServiceInGroup(registry, "testGroup", "testService")
		}
	})
}

func BenchmarkGetAllGroups(b *testing.B) {
	registry := newMockRegistry()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			GetAllGroups(registry)
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

func newPipe() Pipe  {
	return nil
}

func newMockRegistry() *TestRegistry {
	registry := &TestRegistry{}
	registry.URL = &URL{Protocol: "mock", Host: "testHost", Port: 0}
	registry.URL.GetIdentity()
	registry.GroupService = map[string][]string{"testGroup": {"testService"}}
	return registry
}

func newDiscoverErrorRegistry() *TestRegistry {
	registry := newMockRegistry()
	registry.URL = &URL{Protocol: "mockDiscoverError", Host: "testHost", Port: 0}
	registry.URL.GetIdentity()
	registry.DiscoverError = true
	return registry
}
