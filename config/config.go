package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/mitchellh/mapstructure"
	"github.com/weibocom/motan-go/log"
	"gopkg.in/yaml.v2"
	"reflect"
	"regexp"
)

type Config struct {
	conf         map[string]interface{}
	placeHolders map[string]interface{}
	rex          *regexp.Regexp
}

// NewConfigFromFile parse config from file.
func NewConfigFromFile(path string) (*Config, error) {
	fmt.Printf("start parse config path:%s \n", path)
	filename, err := filepath.Abs(path)
	if err != nil {
		fmt.Printf("can not find the file:%s\n", filename)
		return nil, errors.New("can not find the file :" + filename)
	}
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Printf("read config file fail. file: %s, error: %s\n", filename, err.Error())
		return nil, errors.New("read config file fail. " + err.Error())
	}
	m := make(map[string]interface{})
	err = yaml.Unmarshal([]byte(data), &m)
	if err != nil {
		fmt.Printf("config unmarshal failed! file: %s, error: %s\n", filename, err)
		return nil, errors.New("config unmarshal failed. " + err.Error())
	}
	return &Config{conf: m}, nil
}

// ParseBool accepts 1, 1.0, t, T, TRUE, true, True, YES, yes, Yes,Y, y, ON, on, On,
// 0, 0.0, f, F, FALSE, false, False, NO, no, No, N,n, OFF, off, Off.
// Any other value returns an error.
func ParseBool(val interface{}) (value bool, err error) {
	if val != nil {
		switch v := val.(type) {
		case bool:
			return v, nil
		case string:
			switch v {
			case "1", "t", "T", "true", "TRUE", "True", "YES", "yes", "Yes", "Y", "y", "ON", "on", "On":
				return true, nil
			case "0", "f", "F", "false", "FALSE", "False", "NO", "no", "No", "N", "n", "OFF", "off", "Off":
				return false, nil
			}
		case int8, int32, int64:
			strV := fmt.Sprintf("%s", v)
			if strV == "1" {
				return true, nil
			} else if strV == "0" {
				return false, nil
			}
		case float64:
			if v == 1 {
				return true, nil
			} else if v == 0 {
				return false, nil
			}
		}
		return false, fmt.Errorf("parsing %q: invalid syntax", val)
	}
	return false, fmt.Errorf("parsing <nil>: invalid syntax")
}

// Bool returns the boolean value for a given key.
func (c *Config) Bool(key string) (bool, error) {
	v, err := c.getData(key)
	if err != nil {
		return false, err
	}
	return ParseBool(v)
}

// Int returns the integer value for a given key.
func (c *Config) Int(key string) (int, error) {
	if v, err := c.getData(key); err != nil {
		return 0, err
	} else if vv, ok := v.(int); ok {
		return vv, nil
	} else if vv, ok := v.(int64); ok {
		return int(vv), nil
	}
	return 0, errors.New("not int value")
}

// DefaultInt returns the integer value for a given key.
// if err != nil return defaltval
func (c *Config) DefaultInt(key string, defaultval int) int {
	v, err := c.Int(key)
	if err != nil {
		return defaultval
	}
	return v
}

// Int64 returns the int64 value for a given key.
func (c *Config) Int64(key string) (int64, error) {
	if v, err := c.getData(key); err != nil {
		return 0, err
	} else if vv, ok := v.(int64); ok {
		return vv, nil
	}
	return 0, errors.New("not bool value")
}

// String returns the string value for a given key.
func (c *Config) String(key string) string {
	if value, err := c.getData(key); err != nil {
		return ""
	} else if vv, ok := value.(string); ok {
		return vv
	}
	return ""
}

// GetSection returns map for the given key
func (c *Config) GetSection(key string) (map[interface{}]interface{}, error) {
	if v, err := c.getData(key); err != nil {
		return nil, errors.New("not exist setction")
	} else if vv, ok := v.(map[interface{}]interface{}); ok {
		return vv, nil
	}
	return nil, errors.New("section not map[interface{}]interface{}")
}

// GetStruct returns struct for the give key and given struct type
func (c *Config) GetStruct(key string, out interface{}) error {
	v, err := c.getData(key)
	if err != nil {
		return err
	}
	err = mapstructure.Decode(v, out)
	if err != nil {
		return err
	}
	return nil
}

// DIY returns the raw value by a given key.
func (c *Config) DIY(key string) (v interface{}, err error) {
	return c.getData(key)
}

func (c *Config) getData(key string) (interface{}, error) {

	if len(key) == 0 {
		return nil, errors.New("key is empty")
	}

	if v, ok := c.conf[key]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("not exist key %q", key)
}

// ReplacePlaceHolder will replace all palceholders like '${key}', replace 'key' to 'value' according to the properties map.
func (c *Config) ReplacePlaceHolder(placeHolders map[string]interface{}) {
	if len(placeHolders) > 0 {
		c.placeHolders = placeHolders
		fmt.Printf("dynamic configs:%+v\n", placeHolders)
		for k, v := range c.conf {
			if sv, ok := v.(string); ok {
				nv := c.getPlaceholderValue(sv)
				if nv != nil {
					c.conf[k] = nv
					vlog.Infof("config value '%s' is replaced by '%v'\n", sv, nv)
					fmt.Printf("config value '%s' is replaced by '%v'\n", sv, nv)
				}
			} else if mv, ok := v.(map[interface{}]interface{}); ok {
				c.replace(&mv)
			}
		}
	}
}

// one key must be single palceholder! the key will be replaced entirely.
func (c *Config) replace(configs *map[interface{}]interface{}) {
	for k, v := range *configs {
		if sv, ok := v.(string); ok {
			nv := c.getPlaceholderValue(sv)
			if nv != nil {
				(*configs)[k] = nv
				vlog.Infof("config value '%s' is replaced by '%v'\n", sv, nv)
				fmt.Printf("config value '%s' is replaced by '%v'\n", sv, nv)
			}
		} else if mv, ok := v.(map[interface{}]interface{}); ok {
			c.replace(&mv)
		}
	}
}

func (c *Config) getPlaceholderValue(v string) interface{} {
	var nv interface{}
	if c.rex == nil {
		c.rex, _ = regexp.Compile("^\\${([^}]+)}$")
	}
	result := c.rex.FindStringSubmatch(v)
	if len(result) == 2 {
		nv = c.placeHolders[result[1]]
	}
	return nv
}

// GetOriginMap : get origin configs map
func (c *Config) GetOriginMap() map[string]interface{} {
	return c.conf
}

func (c *Config) Merge(config *Config) {
	if config != nil {
		for k, v := range config.conf {
			if v != nil {
				if tv, ok := c.conf[k]; !ok || tv == nil {
					c.conf[k] = v
				} else {
					tp := reflect.TypeOf(tv)
					vtp := reflect.TypeOf(v)
					if tp.Kind() == reflect.Map && vtp.Kind() == reflect.Map {
						tvm, ok1 := tv.(map[interface{}]interface{})
						vm, ok2 := v.(map[interface{}]interface{})
						if ok1 && ok2 {
							mergeMap(tvm, vm)
						}
					} else if tp.Kind() == reflect.Slice && vtp.Kind() == reflect.Slice {
						tvs, ok1 := tv.([]interface{})
						vs, ok2 := v.([]interface{})
						if ok1 && ok2 {
							mergeArray(&tvs, vs)
						}
					} else {
						c.conf[k] = v
					}
				}
			}
		}
	}
}

// t & o should not nil
func mergeMap(t map[interface{}]interface{}, o map[interface{}]interface{}) {
	for k, v := range o {
		if v != nil {
			if tv, ok := t[k]; !ok || tv == nil {
				t[k] = v
			} else {
				tp := reflect.TypeOf(tv)
				vtp := reflect.TypeOf(v)
				if tp.Kind() == reflect.Map && vtp.Kind() == reflect.Map {
					tvm, ok1 := tv.(map[interface{}]interface{})
					vm, ok2 := v.(map[interface{}]interface{})
					if ok1 && ok2 {
						mergeMap(tvm, vm)
					}
				} else if tp.Kind() == reflect.Slice && vtp.Kind() == reflect.Slice {
					tvs, ok1 := tv.([]interface{})
					vs, ok2 := v.([]interface{})
					if ok1 && ok2 {
						mergeArray(&tvs, vs)
					}
				} else {
					t[k] = v
				}
			}
		}
	}
}

// only append in slice type
func mergeArray(t *[]interface{}, o []interface{}) {
	for _, v := range o {
		if v != nil {
			*t = append(*t, v)
		}
	}
}

func NewConfig() *Config {
	cfg := &Config{conf: make(map[string]interface{}, 16), placeHolders: make(map[string]interface{}, 16)}
	return cfg
}
