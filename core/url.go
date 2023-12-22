package core

import (
	"bytes"
	"github.com/weibocom/motan-go/log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type URL struct {
	Protocol   string            `json:"protocol"`
	Host       string            `json:"host"`
	Port       int               `json:"port"`
	Path       string            `json:"path"` //e.g. service name
	Group      string            `json:"group"`
	Parameters map[string]string `json:"parameters"`

	// cached info
	address              atomic.Value
	portStr              atomic.Value
	identity             atomic.Value
	hasMethodParamsCache atomic.Value // Whether it has method parameters
	intParamCache        sync.Map
}

type int64Cache struct {
	value  int64
	isMiss bool // miss cache if true
}

var (
	defaultSerialize          = "simple"
	defaultMethodParamsSubStr = ")."
	defaultMissCache          = &int64Cache{value: 0, isMiss: true} // Uniform miss cache
)

// GetIdentity return the identity of url. identity info includes protocol, host, port, path, group
// the identity will be cached, so must clear cached info after update above info by calling ClearCachedInfo()
func (u *URL) GetIdentity() string {
	temp := u.identity.Load()
	if temp != nil && temp != "" {
		return temp.(string)
	}
	idt := u.Protocol + "://" + u.Host + ":" + u.GetPortStr() + "/" + u.Path + "?group=" + u.Group
	if u.Protocol == "direct" {
		idt += "&address=" + u.GetParam("address", "")
	}
	u.identity.Store(idt)
	return idt
}

// IsMatch is a tool function for comparing parameters: service, group, protocol and version
// with URL. When 'protocol' or 'version' is empty, it will be ignored
func (u *URL) IsMatch(service, group, protocol, version string) bool {
	if u.Path != service {
		return false
	}
	if group != "" && u.Group != group {
		return false
	}
	// for motan v1 request, parameter protocol should be empty
	if protocol != "" {
		if u.Protocol == "motanV1Compatible" {
			if protocol != "motan2" && protocol != "motan" {
				return false
			}
		} else {
			if u.Protocol != protocol {
				return false
			}
		}
	}
	if version != "" && u.GetParam(VersionKey, "") != "" {
		return version == u.GetParam(VersionKey, "")
	}
	return true
}

func (u *URL) GetIdentityWithRegistry() string {
	id := u.GetIdentity()
	registryId := u.GetParam(RegistryKey, "")
	return id + "&registry=" + registryId
}

func (u *URL) ClearCachedInfo() {
	u.address.Store("")
	u.identity.Store("")
	u.portStr.Store("")
	u.hasMethodParamsCache.Store("")
	u.intParamCache.Range(func(key interface{}, value interface{}) bool {
		u.intParamCache.Delete(key)
		return true
	})
}

func (u *URL) GetPositiveIntValue(key string, defaultValue int64) int64 {
	intValue := u.GetIntValue(key, defaultValue)
	if intValue < 1 {
		return defaultValue
	}
	return intValue
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

func (u *URL) GetIntValue(key string, defaultValue int64) int64 {
	result, b := u.GetInt(key)
	if b {
		return result
	}
	return defaultValue
}

func (u *URL) GetInt(key string) (int64, bool) {
	if cache, ok := u.intParamCache.Load(key); ok {
		if c, ok := cache.(*int64Cache); ok { // from cache
			if c.isMiss {
				return 0, false
			}
			return c.value, true
		}
	}

	if v, ok := u.Parameters[key]; ok {
		intValue, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			u.intParamCache.Store(key, &int64Cache{value: intValue, isMiss: false})
			return intValue, true
		}
	}
	u.intParamCache.Store(key, defaultMissCache) // set miss cache
	return 0, false
}

func (u *URL) GetStringParamsWithDefault(key string, defaultValue string) string {
	var ret string
	if u.Parameters != nil {
		ret = u.Parameters[key]
	}
	if ret == "" {
		ret = defaultValue
	}
	return ret
}

func (u *URL) GetMethodIntValue(method string, methodDesc string, key string, defaultValue int64) int64 {
	if u.hasMethodParams() {
		mk := method + "(" + methodDesc + ")." + key
		result, b := u.GetInt(mk)
		if b {
			return result
		}
	}
	result, b := u.GetInt(key)
	if b {
		return result
	}
	return defaultValue
}

func (u *URL) hasMethodParams() bool {
	v := u.hasMethodParamsCache.Load()
	if v == nil || v == "" { // Check if method parameters exist
		if u.Parameters != nil {
			for k := range u.Parameters {
				if strings.Contains(k, defaultMethodParamsSubStr) {
					v = "t"
					u.hasMethodParamsCache.Store("t")
					break
				}
			}
		}
		v = "f"
		u.hasMethodParamsCache.Store("f")
	}
	return v == "t"
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
	u.intParamCache.Delete(key)                           // remove cache
	if strings.Contains(key, defaultMethodParamsSubStr) { // Check if method parameter
		u.hasMethodParamsCache.Store("t")
	}
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

	var keys []string
	for k := range u.Parameters {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		buf.WriteString("&")
		buf.WriteString(k)
		buf.WriteString("=")
		buf.WriteString(u.Parameters[k])
	}
	return buf.String()

}

func FromExtInfo(extInfo string) *URL {
	defer func() { // if extInfo format not correct, just return nil URL
		if err := recover(); err != nil {
			vlog.Warningf("from ext to url fail. extInfo:%s, err:%v", extInfo, err)
		}
	}()
	arr := strings.Split(extInfo, "?")
	nodeInfos := strings.Split(arr[0], "://")
	protocol := nodeInfos[0]
	nodeInfos = strings.Split(nodeInfos[1], "/")
	path := nodeInfos[1]
	nodeInfos = strings.Split(nodeInfos[0], ":")
	host := nodeInfos[0]
	port, _ := strconv.ParseInt(nodeInfos[1], 10, 64)

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
	temp := u.portStr.Load()
	if temp != nil && temp != "" {
		return temp.(string)
	}
	p := strconv.FormatInt(int64(u.Port), 10)
	u.portStr.Store(p)
	return p
}

func (u *URL) GetAddressStr() string {
	temp := u.address.Load()
	if temp != nil && temp != "" {
		return temp.(string)
	}
	if strings.HasPrefix(u.Host, UnixSockProtocolFlag) {
		temp = u.Host
	} else {
		temp = u.Host + ":" + u.GetPortStr()
	}
	u.address.Store(temp)
	return temp.(string)
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
	// compatible with old version: 0.1
	if !(IsSame(u.Parameters, other.Parameters, VersionKey, "0.1") || IsSame(u.Parameters, other.Parameters, VersionKey, DefaultReferVersion)) {
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
