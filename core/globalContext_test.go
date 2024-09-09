package core

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weibocom/motan-go/config"

	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	flag.Parse()
	m.Run()
}

func TestGetContext(t *testing.T) {
	rs := &Context{ConfigFile: "../config/testconf.yaml"}
	rs.Initialize()
	assert.NotNil(t, rs.RefersURLs, "parse refers urls fail.")
	assert.NotNil(t, rs.RefersURLs["status-rpc-json"], "parse refer section fail.")
	assert.Equal(t, "test-group", rs.RefersURLs["status-rpc-json"].Group, "get refer key fail.")
	assert.Contains(t, rs.RefersURLs["status-rpc-json"].Parameters["filter"], "accessLog", "get refer filter fail.")
	assert.Contains(t, rs.RefersURLs["status-rpc-json"].Parameters["filter"], "metrics", "get refer filter fail.")
	assert.NotEqual(t, 0, len(rs.ServiceURLs), "parse service urls fail")
	assert.Equal(t, "motan-demo-rpc", rs.ServiceURLs["mytest-motan2"].Group, "parse serivce key fail")
}

func TestCompatible(t *testing.T) {
	rs := &Context{ConfigFile: "../config/compatible.yaml"}
	rs.Initialize()
	assert.NotNil(t, rs.RefersURLs)
	for _, j := range rs.RefersURLs {
		assert.Equal(t, j.Protocol, "motan")
	}
}

func TestNewContext(t *testing.T) {
	configFile := filepath.Join("testdata", "app.yaml")
	pool1Context := NewContext(configFile, "app", "app-idc1")
	pool2Context := NewContext(configFile, "app", "app-idc2")
	pool3Context := NewContext(configFile, "app", "app-idc3")
	pool4Context := NewContext(configFile, "app", "app-idc4")

	testPlaceholder, _ := pool1Context.Config.GetSection("test_placeholder")
	assert.Equal(t, "aaa1", testPlaceholder["aaa"])
	assert.Equal(t, "bbb1", testPlaceholder["sub"].(map[interface{}]interface{})["bbb"])
	assert.Equal(t, "ccc1", testPlaceholder["ccc"])

	testPlaceholder, _ = pool2Context.Config.GetSection("test_placeholder")
	assert.Equal(t, "aaa2", testPlaceholder["aaa"])
	assert.Equal(t, "bbb2", testPlaceholder["sub"].(map[interface{}]interface{})["bbb"])
	assert.Equal(t, "ccc2", testPlaceholder["ccc"])

	testPlaceholder, _ = pool3Context.Config.GetSection("test_placeholder")
	assert.Equal(t, "aaa3", testPlaceholder["aaa"])
	assert.Equal(t, "bbb3", testPlaceholder["sub"].(map[interface{}]interface{})["bbb"])
	assert.Equal(t, "ccc3", testPlaceholder["ccc"])

	testPlaceholder, _ = pool4Context.Config.GetSection("test_placeholder")
	assert.Equal(t, "aaa_default", testPlaceholder["aaa"])
	assert.Equal(t, "bbb_default", testPlaceholder["sub"].(map[interface{}]interface{})["bbb"])
	assert.Equal(t, "ccc_default", testPlaceholder["ccc"])

	pool1Context = NewContext("testdata", "app", "app-idc1")
	testPlaceholder, _ = pool1Context.Config.GetSection("test_placeholder")
	assert.Equal(t, "aaa1", testPlaceholder["aaa"])
	assert.Equal(t, "bbb1", testPlaceholder["sub"].(map[interface{}]interface{})["bbb"])
	assert.Equal(t, 1000, testPlaceholder["ccc"])

	errContext := NewContext("testdata", "", "")
	assert.Nil(t, errContext.Config)
	context := NewContext("testdata", "", "app-idc1")
	assert.NotNil(t, context.Config)
	context = NewContext("testdata", "app", "")
	assert.NotNil(t, context.Config)

	globalFilterContext := NewContext("testdata", "globalFilter", "app-idc1")

	assert.NotNil(t, globalFilterContext)

	filterArrs := make(map[string][]string)
	for _, section := range []string{"test-motan-refer", "test-global-filter-service-1-refer", "test-global-filter-service-2-refer"} {
		filterArrs[section] = strings.Split(globalFilterContext.RefersURLs[section].Parameters["filter"], ",")
	}
	for _, section := range []string{"test-global-filter-0-service", "test-global-filter-1-service", "test-global-filter-2-service"} {
		filterArrs[section] = strings.Split(globalFilterContext.ServiceURLs[section].Parameters["filter"], ",")
	}

	for key, filterArr := range filterArrs {
		t.Run(key, func(t *testing.T) {
			assert.Contains(t, filterArr, "testGlobalFilter1")
			assert.Contains(t, filterArr, "testGlobalFilter2")
			assert.Contains(t, filterArr, "testGlobalFilter3")
			assert.Contains(t, filterArr, "accessLog")
			assert.Contains(t, filterArr, "metrics")
		})
	}

	filterArrs = map[string][]string{
		"test-global-filter-service-3-refer": strings.Split(globalFilterContext.RefersURLs["test-global-filter-service-3-refer"].Parameters["filter"], ","),
		"test-global-filter-3-service":       strings.Split(globalFilterContext.ServiceURLs["test-global-filter-3-service"].Parameters["filter"], ","),
	}
	for key, filterArr := range filterArrs {
		t.Run(key, func(t *testing.T) {
			assert.Contains(t, filterArr, "testGlobalFilter1")
			assert.NotContains(t, filterArr, "testGlobalFilter2")
			assert.NotContains(t, filterArr, "testGlobalFilter3")
			assert.Contains(t, filterArr, "accessLog")
			assert.Contains(t, filterArr, "metrics")
		})
	}
}

func Test_fixMergeGlobalFilter(t *testing.T) {
	testCases := []map[string]interface{}{
		{
			"config": `
motan-server:
  application: testFix

motan-service:
  mytest:
    path: test
`,
			"expect_value": "",
			"expect_ok":    false,
		},
		{
			"config": `
motan-server:
  application: testFix

motan-service:
  mytest:
    path: test
    filter: test
`,
			"expect_value": "test",
			"expect_ok":    true,
		},
	}
	for _, testCase := range testCases {
		conf, err := config.NewConfigFromReader(bytes.NewReader([]byte(testCase["config"].(string))))
		assert.Nil(t, err)
		context := NewContextFromConfig(conf, "", "")
		Initialize(context)
		for _, url := range context.ServiceURLs {
			v, ok := url.Parameters[FilterKey]
			assert.Equal(t, testCase["expect_value"].(string), v)
			assert.Equal(t, testCase["expect_ok"].(bool), ok)
		}
	}
}

func TestContext_parseMultipleServiceGroup(t *testing.T) {
	data0 := map[string]*URL{
		"service1": {
			Group: "",
		},
	}
	data1 := map[string]*URL{
		"service1": {
			Group: "hello",
		},
	}
	data2 := map[string]*URL{
		"service1": {
			Group: "hello,hello1,hello2",
		},
	}
	data3 := map[string]*URL{
		"service1": {
			Group: "hello,hello1",
		},
	}
	data4 := map[string]*URL{
		"service1": {
			Group: "",
		},
	}
	ctx := Context{}
	ctx.parseMultipleServiceGroup(map[string]*URL{})

	ctx.parseMultipleServiceGroup(data0)
	assert.Len(t, data0, 1)

	ctx.parseMultipleServiceGroup(data1)
	assert.Len(t, data1, 1)

	ctx.parseMultipleServiceGroup(data2)
	assert.Len(t, data2, 3)
	assert.Equal(t, data2["service1"].Group, "hello")
	assert.Equal(t, data2["service1-0"].Group, "hello1")
	assert.Equal(t, data2["service1-1"].Group, "hello2")

	os.Setenv(GroupEnvironmentName, "hello2")
	ctx.parseMultipleServiceGroup(data3)
	assert.Len(t, data3, 3)
	assert.Equal(t, data3["service1"].Group, "hello")
	assert.Equal(t, data3["service1-0"].Group, "hello1")
	assert.Equal(t, data3["service1-1"].Group, "hello2")

	os.Setenv(GroupEnvironmentName, "hello")
	ctx.parseMultipleServiceGroup(data4)
	assert.Len(t, data4, 1)
	assert.Equal(t, data3["service1"].Group, "hello")
}

func TestContext_parseRegGroupSuffix(t *testing.T) {
	regGroupSuffix := "-test"
	countGroup := func(urlMap map[string]*URL) map[string]int {
		groupMap := map[string]int{}
		for _, i2 := range urlMap {
			groupMap[i2.Group] += 1
		}
		return groupMap
	}
	cases := []struct {
		UrlMap     map[string]*URL
		AssertFunc func(t *testing.T, urlMap map[string]*URL)
	}{
		{
			UrlMap: map[string]*URL{
				"service1": {
					Group: "group1",
				},
				"service2": {
					Group: "group1",
				},
				"service3": {
					Group: "group2",
				},
				"service4": {
					Group: "group3",
				},
				"service5": {
					Group: "group3" + regGroupSuffix,
				},
				"service6": {
					Group: "group4",
					Parameters: map[string]string{
						RegistryKey: "reg1",
					},
				},
				"service7": {
					Group: "group4",
					Parameters: map[string]string{
						RegistryKey: "reg2",
					},
				},
				"service8": {
					Group: "group5",
					Parameters: map[string]string{
						RegistryKey: "reg1",
					},
				},
				"service9": {
					Group: "group5" + regGroupSuffix,
					Parameters: map[string]string{
						RegistryKey: "reg2",
					},
				},
			},
			AssertFunc: func(t *testing.T, urlMap map[string]*URL) {
				groupMap := countGroup(urlMap)
				assert.Equal(t, 0, groupMap["group1"])
				assert.Equal(t, 2, groupMap["group1"+regGroupSuffix])
				assert.Equal(t, 1, groupMap["group2"+regGroupSuffix])
				assert.Equal(t, 0, groupMap["group3"])
				assert.Equal(t, 2, groupMap["group3"+regGroupSuffix])
				assert.Equal(t, 2, groupMap["group4"+regGroupSuffix])
				assert.Equal(t, 2, groupMap["group5"+regGroupSuffix])
			},
		},
	}
	os.Setenv(RegGroupSuffix, regGroupSuffix)
	ctx := &Context{}
	// replace all groups with regGroupSuffix
	for _, s := range cases {
		ctx.parseRegGroupSuffix(s.UrlMap)
		s.AssertFunc(t, s.UrlMap)
	}
	os.Unsetenv(RegGroupSuffix)
}

func TestContext_parseSubGroupSuffix(t *testing.T) {
	subGroupSuffix := "-test"
	countGroup := func(urlMap map[string]*URL) map[string]int {
		groupMap := map[string]int{}
		for _, i2 := range urlMap {
			groupMap[i2.Group] += 1
		}
		return groupMap
	}
	cases := []struct {
		Ctx        *Context
		UrlMap     map[string]*URL
		AssertFunc func(t *testing.T, urlMap map[string]*URL)
	}{
		{
			Ctx: &Context{
				AgentURL: &URL{},
			},
			UrlMap: map[string]*URL{
				"refer1": {
					Group: "group1",
					Path:  "p1",
				},
				"refer2": {
					Group: "group1",
					Path:  "p2",
				},
				"refer3": {
					Group: "group2",
				},
				"refer4": {
					Group: "group3",
				},
				"refer5": {
					Group: "group3" + subGroupSuffix,
				},
			},
			AssertFunc: func(t *testing.T, urlMap map[string]*URL) {
				groupMap := countGroup(urlMap)
				assert.Equal(t, 2, groupMap["group1"])
				assert.Equal(t, 2, groupMap["group1"+subGroupSuffix])
				assert.Equal(t, 1, groupMap["group2"])
				assert.Equal(t, 1, groupMap["group2"+subGroupSuffix])
				assert.Equal(t, 1, groupMap["group3"])
				assert.Equal(t, 1, groupMap["group3"+subGroupSuffix])
			},
		},
		{
			Ctx: &Context{},
			UrlMap: map[string]*URL{
				"refer1": {
					Group: "group1",
					Path:  "p1",
				},
				"refer2": {
					Group: "group1",
					Path:  "p2",
				},
				"refer3": {
					Group: "group2",
				},
				"refer4": {
					Group: "group3",
				},
				"refer5": {
					Group: "group3" + subGroupSuffix,
				},
			},
			AssertFunc: func(t *testing.T, urlMap map[string]*URL) {
				groupMap := countGroup(urlMap)
				assert.Equal(t, 2, groupMap["group1"])
				assert.Equal(t, 0, groupMap["group1"+subGroupSuffix])
				assert.Equal(t, 1, groupMap["group2"])
				assert.Equal(t, 0, groupMap["group2"+subGroupSuffix])
				assert.Equal(t, 1, groupMap["group3"])
				assert.Equal(t, 1, groupMap["group3"+subGroupSuffix])
				assert.Equal(t, 5, len(urlMap))
			},
		},
	}
	os.Setenv(SubGroupSuffix, subGroupSuffix)
	for _, s := range cases {
		s.Ctx.parseSubGroupSuffix(s.UrlMap)
		s.AssertFunc(t, s.UrlMap)
	}
	os.Unsetenv(SubGroupSuffix)
}

func TestContext_mergeDefaultFilter(t *testing.T) {
	c := Context{AgentURL: &URL{
		Parameters: map[string]string{"defaultFilter": "a,b,d"},
	}}
	u1 := &URL{
		Parameters: map[string]string{"filter": "a,c", "disableDefaultFilter": "b"},
	}
	u2 := &URL{
		Parameters: map[string]string{"filter": "a,c", "disableDefaultFilter": "b"},
	}

	u1.Parameters[FilterKey] = c.FilterSetToStr(
		c.MergeFilterSet(
			c.GetDefaultFilterSet(u1), c.GetFilterSet(u1.Parameters[FilterKey], ""),
		),
	)

	for _, v := range strings.Split("a,d,c", ",") {
		assert.Contains(t, u1.Parameters["filter"], v)
	}

	c = Context{}
	u2.Parameters[FilterKey] = c.FilterSetToStr(
		c.MergeFilterSet(
			c.GetDefaultFilterSet(u1), c.GetFilterSet(u2.Parameters[FilterKey], ""),
		),
	)
	for _, v := range strings.Split("a,c", ",") {
		assert.Contains(t, u1.Parameters["filter"], v)
	}

	c = Context{AgentURL: &URL{}}
	u2.Parameters[FilterKey] = c.FilterSetToStr(
		c.MergeFilterSet(
			c.GetDefaultFilterSet(u1), c.GetFilterSet(u2.Parameters[FilterKey], ""),
		),
	)
	for _, v := range strings.Split("a,c", ",") {
		assert.Contains(t, u2.Parameters["filter"], v)
	}
}

func TestContext_getFilterSet(t *testing.T) {
	c := Context{}
	a := "a,b,"
	b := "b,"
	assert.Equal(t, c.GetFilterSet("a", ""), c.GetFilterSet(a, b))
}

func TestContext_mergeFilterSet(t *testing.T) {
	c := Context{}
	a := c.GetFilterSet("a,b,c,", "")
	b := c.GetFilterSet("b,", "")
	for v := range c.MergeFilterSet(a, b) {
		assert.Contains(t, "a,b,c", v)
	}
}
