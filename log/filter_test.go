package vlog

import (
	"encoding/json"
	assert2 "github.com/stretchr/testify/assert"
	"testing"
)

var (
	asyncLogChanTest = make(chan *FilterItem, 100)
)

func Test_AsyncFilter(t *testing.T) {
	assert := assert2.New(t)
	assert.Equal(0, len(asyncFilters))
	f1 := &testAsyncFilter{
		channel: asyncLogChanTest,
	}
	AddAsyncFilter("test1", f1)
	assert.Equal(1, len(asyncFilters))

	doAsyncFilters(InfoLevel, EntrypointInfof, "", "3123")
	doAsyncFilters(InfoLevel, EntrypointInfof, "", "3123")
	doAsyncFilters(InfoLevel, EntrypointInfof, "", "3123")
	doAsyncFilters(InfoLevel, EntrypointInfof, "", "3123")
	assert.True(true, len(asyncLogChanTest)+len(asyncFilterItemChan) >= 4)

	f2 := &testAsyncFilter{
		channel: asyncLogChanTest,
	}
	AddAsyncFilter("test2", f2)
	assert.Equal(2, len(asyncFilters))
	doAsyncFilters(InfoLevel, EntrypointInfof, "", "3123")
	assert.True(true, len(asyncLogChanTest)+len(asyncFilterItemChan) > 5)

	f3 := &testAsyncFilter{
		channel: asyncLogChanTest,
	}
	AddAsyncFilter("test3", f3)
	assert.Equal(3, len(asyncFilters))
	doAsyncFilters(InfoLevel, EntrypointInfof, "", "3123")
	assert.True(true, len(asyncLogChanTest)+len(asyncFilterItemChan) > 6)

	accessEntry := &AccessLogEntity{
		FilterName:    "",
		Role:          "",
		RequestID:     0,
		Service:       "",
		Method:        "",
		Desc:          "",
		RemoteAddress: "",
		ReqSize:       0,
		ResSize:       0,
		BizTime:       0,
		TotalTime:     0,
		Success:       false,
		ResponseCode:  "",
		Exception:     "",
	}
	accessLogBytes, _ := json.Marshal(accessEntry)
	getLogContentTestCases := []map[string]interface{}{
		{
			"format":     "",
			"fields":     []interface{}{},
			"entrypoint": EntrypointInfof,
			"expect":     "",
		},
		{
			"format": "s%s",
			"fields": []interface{}{
				"1",
			},
			"entrypoint": EntrypointInfof,
			"expect":     "s1",
		},
		{
			"format": "",
			"fields": []interface{}{
				accessEntry,
			},
			"entrypoint": EntrypointAccessLog,
			"expect":     string(accessLogBytes),
		},
	}
	for _, testCase := range getLogContentTestCases {
		res := getLogContent(testCase["entrypoint"].(Entrypoint), testCase["format"].(string), testCase["fields"].([]interface{})...)
		assert.Equal(testCase["expect"].(string), res)
	}
}

type testAsyncFilter struct {
	channel chan *FilterItem
}

func (f *testAsyncFilter) GetChannel() chan *FilterItem {
	return f.channel
}
