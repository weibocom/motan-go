package core

import (
	"strconv"
	"sync"
	"testing"
)

func TestExtFactory(t *testing.T) {
	ext := DefaultExtentionFactory{}
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
