package core

import (
	"bytes"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/weibocom/motan-go/log"
)

type URL struct {
	Protocol   string            `json:"protocol"`
	Host       string            `json:"host"`
	Port       int               `json:"port"`
	Path       string            `json:"path"` //e.g. service name
	Group      string            `json:"group"`
	Parameters map[string]string `json:"parameters"`

	// cached info
	address  string
	identity string
}

var (
	defaultSerialize = "simple"
)

//TODO int param cache

// GetIdentity return the identity of url. identity info includes protocol, host, port, path, group
// the identity will cached, so must clear cached info after update above info by calling ClearCachedInfo()
func (u *URL) GetIdentity() string {
	if u.identity != "" {
		return u.identity
	}
	u.identity = u.Protocol + "://" + u.Host + ":" + u.GetPortStr() + "/" + u.Path + "?group=" + u.Group
	return u.identity
}

func (u *URL) ClearCachedInfo() {
	u.address = ""
	u.identity = ""
}

func (u *URL) GetPositiveIntValue(key string, defaultvalue int64) int64 {
	intvalue := u.GetIntValue(key, defaultvalue)
	if intvalue < 1 {
		return defaultvalue
	}
	return intvalue
}

func (u *URL) GetIntValue(key string, defaultValue int64) int64 {
	result, b := u.GetInt(key)
	if b {
		return result
	}
	return defaultValue
}

func (u *URL) GetBoolValue(key string, defaultValue bool) bool {
	if v, ok := u.Parameters[key]; ok {
		boolValue, err := strconv.ParseBool(v)
		if err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func (u *URL) GetInt(key string) (int64, bool) {
	if v, ok := u.Parameters[key]; ok {
		intValue, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return intValue, true
		}
	}
	return 0, false
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
	result, b := u.GetInt(mkey)
	if b {
		return result
	}
	result, b = u.GetInt(key)
	if b {
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
	defer func() { // if extinfo format not correct, just return nil URL
		if err := recover(); err != nil {
			vlog.Warningf("from ext to url fail. extinfo:%s, err:%v", extinfo, err)
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
	if u.address != "" {
		return u.address
	}
	u.address = u.Host + ":" + u.GetPortStr()
	return u.address
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
	if u.Protocol != other.Protocol && u.Protocol != ProtocolLocal {
		vlog.Errorf("can not serve protocol, err : p1:%s, p2:%s", u.Protocol, other.Protocol)
		return false
	}
	if u.Path != other.Path {
		vlog.Errorf("can not serve path, err : p1:%s, p2:%s", u.Path, other.Path)
		return false
	}
	if !IsSame(u.Parameters, other.Parameters, SerializationKey, "") {
		vlog.Errorf("can not serve serialization, err : s1:%s, s2:%s", u.Parameters[SerializationKey], other.Parameters[SerializationKey])
		return false
	}
	if !IsSame(u.Parameters, other.Parameters, VersionKey, "0.1") {
		vlog.Errorf("can not serve version, err : v1:%s, v2:%s", u.Parameters[VersionKey], other.Parameters[VersionKey])
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

func GetURLFilters(url *URL, extFactory ExtensionFactory) (clusterFilter ClusterFilter, endpointFilters []Filter) {
	if filters, ok := url.Parameters[FilterKey]; ok {
		clusterFilters := make([]Filter, 0, 10)
		endpointFilters = make([]Filter, 0, 10)
		arr := TrimSplit(filters, ",")
		for _, f := range arr {
			filter := extFactory.GetFilter(f)
			if filter != nil {
				if filter.GetType() == ClusterFilterType {
					// filter should use new instance
					if filter = filter.NewFilter(url); filter != nil {
						clusterFilters = append(clusterFilters, filter)
					}
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

func GetSerialization(url *URL, extFactory ExtensionFactory) Serialization {
	s := url.Parameters[SerializationKey]
	if s == "" {
		s = defaultSerialize
	}
	return extFactory.GetSerialization(s, -1)
}
