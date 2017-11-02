package core

import (
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
