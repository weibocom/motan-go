package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/core"
)

func TestLocalRegistry_Discover(t *testing.T) {
	localRegistry := &LocalRegistry{}
	service := "testService"
	group := "testGroup"
	discoveredURL := localRegistry.Discover(&core.URL{Path: service, Group: group})[0]
	assert.Equal(t, core.ProtocolLocal, discoveredURL.Protocol)
	assert.Equal(t, service, discoveredURL.Path)
	assert.Equal(t, group, discoveredURL.Group)
}
