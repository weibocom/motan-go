package ha

import (
	"testing"

	motan "github.com/weibocom/motan-go/core"
)

func NewFailOver(url *motan.Url) motan.HaStrategy {
	return &FailOverHA{url: url}
}

func TestGetHa(t *testing.T) {
	defaultExtFactory := &motan.DefaultExtentionFactory{}
	defaultExtFactory.Initialize()
	RegistDefaultHa(defaultExtFactory)
	p := make(map[string]string)
	p[motan.Hakey] = FailOver
	url := &motan.Url{Parameters: p}
	ha := defaultExtFactory.GetHa(url)
	if ha.GetName() != "failover" {
		t.Error("GetHa name Error")
	}
	getUrl := ha.GetUrl()
	if url.GetIdentity() != getUrl.GetIdentity() {
		t.Error("GetUrl Error")
	}
}
