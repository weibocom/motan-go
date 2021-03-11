package registry

import (
	motan "github.com/weibocom/motan-go/core"
)

type LocalRegistry struct {
	url *motan.URL
}

func (d *LocalRegistry) GetURL() *motan.URL {
	return d.url
}

func (d *LocalRegistry) SetURL(url *motan.URL) {
	d.url = url
}
func (d *LocalRegistry) GetName() string {
	return "local"
}

func (d *LocalRegistry) InitRegistry() {
}

func (d *LocalRegistry) Subscribe(url *motan.URL, listener motan.NotifyListener) {
}

func (d *LocalRegistry) Unsubscribe(url *motan.URL, listener motan.NotifyListener) {
}

func (d *LocalRegistry) Discover(url *motan.URL) []*motan.URL {
	newURL := url.Copy()
	newURL.Protocol = motan.ProtocolLocal
	return []*motan.URL{newURL}
}

func (d *LocalRegistry) Register(serverURL *motan.URL) {
}

func (d *LocalRegistry) UnRegister(serverURL *motan.URL) {

}
func (d *LocalRegistry) Available(serverURL *motan.URL) {

}
func (d *LocalRegistry) Unavailable(serverURL *motan.URL) {

}
func (d *LocalRegistry) GetRegisteredServices() []*motan.URL {
	return nil
}

func (d *LocalRegistry) StartSnapshot(conf *motan.SnapshotConf) {}
