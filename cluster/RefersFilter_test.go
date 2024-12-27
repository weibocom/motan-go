package cluster

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	motan "github.com/weibocom/motan-go/core"
	"testing"
)

func TestRefersFilterConfig_Verify(t *testing.T) {
	cases := []struct {
		configString string
		err          error
	}{
		{
			configString: `[]`,
			err:          nil,
		},
		{
			configString: `[{"mode":"invalid"}]`,
			err:          fmt.Errorf("exist empty rule in filter config"),
		},
		{
			configString: `[{"rule":"123,21","mode":"invalid"}]`,
			err:          fmt.Errorf("invalid mode(invalid) for rule(123,21)"),
		},
		{
			configString: `[{"rule":"123,21","mode":"include"}]`,
			err:          nil,
		},
		{
			configString: `[{"rule":"123,21","mode":"include","group":"group1","service":"service1"}]`,
			err:          nil,
		},
	}

	for _, c := range cases {
		var config RefersFilterConfigList
		err := json.Unmarshal([]byte(c.configString), &config)
		assert.Nil(t, err)
		err = config.Verify()
		assert.Equal(t, c.err, err)
	}
}

func TestRefersFilterConfig_ParseRefersFilters(t *testing.T) {
	cases := []struct {
		desc       string
		config     RefersFilterConfigList
		assertFunc func(t *testing.T, config RefersFilterConfigList)
	}{
		{
			desc:   "empty refer filter",
			config: RefersFilterConfigList{},
			assertFunc: func(t *testing.T, config RefersFilterConfigList) {
				filter := config.ParseRefersFilters(&motan.URL{Group: "g1", Path: "s1"})
				assert.Nil(t, filter)
			},
		},
		{
			desc: "include rule",
			config: RefersFilterConfigList{
				{
					Mode: FilterModeInclude,
					Rule: "123,1",
				},
			},
			assertFunc: func(t *testing.T, config RefersFilterConfigList) {
				filter := config.ParseRefersFilters(&motan.URL{Group: "g1", Path: "s1"})
				assert.NotNil(t, filter)
				df := filter.(*DefaultRefersFilter)
				assert.Equal(t, 1, len(df.includeRules))
				assert.Equal(t, 2, len(df.includeRules[0].prefixes))
				assert.Equal(t, 0, len(df.excludeRules))
			},
		},
		{
			desc: "mix rule",
			config: RefersFilterConfigList{
				{
					Mode:    FilterModeInclude,
					Rule:    "123,1",
					Group:   "g1",
					Service: "s1",
				},
				{
					Rule:    "123,1",
					Mode:    FilterModeInclude,
					Group:   "g2",
					Service: "s2",
				},
				{
					Rule:    "123,1",
					Mode:    FilterModeInclude,
					Group:   "g",
					Service: "",
				},
				{
					Rule:    "123,1",
					Mode:    FilterModeInclude,
					Group:   "",
					Service: "s",
				},
			},
			assertFunc: func(t *testing.T, config RefersFilterConfigList) {
				filter1 := config.ParseRefersFilters(&motan.URL{Group: "g1", Path: "s1"})
				assert.NotNil(t, filter1)
				filter2 := config.ParseRefersFilters(&motan.URL{Group: "g2", Path: "s2"})
				assert.NotNil(t, 1, filter2)
				filter3 := config.ParseRefersFilters(&motan.URL{Group: "g3", Path: "s3"})
				assert.Nil(t, filter3)
				filter4 := config.ParseRefersFilters(&motan.URL{Group: "g1", Path: "s4"})
				assert.Nil(t, filter4)
				filter5 := config.ParseRefersFilters(&motan.URL{Group: "g", Path: "s4"})
				assert.NotNil(t, filter5)
				filter6 := config.ParseRefersFilters(&motan.URL{Group: "g4", Path: "s"})
				assert.NotNil(t, filter6)
			},
		},
	}
	for _, c := range cases {
		t.Logf("test case: %s start", c.desc)
		c.assertFunc(t, c.config)
		t.Logf("test case: %s finish", c.desc)
	}
}

func TestDefaultRefersFilter_Filter(t *testing.T) {
	cases := []struct {
		desc       string
		filter     *DefaultRefersFilter
		assertFunc func(t *testing.T, filter DefaultRefersFilter)
	}{
		{
			desc: "include prefix filter",
			filter: NewDefaultRefersFilter([]RefersFilterConfig{
				{
					Mode: FilterModeInclude,
					Rule: "123,124",
				},
			}),
			assertFunc: func(t *testing.T, filter DefaultRefersFilter) {
				refers := []motan.EndPoint{
					&motan.TestEndPoint{URL: &motan.URL{Host: "123.1.2.0"}},
					&motan.TestEndPoint{URL: &motan.URL{Host: "124.1.2.0"}},
				}
				newRefers := filter.Filter(refers)
				assert.Equal(t, len(refers), len(newRefers))

				refers = []motan.EndPoint{
					&motan.TestEndPoint{URL: &motan.URL{Host: "110.1.2.0"}},
					&motan.TestEndPoint{URL: &motan.URL{Host: "111.1.2.0"}},
				}
				newRefers = filter.Filter(refers)
				assert.Equal(t, 0, len(newRefers))
			},
		},
		{
			desc: "include subnet filter",
			filter: NewDefaultRefersFilter([]RefersFilterConfig{
				{
					Mode: FilterModeInclude,
					Rule: "10.93.0.0/16",
				},
			}),
			assertFunc: func(t *testing.T, filter DefaultRefersFilter) {
				refers := []motan.EndPoint{
					&motan.TestEndPoint{URL: &motan.URL{Host: "10.93.1.1"}},
					&motan.TestEndPoint{URL: &motan.URL{Host: "10.93.2.10"}},
				}
				newRefers := filter.Filter(refers)
				assert.Equal(t, len(refers), len(newRefers))

				refers = []motan.EndPoint{
					&motan.TestEndPoint{URL: &motan.URL{Host: "10.94.1.1"}},
					&motan.TestEndPoint{URL: &motan.URL{Host: "10.95.1.1"}},
				}
				newRefers = filter.Filter(refers)
				assert.Equal(t, 0, len(newRefers))
			},
		},
		{
			desc: "exclude prefix filter",
			filter: NewDefaultRefersFilter([]RefersFilterConfig{
				{
					Mode: FilterModeExclude,
					Rule: "123,177",
				},
			}),
			assertFunc: func(t *testing.T, filter DefaultRefersFilter) {
				refers := []motan.EndPoint{
					&motan.TestEndPoint{URL: &motan.URL{Host: "123.1.2.0"}},
					&motan.TestEndPoint{URL: &motan.URL{Host: "177.1.2.0"}},
				}
				newRefers := filter.Filter(refers)
				assert.Equal(t, 0, len(newRefers))

				refers = []motan.EndPoint{
					&motan.TestEndPoint{URL: &motan.URL{Host: "121.1.2.0"}},
					&motan.TestEndPoint{URL: &motan.URL{Host: "122.1.2.0"}},
				}
				newRefers = filter.Filter(refers)
				assert.Equal(t, len(refers), len(newRefers))
			},
		},
		{
			desc: "exclude subnet filter",
			filter: NewDefaultRefersFilter([]RefersFilterConfig{
				{
					Mode: FilterModeExclude,
					Rule: "10.93.0.0/16",
				},
			}),
			assertFunc: func(t *testing.T, filter DefaultRefersFilter) {
				refers := []motan.EndPoint{
					&motan.TestEndPoint{URL: &motan.URL{Host: "10.93.1.1"}},
					&motan.TestEndPoint{URL: &motan.URL{Host: "10.93.2.10"}},
				}
				newRefers := filter.Filter(refers)
				assert.Equal(t, 0, len(newRefers))

				refers = []motan.EndPoint{
					&motan.TestEndPoint{URL: &motan.URL{Host: "10.94.1.1"}},
					&motan.TestEndPoint{URL: &motan.URL{Host: "10.95.1.1"}},
				}
				newRefers = filter.Filter(refers)
				assert.Equal(t, len(refers), len(newRefers))
			},
		},
		{
			desc: "mix filter",
			filter: NewDefaultRefersFilter([]RefersFilterConfig{
				{
					Mode: FilterModeInclude,
					Rule: "123,190,10.93.0.0/16",
				},
				{
					Mode: FilterModeExclude,
					Rule: "123,145,10.96.0.0/16",
				},
			}),
			assertFunc: func(t *testing.T, filter DefaultRefersFilter) {
				refers := []motan.EndPoint{
					&motan.TestEndPoint{URL: &motan.URL{Host: "123.1.2.0"}},
					&motan.TestEndPoint{URL: &motan.URL{Host: "145.1.2.0"}},
					&motan.TestEndPoint{URL: &motan.URL{Host: "190.1.2.0"}},
					&motan.TestEndPoint{URL: &motan.URL{Host: "110.1.2.0"}},
					&motan.TestEndPoint{URL: &motan.URL{Host: "10.93.1.2"}},
					&motan.TestEndPoint{URL: &motan.URL{Host: "10.96.1.2"}},
					&motan.TestEndPoint{URL: &motan.URL{Host: "190.96"}}, // retained invalid host
				}
				newRefers := filter.Filter(refers)
				assert.Equal(t, 3, len(newRefers))
			},
		},
	}

	for _, f := range cases {
		t.Logf("test case: %s start", f.desc)
		f.assertFunc(t, *f.filter)
		t.Logf("test case: %s finish", f.desc)
	}
}

func TestNewDefaultRefersFilter(test *testing.T) {
	fr := NewDefaultRefersFilter([]RefersFilterConfig{
		{
			Mode: FilterModeInclude,
			Rule: "123.1,10.2,10.93.0.0/18,10.93.0.0/34", // discard invalid subnet rule
		},
		{
			Mode: FilterModeExclude,
			Rule: "124.1,10.1,10.14.0.0/18,10.14.0.0/34", // discard invalid subnet rule
		},
	})
	assert.NotNil(test, fr)
	assert.Equal(test, 1, len(fr.includeRules))
	assert.Equal(test, 1, len(fr.excludeRules))
	assert.Equal(test, 2, len(fr.includeRules[0].prefixes))
	assert.Equal(test, 1, len(fr.includeRules[0].subnets))
	assert.Equal(test, 2, len(fr.excludeRules[0].prefixes))
	assert.Equal(test, 1, len(fr.excludeRules[0].subnets))
}
