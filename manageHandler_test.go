package motan

import (
	"encoding/json"
	assert2 "github.com/stretchr/testify/assert"
	motan "github.com/weibocom/motan-go/core"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSleep(t *testing.T) {
	start := time.Now()
	sleep(&httptest.ResponseRecorder{}, time.Second*3)
	end := time.Now()
	assert2.True(t, end.Sub(start).Seconds() >= 3)
}

func TestGetAllService(t *testing.T) {
	i := InfoHandler{}
	ctx := &motan.Context{
		ServiceURLs: map[string]*motan.URL{
			"test": &motan.URL{
				Group: "testgroup",
				Path:  "testpath",
			},
		},
	}
	agent := &Agent{Context: ctx}
	agent.configurer = NewDynamicConfigurer(agent)
	i.a = agent
	serviceInfo := i.getAllServices()
	resMap := make(map[string][]*motan.URL)
	err := json.Unmarshal(serviceInfo, &resMap)
	assert2.Nil(t, err)
	assert2.Equal(t, len(resMap["services"]), 1)
}
