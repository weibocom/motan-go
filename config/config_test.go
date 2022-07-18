package config

import (
	"fmt"
	"testing"

	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/assert"
)

type graphite struct {
	Host string
	Port int
	Pool string
}

func Test_Config(t *testing.T) {
	//TODO
	c, err := NewConfigFromFile("./testconf.yaml")

	//getStruct

	var result []graphite
	v, err := c.DIY("metrics")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(v)
	err = mapstructure.Decode(v, &result)
	fmt.Println(result)

	var te []graphite
	c.GetStruct("metrics", &te)
	fmt.Println(te)
	//getSection

	m, err := c.GetSection("motan-registry")
	if err != nil {
		fmt.Println(err)
	}
	for key, info := range m {
		fmt.Printf("key:%+v, v:%+v\n", key, info)
	}

}

func Test_ReplacePlaceHolder(t *testing.T) {
	c, _ := NewConfigFromFile("./testconf.yaml")
	m := make(map[string]interface{})
	m["aaa"] = "testa"
	m["bbb"] = "testb"
	m["ccc"] = 2345 //test int
	c.ReplacePlaceHolder(m)
	s, _ := c.GetSection("testplaceholder")
	if s["aaa"] != "testa" {
		t.Errorf("value replace fail! aaa:%v\n", s["aaa"])
	}
	if s["ccc"] != 2345 {
		t.Errorf("value replace fail! ccc:%v\n", s["ccc"])
	}
	sub, _ := s["sub"].(map[interface{}]interface{})
	if sub["bbb"] != "testb" {
		t.Errorf("value replace fail! bbb:%v\n", sub["bbb"])
	}

}

func Test_Merge(t *testing.T) {
	c, _ := NewConfigFromFile("./testconf.yaml")

	newcfg := NewConfig()
	tm := make(map[interface{}]interface{})
	tm["port"] = 1234
	tm["registry"] = "replaced"

	tm2 := make(map[interface{}]interface{})
	tm2["mybasicRefer"] = tm
	newcfg.conf["motan-agent"] = tm
	newcfg.conf["motan-basicRefer"] = tm2

	a := make([]interface{}, 0, 16)
	a = append(a, "ss")
	a = append(a, "xxx")

	tm3 := make(map[interface{}]interface{})
	tm3["ddd"] = a

	newcfg.conf["testplaceholder"] = tm3

	c.Merge(newcfg)

	//fmt.Printf("%+v\n", c.conf["motan-basicRefer"])

	rm := c.conf["motan-agent"].(map[interface{}]interface{})
	if 1234 != rm["port"] {
		t.Errorf("value merge fail! result:%v\n", rm)
	}
	if "replaced" != rm["registry"] {
		t.Errorf("value merge fail! result:%v\n", rm)
	}

	rm = (c.conf["motan-basicRefer"].(map[interface{}]interface{}))["mybasicRefer"].(map[interface{}]interface{})
	if 1234 != rm["port"] {
		t.Errorf("value merge fail! result:%v\n", rm)
	}
	if "replaced" != rm["registry"] {
		t.Errorf("value merge fail! result:%v\n", rm)
	}

	tph := c.conf["testplaceholder"].(map[interface{}]interface{})
	dddSlice := tph["ddd"].([]interface{})
	if len(dddSlice) != 5 {
		t.Errorf("value merge fail! result:%v\n", dddSlice)
	}
}

func Test_multipleServiceGroupParse(t *testing.T) {
	data0 := map[interface{}]interface{}{
		"motan-service": map[interface{}]interface{}{
			"service1": map[interface{}]interface{}{
				"group": "",
			},
		},
	}
	data1 := map[interface{}]interface{}{
		"motan-service": map[interface{}]interface{}{
			"service1": map[interface{}]interface{}{
				"group": "hello",
			},
		},
	}
	data2 := map[interface{}]interface{}{
		"motan-service": map[interface{}]interface{}{
			"service1": map[interface{}]interface{}{
				"group": "hello,hello1,hello2",
			},
		},
	}
	multipleServiceGroupParse(map[interface{}]interface{}{})
	multipleServiceGroupParse(data0)
	assert.Len(t, data0["motan-service"], 1)
	multipleServiceGroupParse(data1)
	assert.Len(t, data1["motan-service"], 1)
	multipleServiceGroupParse(data2)
	assert.Len(t, data2["motan-service"], 3)
	assert.Equal(t, data2["motan-service"].(map[interface{}]interface{})["service1"].(map[interface{}]interface{})["group"], "hello")
	assert.Equal(t, data2["motan-service"].(map[interface{}]interface{})["service1-0"].(map[interface{}]interface{})["group"], "hello1")
	assert.Equal(t, data2["motan-service"].(map[interface{}]interface{})["service1-1"].(map[interface{}]interface{})["group"], "hello2")
}
