package registry

import (
	"strconv"
	"strings"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

type DirectRegistry struct {
	url  *motan.Url
	urls []*motan.Url
}

func (d *DirectRegistry) GetUrl() *motan.Url {
	return d.url
}
func (d *DirectRegistry) SetUrl(url *motan.Url) {
	d.url = url
	d.urls = parseUrls(url)
}
func (d *DirectRegistry) GetName() string {
	return "direct"
}

func (d *DirectRegistry) InitRegistry() {
}
func (d *DirectRegistry) Subscribe(url *motan.Url, listener motan.NotifyListener) {
}

func (d *DirectRegistry) Unsubscribe(url *motan.Url, listener motan.NotifyListener) {
}
func (d *DirectRegistry) Discover(url *motan.Url) []*motan.Url {
	if d.urls == nil {
		d.urls = parseUrls(d.url)
	}
	result := make([]*motan.Url, 0, len(d.urls))
	for _, u := range d.urls {
		newUrl := *url
		newUrl.Host = u.Host
		newUrl.Port = u.Port
		result = append(result, &newUrl)
	}
	return result
}
func (d *DirectRegistry) Register(serverUrl *motan.Url) {
	vlog.Infof("direct registry:register url :%+v\n", serverUrl)
}
func (d *DirectRegistry) UnRegister(serverUrl *motan.Url) {

}
func (d *DirectRegistry) Available(serverUrl *motan.Url) {

}
func (d *DirectRegistry) Unavailable(serverUrl *motan.Url) {

}
func (d *DirectRegistry) GetRegisteredServices() []*motan.Url {
	return nil
}
func (v *DirectRegistry) StartSnapshot(conf *motan.SnapshotConf) {}
func parseUrls(url *motan.Url) []*motan.Url {
	urls := make([]*motan.Url, 0)
	if len(url.Host) > 0 && url.Port > 0 {
		urls = append(urls, url)
	} else if address, exist := url.Parameters["address"]; exist {
		for _, add := range strings.Split(address, ",") {
			hostport := strings.Split(add, ":")
			if len(hostport) == 2 {
				port, err := strconv.Atoi(hostport[1])
				if err == nil {
					u := &motan.Url{Host: hostport[0], Port: port}
					urls = append(urls, u)
				}
			}
		}
	} else {
		vlog.Warningf("direct registry parse fail.url:L %+v", url)
	}
	return urls
}
