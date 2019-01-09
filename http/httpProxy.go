package http

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	Proxy = "HTTP_PROXY"
)
const (
	DomainKey           = "domain"
	KeepaliveTimeoutKey = "keepaliveTimeout"
)
const (
	proxyMatchTypeUnknown = iota

	proxyMatchTypeRegexp
	proxyMatchTypeRegexpIgnoreCase
	proxyMatchTypeStart
	proxyMatchTypeExact
)

var (
	WhitespaceSplitPattern = regexp.MustCompile(`\s+`)
)

func PatternSplit(s string, pattern *regexp.Regexp) []string {
	matches := pattern.FindAllStringIndex(s, -1)
	strings := make([]string, 0, len(matches))
	beg := 0
	end := 0
	for _, match := range matches {
		end = match[0]
		if match[1] != 0 {
			strings = append(strings, s[beg:end])
		}
		beg = match[1]
	}

	if end != len(s) {
		strings = append(strings, s[beg:])
	}

	return strings
}

type URIConverter interface {
	URIToServiceName(uri string) string
}

type ProxyMatchType uint8

func (t ProxyMatchType) String() string {
	switch t {
	case proxyMatchTypeRegexp:
		return "regexp"
	case proxyMatchTypeRegexpIgnoreCase:
		return "iregexp"
	case proxyMatchTypeStart:
		return "start"
	case proxyMatchTypeExact:
		return "exact"
	default:
		return "unknown"
	}
}

func stringToProxyMatchType(s string) ProxyMatchType {
	switch s {
	case "regexp":
		return proxyMatchTypeRegexp
	case "iregexp":
		return proxyMatchTypeRegexpIgnoreCase
	case "start":
		return proxyMatchTypeStart
	case "exact":
		return proxyMatchTypeExact
	default:
		return proxyMatchTypeUnknown
	}
}

type ProxyLocation struct {
	Upstream     string   `yaml:"upstream"`
	Match        string   `yaml:"match"`
	Type         string   `yaml:"type"`
	RewriteRules []string `yaml:"rewriteRules"`

	pattern      *regexp.Regexp
	locationType ProxyMatchType
	rewriteRules []*rewriteRule
	length       int
}

// config like follows
// !regexp ^/2/.* ^/(.*) /2/$1
type rewriteRule struct {
	not         bool
	condType    ProxyMatchType
	condString  string
	condPattern *regexp.Regexp
	pattern     *regexp.Regexp
	replace     string
}

func newRewriteRule(rule string) (*rewriteRule, error) {
	args := PatternSplit(rule, WhitespaceSplitPattern)
	argc := len(args)
	if argc != 4 {
		return nil, fmt.Errorf("illegal argument number %d for rewrite rule", argc)
	}
	r := rewriteRule{}
	matchTypeString := args[0]
	if strings.HasPrefix(matchTypeString, "!") {
		r.not = true
		matchTypeString = matchTypeString[1:]
	}
	r.condType = stringToProxyMatchType(matchTypeString)
	if r.condType == proxyMatchTypeUnknown {
		return nil, errors.New("unsupported condition type " + args[0])
	}
	r.condString = args[1]
	if r.condType == proxyMatchTypeRegexp {
		pattern, err := regexp.Compile(args[1])
		if err != nil {
			return nil, err
		}
		r.condPattern = pattern
	} else if r.condType == proxyMatchTypeRegexpIgnoreCase {
		pattern, err := regexp.Compile("(?i)" + args[1])
		if err != nil {
			return nil, err
		}
		r.condPattern = pattern
	}
	pattern, err := regexp.Compile(args[2])
	if err != nil {
		return nil, err
	}
	r.pattern = pattern
	r.replace = args[3]
	return &r, nil
}

func (r *rewriteRule) rewrite(uri string) (string, bool) {
	ruleMatched := false
	switch r.condType {
	case proxyMatchTypeExact:
		ruleMatched = r.condString == uri
	case proxyMatchTypeStart:
		ruleMatched = strings.HasPrefix(uri, r.condString)
	case proxyMatchTypeRegexp, proxyMatchTypeRegexpIgnoreCase:
		ruleMatched = r.condPattern.MatchString(uri)
	}
	if r.not {
		ruleMatched = !ruleMatched
	}
	if ruleMatched {
		matchedIndex := r.pattern.FindStringSubmatchIndex(uri)
		if matchedIndex == nil {
			return uri, false
		}
		return string(r.pattern.ExpandString(nil, r.replace, uri, matchedIndex)), true
	}
	return uri, false
}

func (l *ProxyLocation) DeterminePath(path string, doRewrite bool) string {
	if !doRewrite {
		return path
	}
	for _, r := range l.rewriteRules {
		if s, b := r.rewrite(path); b {
			return s
		}
	}
	return path
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
		proxyLocation := ProxyLocation{}
		proxyLocation.Upstream = locationConfig["upstream"].(string)
		proxyLocation.Match = locationConfig["match"].(string)
		if t, ok := locationConfig["type"]; ok {
			proxyLocation.Type = t.(string)
		}
		if rs, ok := locationConfig["rewriteRules"]; ok {
			for _, r := range rs.([]interface{}) {
				proxyLocation.RewriteRules = append(proxyLocation.RewriteRules, r.(string))
			}
		}
		locations = append(locations, &proxyLocation)
	}
	return NewLocationMatcher(locations)
}

func NewLocationMatcher(locations []*ProxyLocation) *LocationMatcher {
	matcher := &LocationMatcher{
		locations: locations,
	}
	for _, l := range locations {
		l.length = len(l.Match)
		if len(l.RewriteRules) != 0 {
			rewriteRules := make([]*rewriteRule, 0, len(l.RewriteRules))
			for _, rule := range l.RewriteRules {
				if rule == "" {
					continue
				}
				r, err := newRewriteRule(rule)
				if err != nil {
					vlog.Errorf("Illegal rewrite rule %s for location %s: %s", rule, l.Match, err.Error())
					continue
				}
				rewriteRules = append(rewriteRules, r)
			}
			l.rewriteRules = rewriteRules
		}
		if l.Type == "" {
			// if not configured treat as a start rule by default
			l.Type = "start"
		}
		l.locationType = stringToProxyMatchType(l.Type)
		if l.locationType == proxyMatchTypeUnknown {
			vlog.Errorf("URL location unsupported type %s for location %s", l.Type, l.Match)
			continue
		}
		if l.locationType == proxyMatchTypeExact {
			matcher.exactLocations = append(matcher.exactLocations, l)
		}
		if l.locationType == proxyMatchTypeStart {
			matcher.startLocations = append(matcher.startLocations, l)
		}
		if l.locationType == proxyMatchTypeRegexp {
			pattern, err := regexp.Compile(l.Match)
			if err != nil {
				vlog.Errorf("Malformed regexp location %s", l.Match)
				continue
			}
			l.pattern = pattern
			matcher.regexpLocations = append(matcher.regexpLocations, l)
		}
		if l.locationType == proxyMatchTypeRegexpIgnoreCase {
			pattern, err := regexp.Compile("(?i)" + l.Match)
			if err != nil {
				vlog.Errorf("Malformed regexp location %s", l.Match)
				continue
			}
			l.pattern = pattern
			matcher.regexpLocations = append(matcher.regexpLocations, l)
		}
	}
	return matcher
}

// Pick returns the matched upstream and do url rewrite and etc
// Now this functions just compatible with nginx location match rules
// See http://nginx.org/en/docs/http/ngx_http_core_module.html#location
func (m *LocationMatcher) Pick(path string, doRewrite bool) (string, string, bool) {
	// First do exact location match
	for _, l := range m.exactLocations {
		if path == l.Match {
			return l.Upstream, l.DeterminePath(path, doRewrite), true
		}
	}
	// Second do regexp match by order
	for _, l := range m.regexpLocations {
		if l.pattern.MatchString(path) {
			return l.Upstream, l.DeterminePath(path, doRewrite), true
		}
	}
	// Last do start location match, use longest prefix match
	// TODO: nginx '^~' will disable regex match if longest prefix matched
	var longestLocation *ProxyLocation
	lastPrefixLen := 0
	for _, l := range m.startLocations {
		if strings.HasPrefix(path, l.Match) {
			if l.length > lastPrefixLen {
				longestLocation = l
				lastPrefixLen = l.length
			}
		}
	}
	if longestLocation != nil {
		return longestLocation.Upstream, longestLocation.DeterminePath(path, doRewrite), true
	}
	return "", "", false
}

func (m *LocationMatcher) URIToServiceName(uri string) string {
	if s, _, b := m.Pick(uri, false); b {
		return s
	}
	return ""
}
