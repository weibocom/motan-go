package http

import (
	"regexp"
	"strings"

	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	proxyLocationTypeUnknown = iota

	proxyLocationTypeRegexp
	proxyLocationTypeRegexpIgnoreCase
	proxyLocationTypeStart
	proxyLocationTypeExact
)

// ServiceDiscover 对于正向代理来说需要根据url确定使用哪个upstream
type ServiceDiscover interface {
	// DiscoverService  通过路径发现归属于哪个service(upstream), 如果能匹配出来则返回否则返回空字符串表示没有找到
	DiscoverService(uri string) string
}

// HTTPServiceExposer 对于反向代理来说需要根据ip获取自己在哪些域名的哪些upstream下
type ServiceExposer interface {
}

type ProxyLocationType uint8

func (t ProxyLocationType) String() string {
	switch t {
	case proxyLocationTypeRegexp:
		return "regexp"
	case proxyLocationTypeRegexpIgnoreCase:
		return "iregexp"
	case proxyLocationTypeStart:
		return "start"
	case proxyLocationTypeExact:
		return "exact"
	default:
		return "unknown"
	}
}

func stringToProxyLocationType(st string) ProxyLocationType {
	switch st {
	case "regexp":
		return proxyLocationTypeRegexp
	case "iregexp":
		return proxyLocationTypeRegexpIgnoreCase
	case "start":
		return proxyLocationTypeStart
	case "exact":
		return proxyLocationTypeExact
	default:
		return proxyLocationTypeUnknown
	}
}

type ProxyLocation struct {
	Upstream string `json:"upstream"`
	Match    string `json:"match"`
	Rewrite  string `json:"rewrite"`
	Type     string `json:"type"`

	reg          *regexp.Regexp
	locationType ProxyLocationType
}

func (l *ProxyLocation) DeterminePath(path string, doRewrite bool) string {
	if !doRewrite {
		return path
	}
	if l.Rewrite == "" {
		return path
	}
	// TODO: rewrite
	return path
}

func (l *ProxyLocation) Copy() *ProxyLocation {
	cl := &ProxyLocation{
		Upstream:     l.Upstream,
		Match:        l.Match,
		Rewrite:      l.Rewrite,
		Type:         l.Type,
		locationType: l.locationType,
	}
	if l.reg != nil {
		cl.reg = l.reg.Copy()
	}
	return cl
}

type LocationMatcher struct {
	locations       []*ProxyLocation
	exactLocations  []*ProxyLocation
	startLocations  []*ProxyLocation
	regexpLocations []*ProxyLocation
}

func NewLocationMatcherFromContext(domain string, context *core.Context) *LocationMatcher {
	section, err := context.Config.GetSection("http-locations")
	if err != nil || section == nil {
		return NewLocationMatcher(nil)
	}
	domainLocations := section[domain]
	if _, ok := domainLocations.([]interface{}); !ok {
		return NewLocationMatcher(nil)
	}
	domainLocationsSlice := domainLocations.([]interface{})
	locations := make([]*ProxyLocation, 0, len(domainLocationsSlice))
	for _, location := range domainLocationsSlice {
		locationConfig := location.(map[interface{}]interface{})
		pl := ProxyLocation{}
		pl.Upstream = locationConfig["upstream"].(string)
		pl.Match = locationConfig["match"].(string)
		if t, ok := locationConfig["type"]; ok {
			pl.Type = t.(string)
		}
		if r, ok := locationConfig["rewrite"]; ok {
			pl.Rewrite = r.(string)
		}
		locations = append(locations, &pl)
	}
	return NewLocationMatcher(locations)
}

func NewLocationMatcher(locations []*ProxyLocation) *LocationMatcher {
	matcher := &LocationMatcher{
		locations: locations,
	}
	for _, l := range locations {
		if l.Type == "" {
			// if no configured as a start rule by default
			l.Type = "start"
		}
		l.locationType = stringToProxyLocationType(l.Type)
		if l.locationType == proxyLocationTypeUnknown {
			vlog.Errorf("URL location unsupported type %s for location %s", l.Type, l.Match)
			continue
		}
		if l.locationType == proxyLocationTypeExact {
			matcher.exactLocations = append(matcher.exactLocations, l)
		}
		if l.locationType == proxyLocationTypeStart {
			matcher.startLocations = append(matcher.startLocations, l)
		}
		if l.locationType == proxyLocationTypeRegexp {
			reg, err := regexp.Compile(l.Match)
			if err != nil {
				vlog.Errorf("Malformed regexp location %s", l.Match)
			}
			l.reg = reg
			matcher.regexpLocations = append(matcher.regexpLocations, l)
		}
		if l.locationType == proxyLocationTypeRegexpIgnoreCase {
			reg, err := regexp.Compile("(?i)" + l.Match)
			if err != nil {
				vlog.Errorf("Malformed regexp location %s", l.Match)
			}
			l.reg = reg
			matcher.regexpLocations = append(matcher.regexpLocations, l)
		}
	}
	return matcher
}

// Pick returns the matched upstream and do url rewrite and etc
// Now this functions just compatible with nginx location match rules
func (m *LocationMatcher) Pick(path string, doRewrite bool) (string, string, bool) {
	// First do exact location match
	for _, l := range m.exactLocations {
		if path == l.Match {
			return l.Upstream, l.DeterminePath(path, doRewrite), true
		}
	}
	// Second do start location match
	for _, l := range m.startLocations {
		if strings.HasPrefix(path, l.Match) {
			return l.Upstream, l.DeterminePath(path, doRewrite), true
		}
	}
	// Last do regexp match by order
	for _, l := range m.regexpLocations {
		if l.reg.MatchString(path) {
			return l.Upstream, l.DeterminePath(path, doRewrite), true
		}
	}
	return "", "", false
}

func (m *LocationMatcher) DiscoverService(uri string) string {
	if s, _, b := m.Pick(uri, false); b {
		return s
	}
	return ""
}
