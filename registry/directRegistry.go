package registry

import (
	"strconv"
	"strings"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

type DirectRegistry struct {
	url  *motan.URL
	urls []*motan.URL
}

func (d *DirectRegistry) GetURL() *motan.URL {
	return d.url
}
func (d *DirectRegistry) SetURL(url *motan.URL) {
	d.url = url
	d.urls = parseURLs(url)
}
func (d *DirectRegistry) GetName() string {
	return "direct"
}

func (d *DirectRegistry) InitRegistry() {
}
func (d *DirectRegistry) Subscribe(url *motan.URL, listener motan.NotifyListener) {
}

func (d *DirectRegistry) Unsubscribe(url *motan.URL, listener motan.NotifyListener) {
}
func (d *DirectRegistry) Discover(url *motan.URL) []*motan.URL {
	if d.urls == nil {
		d.urls = parseURLs(d.url)
	}
	result := make([]*motan.URL, 0, len(d.urls))
	for _, u := range d.urls {
		newURL := url.Copy()
		newURL.Host = u.Host
		newURL.Port = u.Port
		if u.Group != "" { // specify group
			newURL.Group = u.Group
		}
		result = append(result, newURL)
	}
	return result
}
func (d *DirectRegistry) Register(serverURL *motan.URL) {
	vlog.Infof("direct registry:register url :%+v", serverURL)
}
func (d *DirectRegistry) UnRegister(serverURL *motan.URL) {

}
func (d *DirectRegistry) Available(serverURL *motan.URL) {

}
func (d *DirectRegistry) Unavailable(serverURL *motan.URL) {

}
func (d *DirectRegistry) GetRegisteredServices() []*motan.URL {
	return nil
}
func (d *DirectRegistry) StartSnapshot(conf *motan.SnapshotConf) {}
func parseURLs(url *motan.URL) []*motan.URL {
	urls := make([]*motan.URL, 0)
	if len(url.Host) > 0 && url.Port > 0 {
		urls = append(urls, url.Copy())
	} else if address, exist := url.Parameters[motan.AddressKey]; exist {
		for _, add := range strings.Split(address, ",") {
			u := url.Copy()
			if strings.HasPrefix(add, "unix://") {
				u.Host = add
				u.Port = 0
				u.PutParam(motan.AddressKey, add)
				urls = append(urls, u)
			} else {
				hostport := motan.TrimSplit(add, ":")
				if len(hostport) == 2 {
					port, err := strconv.Atoi(hostport[1])
					if err == nil {
						u.Host = hostport[0]
						u.Port = port
						urls = append(urls, u)
					}
				}
			}
		}
	} else {
		vlog.Warningf("direct registry parse fail.url:L %+v", url)
	}
	return urls
}
