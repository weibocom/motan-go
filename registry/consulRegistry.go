package registry

import (
	"time"

	motan "github.com/weibocom/motan-go/core"
)

//TODO
type ConsulRegistry struct {
	url           *motan.Url
	timeout       time.Duration
	heartInterval time.Duration
}

func (v *ConsulRegistry) GetUrl() *motan.Url {
	return v.url
}
func (v *ConsulRegistry) SetUrl(url *motan.Url) {
	v.url = url
}
func (v *ConsulRegistry) GetName() string {
	return "consulRegistry"
}

func (v *ConsulRegistry) Initialize() {

}
func (v *ConsulRegistry) Subscribe(url *motan.Url, listener motan.NotifyListener) {

}

func (v *ConsulRegistry) Unsubscribe(url *motan.Url, listener motan.NotifyListener) {

}
func (v *ConsulRegistry) Discover(url *motan.Url) []*motan.Url {
	return nil
}
func (v *ConsulRegistry) Register(serverUrl *motan.Url) {

}
func (v *ConsulRegistry) UnRegister(serverUrl *motan.Url) {

}
func (v *ConsulRegistry) Available(serverUrl *motan.Url) {

}
func (v *ConsulRegistry) Unavailable(serverUrl *motan.Url) {

}
func (v *ConsulRegistry) GetRegisteredServices() []*motan.Url {
	return nil
}
func (v *ConsulRegistry) StartSnapshot(conf *motan.SnapshotConf) {}
