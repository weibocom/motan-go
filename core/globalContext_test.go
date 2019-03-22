package core

import (
	"flag"
	"path/filepath"
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
}
