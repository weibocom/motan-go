package config

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v2"
)

type Config struct {
	conf         map[interface{}]interface{}
	placeHolders map[string]interface{}
	rex          *regexp.Regexp
}

// NewConfigFromReader parse config from a reader
func NewConfigFromReader(r io.Reader) (*Config, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, errors.New("read config data failed: " + err.Error())
	}
	m := make(map[interface{}]interface{})
	err = yaml.Unmarshal([]byte(data), &m)
	if err != nil {
		fmt.Printf("config unmarshal failed. " + err.Error())
		return nil, errors.New("config unmarshal failed: " + err.Error())
	}
	return &Config{conf: m}, nil
}

// NewConfigFromFile parse config from file.
func NewConfigFromFile(path string) (*Config, error) {
	filename, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.New("can not find the file :" + filename)
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, errors.New("read config file fail. " + err.Error())
	}
	defer f.Close()
	fmt.Printf("start parse config path:%s \n", path)
	return NewConfigFromReader(f)
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
	return c.GetStringWithDefault(key, "")
}

// String returns the string value for a given key.
func (c *Config) GetStringWithDefault(key string, def string) string {
	if value, err := c.getData(key); err != nil {
		return def
	} else if vv, ok := value.(string); ok {
		return vv
	}
	return def
}

// GetSection returns map for the given key
func (c *Config) GetSection(key string) (map[interface{}]interface{}, error) {
	if v, err := c.getData(key); err != nil {
		return nil, err
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
		c.replace(&c.conf)
	}
}

// one key must be single palceholder! the key will be replaced entirely.
func (c *Config) replace(configs *map[interface{}]interface{}) {
	for k, v := range *configs {
		if sv, ok := v.(string); ok {
			nv := c.getPlaceholderValue(sv)
			if nv != nil {
				(*configs)[k] = nv
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
func (c *Config) GetOriginMap() map[interface{}]interface{} {
	return c.conf
}

func (c *Config) Merge(config *Config) {
	if config != nil {
		mergeMap(c.conf, config.conf)
	}
}

// target & origin should not nil
func mergeMap(target map[interface{}]interface{}, origin map[interface{}]interface{}) {
	for originKey, originValue := range origin {
		if originValue != nil {
			if targetValue, ok := target[originKey]; !ok || targetValue == nil {
				target[originKey] = originValue
			} else {
				targetValueType := reflect.TypeOf(targetValue)
				originValueType := reflect.TypeOf(originValue)
				if targetValueType.Kind() == reflect.Map && originValueType.Kind() == reflect.Map {
					targetValueMap, ok1 := targetValue.(map[interface{}]interface{})
					originValueMap, ok2 := originValue.(map[interface{}]interface{})
					if ok1 && ok2 {
						mergeMap(targetValueMap, originValueMap)
					}
				} else if targetValueType.Kind() == reflect.Slice && originValueType.Kind() == reflect.Slice {
					targetValueSlice, ok1 := targetValue.([]interface{})
					originValueSlice, ok2 := originValue.([]interface{})
					if ok1 && ok2 {
						target[originKey] = append(targetValueSlice, originValueSlice...)
					}
				} else {
					target[originKey] = originValue
				}
			}
		}
	}
}

func NewConfig() *Config {
	cfg := &Config{conf: make(map[interface{}]interface{}, 16), placeHolders: make(map[string]interface{}, 16)}
	return cfg
}
