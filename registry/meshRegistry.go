package registry

import (
	motan "github.com/weibocom/motan-go/core"
)

type MeshRegistry struct {
	url              *motan.URL
	urls             []*motan.URL
	registerService  []*motan.URL
	subscribeService []*motan.URL
}

func (d *MeshRegistry) GetURL() *motan.URL {
	return d.url
}

func (d *MeshRegistry) SetURL(url *motan.URL) {
	d.url = url
	d.urls = parseURLs(url)
}

func (d *MeshRegistry) GetName() string {
	return "mesh"
}

func (d *MeshRegistry) InitRegistry() {
}

func (d *MeshRegistry) Subscribe(url *motan.URL, listener motan.NotifyListener) {
}

func (d *MeshRegistry) Unsubscribe(url *motan.URL, listener motan.NotifyListener) {
	// Do nothing
}

func (d *MeshRegistry) Discover(url *motan.URL) []*motan.URL {
	if d.urls == nil {
		d.urls = parseURLs(d.url)
	}
	result := make([]*motan.URL, 0, len(d.urls))
	for _, u := range d.urls {
		newURL := *url
		newURL.Host = u.Host
		meshPort, _ := url.GetInt("meshPort")
		newURL.Port = int(meshPort)
		result = append(result, &newURL)
	}
	return result
}

func (d *MeshRegistry) Register(serverURL *motan.URL) {
}

func (d *MeshRegistry) UnRegister(serverURL *motan.URL) {

}

func (d *MeshRegistry) Available(serverURL *motan.URL) {
}

func (d *MeshRegistry) Unavailable(serverURL *motan.URL) {
}

func (d *MeshRegistry) GetRegisteredServices() []*motan.URL {
	return d.registerService
}

func (d *MeshRegistry) StartSnapshot(conf *motan.SnapshotConf) {}
