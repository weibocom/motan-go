package filter

import (
	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/core"
	"testing"
)

func TestGetFilterRuntimeInfo(t *testing.T) {
	ext := &core.DefaultExtensionFactory{}
	ext.Initialize()
	RegistDefaultFilters(ext)

	for _, name := range []string{
		AccessLog,
		Metrics,
		CircuitBreaker,
		FailFast,
		Trace,
		RateLimit,
		ClusterAccessLog,
		ClusterMetrics,
		ClusterCircuitBreaker,
	} {
		filter := ext.GetFilter(name)
		assert.NotNil(t, filter)
		info := filter.GetRuntimeInfo()
		assert.NotNil(t, info)

		filterName, ok := info[core.RuntimeNameKey]
		assert.True(t, ok)
		assert.Equal(t, name, filterName)

		index, ok := info[core.RuntimeIndexKey]
		assert.True(t, ok)
		assert.Equal(t, filter.GetIndex(), index)

		filterType, ok := info[core.RuntimeTypeKey]
		assert.True(t, ok)
		assert.Equal(t, filter.GetType(), filterType)
	}
}
