package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/mitchellh/mapstructure"
	"github.com/weibocom/motan-go/log"
	"gopkg.in/yaml.v2"
)

type Config struct {
	conf *map[string]interface{}
}

// NewConfigFromFile parse config from file.
func NewConfigFromFile(path string) (*Config, error) {
	vlog.Infof("start parse config path:%s \n", path)
	filename, err := filepath.Abs(path)
	if err != nil {
		vlog.Errorf("can not find the file:%s\n", filename)
	}
	data, err := ioutil.ReadFile(filename)
	m := make(map[string]interface{})

	if err != nil {
		vlog.Errorf("config init failed! file: %s, error: %s\n", filename, err.Error())
	}
	err = yaml.Unmarshal([]byte(data), &m)
	if err != nil {
		vlog.Errorf("config unmarshal failed! file: %s, error: %s\n", filename, err)
	}
	return &Config{&m}, nil
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

	if v, ok := (*c.conf)[key]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("not exist key %q", key)
}
