package core

import (
	"bytes"
	"flag"
	"github.com/weibocom/motan-go/config"
	"path/filepath"
	"strings"
	"testing"

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
