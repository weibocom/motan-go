package core

import (
	"bytes"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/weibocom/motan-go/log"
)

type URL struct {
	Protocol   string
	Host       string
	Port       int
	Path       string //e.g. service name
	Group      string
	Parameters map[string]string
}

var (
	defaultSerialize = "simple"
)

func (u *URL) GetIdentity() string {
	return u.Protocol + "://" + u.Host + ":" + u.GetPortStr() + "/" + u.Path + "?group=" + u.Group
}

func (u *URL) GetPositiveIntValue(key string, defaultvalue int64) int64 {
	intvalue := u.GetIntValue(key, defaultvalue)
	if intvalue < 1 {
		return defaultvalue
	}
	return intvalue
}

func (u *URL) GetIntValue(key string, defaultValue int64) int64 {
	result, err := u.GetInt(key)
	if err == nil {
		return result
	}
	return defaultValue
}

func (u *URL) GetInt(key string) (i int64, err error) {

	if v, ok := u.Parameters[key]; ok {
		intvalue, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return intvalue, nil
		}
	} else {
		err = errors.New("map key not exist")
	}

	return 0, err
}

func (u *URL) GetStringParamsWithDefault(key string, defaultvalue string) string {
	var ret string
	if u.Parameters != nil {
		ret = u.Parameters[key]
	}
	if ret == "" {
		ret = defaultvalue
	}
	return ret
}

func (u *URL) GetMethodIntValue(method string, methodDesc string, key string, defaultValue int64) int64 {
	mkey := method + "(" + methodDesc + ")." + key
	result, err := u.GetInt(mkey)
	if err == nil {
		return result
	}
	result, err = u.GetInt(key)
	if err == nil {
		return result
	}
	return defaultValue
}

func (u *URL) GetMethodPositiveIntValue(method string, methodDesc string, key string, defaultValue int64) int64 {
	result := u.GetMethodIntValue(method, methodDesc, key, defaultValue)
	if result > 0 {
		return result
	}
	return defaultValue
}

func (u *URL) GetParam(key string, defaultValue string) string {
	if u.Parameters == nil || len(u.Parameters) == 0 {
		return defaultValue
	}
	ret := u.Parameters[key]
	if ret == "" {
		return defaultValue
	}
	return ret
}

// GetTimeDuration get time duration from params.
func (u *URL) GetTimeDuration(key string, unit time.Duration, defaultDuration time.Duration) time.Duration {
	if t, ok := u.Parameters[key]; ok {
		if ti, err := strconv.ParseInt(t, 10, 32); err == nil {
			return time.Duration(ti) * unit
		}
	}
	return defaultDuration
}

func (u *URL) PutParam(key string, value string) {
	if u.Parameters == nil {
		u.Parameters = make(map[string]string)
	}
	u.Parameters[key] = value
}

func (u *URL) ToExtInfo() string {
	var buf bytes.Buffer
	buf.WriteString(u.Protocol)
	buf.WriteString("://")
	buf.WriteString(u.Host)
	buf.WriteString(":")
	buf.WriteString(u.GetPortStr())
	buf.WriteString("/")
	buf.WriteString(u.Path)
	buf.WriteString("?")
	buf.WriteString("group=")
	buf.WriteString(u.Group)

	for k, v := range u.Parameters {
		buf.WriteString("&")
		buf.WriteString(k)
		buf.WriteString("=")
		buf.WriteString(v)
	}
	return buf.String()

}

func FromExtInfo(extinfo string) *URL {
	defer func() {
		if err := recover(); err != nil {
			vlog.Errorf("parse url from extinfo fail! err: %v, extinfo: %s", err, extinfo)
		}
	}()
	arr := strings.Split(extinfo, "?")
	nodeinfos := strings.Split(arr[0], "://")
	protocol := nodeinfos[0]
	nodeinfos = strings.Split(nodeinfos[1], "/")
	path := nodeinfos[1]
	nodeinfos = strings.Split(nodeinfos[0], ":")
	host := nodeinfos[0]
	port, _ := strconv.ParseInt(nodeinfos[1], 10, 64)

	paramsMap := make(map[string]string)
	params := strings.Split(arr[1], "&")
	for _, p := range params {
		kv := strings.Split(p, "=")
		if len(kv) != 2 {
			continue
		}
		paramsMap[kv[0]] = kv[1]
	}
	group := paramsMap["group"]
	delete(paramsMap, "group")
	url := &URL{Protocol: protocol, Host: host, Port: int(port), Path: path, Group: group, Parameters: paramsMap}
	return url
}

func (u *URL) GetPortStr() string {
	return strconv.FormatInt(int64(u.Port), 10)
}

func (u *URL) GetAddressStr() string {
	return u.Host + ":" + u.GetPortStr()
}

func (u *URL) Copy() *URL {
	newURL := &URL{Protocol: u.Protocol, Host: u.Host, Port: u.Port, Group: u.Group, Path: u.Path}
	newParams := make(map[string]string)
	for k, v := range u.Parameters {
		newParams[k] = v
	}
	newURL.Parameters = newParams
	return newURL
}

func (u *URL) MergeParams(params map[string]string) {
	for k, v := range params {
		u.Parameters[k] = v
	}
}

func (u *URL) CanServe(other *URL) bool {
	if u.Protocol != other.Protocol {
		vlog.Errorf("can not serve protocol, err : p1:%s, p2:%s\n", u.Protocol, other.Protocol)
		return false
	}
	if u.Path != other.Path {
		vlog.Errorf("can not serve path, err : p1:%s, p2:%s\n", u.Path, other.Path)
		return false
	}
	if !IsSame(u.Parameters, other.Parameters, SerializationKey, "") {
		vlog.Errorf("can not serve serialization, err : s1:%s, s2:%s\n", u.Parameters[SerializationKey], other.Parameters[SerializationKey])
		return false
	}
	if !IsSame(u.Parameters, other.Parameters, VersionKey, "0.1") {
		vlog.Errorf("can not serve version, err : v1:%s, v2:%s\n", u.Parameters[VersionKey], other.Parameters[VersionKey])
		return false
	}
	return true
}

func IsSame(m1 map[string]string, m2 map[string]string, key string, defaultValue string) bool {
	if m1 == nil && m2 == nil {
		return true
	}
	v1 := defaultValue
	v2 := defaultValue
	if m1 != nil {
		if _, ok := m1[key]; ok {
			v1 = m1[key]
		}
	}
	if m2 != nil {
		if _, ok := m2[key]; ok {
			v2 = m2[key]
		}
	}
	return v1 == v2
}

func GetURLFilters(url *URL, extFactory ExtentionFactory) (clusterFilter ClusterFilter, endpointFilters []Filter) {
	if filters, ok := url.Parameters[FilterKey]; ok {
		clusterFilters := make([]Filter, 0, 10)
		endpointFilters = make([]Filter, 0, 10)
		arr := strings.Split(filters, ",")
		for _, f := range arr {
			filter := extFactory.GetFilter(f)
			if filter != nil {
				if filter.GetType() == ClusterFilterType {
					// filter should use new instance
					filter = filter.NewFilter(url)
					clusterFilters = append(clusterFilters, filter)
				} else {
					endpointFilters = append(endpointFilters, filter)
				}
				Initialize(filter)
			}
		}
		if len(clusterFilters) > 0 {
			sort.Sort(filterSlice(clusterFilters))
			var lastFilter ClusterFilter
			lastFilter = GetLastClusterFilter()

			//index小的filter在外层，大的在内层靠近endpoint
			for _, cf := range clusterFilters {
				(cf.(ClusterFilter)).SetNext(lastFilter)
				lastFilter = cf.(ClusterFilter)
			}
			clusterFilter = lastFilter
		}
		if len(endpointFilters) > 0 {
			sort.Sort(filterSlice(endpointFilters))
		}

	}
	return clusterFilter, endpointFilters
}

type filterSlice []Filter

func (f filterSlice) Len() int {
	return len(f)
}
func (f filterSlice) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}
func (f filterSlice) Less(i, j int) bool {
	// desc
	return f[i].GetIndex() > f[j].GetIndex()
}

func GetSerialization(url *URL, extFactory ExtentionFactory) Serialization {
	s := url.Parameters[SerializationKey]
	if s == "" {
		s = defaultSerialize
	}
	return extFactory.GetSerialization(s, -1)
}
