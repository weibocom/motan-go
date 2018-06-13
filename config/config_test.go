package config

import (
	"fmt"
	"testing"

	"github.com/mitchellh/mapstructure"
)

type graphite struct {
	Host string
	Port int
	Pool string
}

func Test_Config(t *testing.T) {
	//TODO
	c, _ := NewConfigFromFile("./testconf.yaml")

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
