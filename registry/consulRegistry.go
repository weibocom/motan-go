package registry

import (
	"time"

	motan "github.com/weibocom/motan-go/core"
)

// ConsulRegistry TODO implement
type ConsulRegistry struct {
	url           *motan.URL
	timeout       time.Duration
	heartInterval time.Duration
}

func (v *ConsulRegistry) GetURL() *motan.URL {
	return v.url
}
func (v *ConsulRegistry) SetURL(url *motan.URL) {
	v.url = url
}
func (v *ConsulRegistry) GetName() string {
	return "consulRegistry"
}

func (v *ConsulRegistry) Initialize() {

}
func (v *ConsulRegistry) Subscribe(url *motan.URL, listener motan.NotifyListener) {

}

func (v *ConsulRegistry) Unsubscribe(url *motan.URL, listener motan.NotifyListener) {

}
func (v *ConsulRegistry) Discover(url *motan.URL) []*motan.URL {
	return nil
}
func (v *ConsulRegistry) Register(serverURL *motan.URL) {

}
func (v *ConsulRegistry) UnRegister(serverURL *motan.URL) {

}
func (v *ConsulRegistry) Available(serverURL *motan.URL) {

}
func (v *ConsulRegistry) Unavailable(serverURL *motan.URL) {

}
func (v *ConsulRegistry) GetRegisteredServices() []*motan.URL {
	return nil
}
func (v *ConsulRegistry) StartSnapshot(conf *motan.SnapshotConf) {}
