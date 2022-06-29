package http

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strings"

	"github.com/valyala/fasthttp"
	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const (
	Proxy       = "HTTP_PROXY"
	Method      = "HTTP_Method"
	QueryString = "HTTP_QueryString"
)

const (
	HeaderContentType = "Content-Type"
)

const (
	DomainKey                = "domain"
	KeepaliveTimeoutKey      = "keepaliveTimeout"
	IdleConnectionTimeoutKey = "idleConnectionTimeout"
	ProxyAddressKey          = "proxyAddress"
	ProxySchemaKey           = "proxySchema"
	MaxConnectionsKey        = "maxConnections"
	EnableRewriteKey         = "enableRewrite"
)

const (
	ProxyRequestIDKey = "requestIdFromClient"
)

const (
	proxyMatchTypeUnknown ProxyMatchType = iota
	proxyMatchTypeRegexp
	proxyMatchTypeRegexpIgnoreCase
	proxyMatchTypeStart
	proxyMatchTypeExact
)

const (
	proxyRewriteTypeUnknown ProxyRewriteType = iota
	proxyRewriteTypeRegexp
	proxyRewriteTypeRegexpIgnoreCase
	proxyRewriteTypeStart
	proxyRewriteTypeExact
	proxyRewriteTypeRegexpVar
)

var (
	WhitespaceSplitPattern        = regexp.MustCompile(`\s+`)
	findRewriteVarPattern         = regexp.MustCompile(`\{[0-9a-zA-Z_-]+\}`)
	httpProxySpecifiedAttachments = []string{Proxy, Method, QueryString}
	rewriteVarFunc                = func(condType ProxyRewriteType, uri string, queryBytes []byte) string {
		if condType != proxyRewriteTypeRegexpVar || len(queryBytes) == 0 {
			return uri
		}
		query, _ := url.ParseQuery(string(queryBytes))
		// replace path with query variables, variable holder format is : {foo}
		for _, varHolder := range findRewriteVarPattern.FindAllString(uri, -1) {
			varKey := varHolder[1 : len(varHolder)-1]
			uri = strings.Replace(uri, varHolder, query.Get(varKey), -1)
		}
		return uri
	}
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
	URIToServiceName(uri string, queryStringBytes []byte) string
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

type ProxyRewriteType uint8

func (t ProxyRewriteType) String() string {
	switch t {
	case proxyRewriteTypeRegexp:
		return "regexp"
	case proxyRewriteTypeRegexpIgnoreCase:
		return "iregexp"
	case proxyRewriteTypeStart:
		return "start"
	case proxyRewriteTypeExact:
		return "exact"
	case proxyRewriteTypeRegexpVar:
		return "regexpVar"
	default:
		return "unknown"
	}
}

func stringToProxyRewriteType(s string) ProxyRewriteType {
	switch s {
	case "regexp":
		return proxyRewriteTypeRegexp
	case "iregexp":
		return proxyRewriteTypeRegexpIgnoreCase
	case "start":
		return proxyRewriteTypeStart
	case "exact":
		return proxyRewriteTypeExact
	case "regexpVar":
		return proxyRewriteTypeRegexpVar
	default:
		return proxyRewriteTypeUnknown
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
	condType    ProxyRewriteType
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
	rewriteTypeString := args[0]
	if strings.HasPrefix(rewriteTypeString, "!") {
		r.not = true
		rewriteTypeString = rewriteTypeString[1:]
	}
	r.condType = stringToProxyRewriteType(rewriteTypeString)
	if r.condType == proxyRewriteTypeUnknown {
		return nil, errors.New("unsupported condition type " + args[0])
	}
	r.condString = args[1]
	if r.condType == proxyRewriteTypeRegexp || r.condType == proxyRewriteTypeRegexpVar {
		pattern, err := regexp.Compile(args[1])
		if err != nil {
			return nil, err
		}
		r.condPattern = pattern
	} else if r.condType == proxyRewriteTypeRegexpIgnoreCase {
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

func (r *rewriteRule) rewrite(uri string, query []byte) (string, bool) {
	ruleMatched := false
	switch r.condType {
	case proxyRewriteTypeExact:
		ruleMatched = r.condString == uri
	case proxyRewriteTypeStart:
		ruleMatched = strings.HasPrefix(uri, r.condString)
	case proxyRewriteTypeRegexp, proxyRewriteTypeRegexpIgnoreCase, proxyRewriteTypeRegexpVar:
		ruleMatched = r.condPattern.MatchString(uri)
	}
	if r.not {
		ruleMatched = !ruleMatched
	}
	if ruleMatched {
		matchedIndex := r.pattern.FindStringSubmatchIndex(uri)
		if matchedIndex == nil {
			return rewriteVarFunc(r.condType, uri, query), false
		}
		uri = rewriteVarFunc(r.condType, string(r.pattern.ExpandString(nil, r.replace, uri, matchedIndex)), query)
		return uri, true
	}
	return rewriteVarFunc(r.condType, uri, query), false
}

func (l *ProxyLocation) DeterminePath(path string, query []byte, doRewrite bool) string {
	if !doRewrite {
		return path
	}
	for _, r := range l.rewriteRules {
		if s, b := r.rewrite(path, query); b {
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
	needQueryString bool
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
				if r.condType == proxyRewriteTypeRegexpVar {
					matcher.needQueryString = true
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
func (m *LocationMatcher) Pick(path string, query []byte, doRewrite bool) (string, string, bool) {

	// First do exact location match
	for _, l := range m.exactLocations {
		if path == l.Match {
			return l.Upstream, l.DeterminePath(path, query, doRewrite), true
		}
	}
	// Second do regexp match by order
	for _, l := range m.regexpLocations {
		if l.pattern.MatchString(path) {
			return l.Upstream, l.DeterminePath(path, query, doRewrite), true
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
		return longestLocation.Upstream, longestLocation.DeterminePath(path, query, doRewrite), true
	}
	return "", "", false
}

func (m *LocationMatcher) URIToServiceName(uri string, queryString []byte) string {
	if s, _, b := m.Pick(uri, queryString, false); b {
		return s
	}
	return ""
}

func (m *LocationMatcher) NeedURLQueryString() bool {
	return m.needQueryString
}

// MotanRequestToFasthttpRequest convert a motan request to a fasthttp request
// For http mesh server side: rpc - motan2-> clientAgent - motan2 -> serverAgent - motan request convert to http request-> httpServer
// We use meta element HTTP_Method as http method, HTTP_QueryString as query string
// Request method as request uri
// Body will transform to a http body with following rules:
//  if body is a map[string]string we transform it as a form data
//  if body is a string or []byte just use it
//  else is unsupported
func MotanRequestToFasthttpRequest(motanRequest core.Request, fasthttpRequest *fasthttp.Request, defaultHTTPMethod string) error {
	httpMethod := motanRequest.GetAttachment(Method)
	if httpMethod == "" {
		httpMethod = defaultHTTPMethod
	}
	fasthttpRequest.Header.SetMethod(httpMethod)
	queryString := motanRequest.GetAttachment(QueryString)
	if queryString != "" {
		fasthttpRequest.URI().SetQueryString(queryString)
	}
	motanRequest.GetAttachments().Range(func(k, v string) bool {
		// ignore some specified key
		for _, attachmentKey := range httpProxySpecifiedAttachments {
			if strings.EqualFold(k, attachmentKey) {
				return true
			}
		}
		// fasthttp will use a special field to store this header
		if strings.EqualFold(k, core.HostKey) {
			fasthttpRequest.Header.SetHost(v)
			return true
		}
		if strings.EqualFold(k, HeaderContentType) {
			fasthttpRequest.Header.SetContentType(v)
			return true
		}
		k = strings.Replace(k, "M_", "MOTAN-", -1)
		fasthttpRequest.Header.Add(k, v)
		return true
	})
	fasthttpRequest.Header.Del("Connection")
	arguments := motanRequest.GetArguments()
	if len(arguments) > 1 {
		return errors.New("http rpc only support one parameter")
	}
	if len(arguments) == 1 {
		var buffer bytes.Buffer
		arg0 := arguments[0]
		if arg0 != nil {
			switch arg0.(type) {
			case map[string]string:
				if httpMethod == "GET" {
					queryArgs := fasthttpRequest.URI().QueryArgs()
					for k, v := range arg0.(map[string]string) {
						// no need escape, fasthttp will do escape atomic
						queryArgs.Add(k, v)
					}
				} else {
					for k, v := range arg0.(map[string]string) {
						buffer.WriteString("&")
						buffer.WriteString(k)
						buffer.WriteString("=")
						buffer.WriteString(url.QueryEscape(v))
					}
					if buffer.Len() != 0 {
						// the first character is '&', we need remove it
						fasthttpRequest.Header.SetContentType("application/x-www-form-urlencoded")
						fasthttpRequest.SetBody(buffer.Bytes()[1:])
					}
				}
			case string:
				fasthttpRequest.SetBody([]byte(arg0.(string)))
			case []byte:
				fasthttpRequest.SetBody(arg0.([]byte))
			default:
				return errors.New("http rpc unsupported parameter type: " + reflect.TypeOf(arg0).String())
			}
		}
	}
	return nil
}

// FasthttpResponseToMotanResponse convert a http response to a motan response
// For http mesh server side, the httpServer response to the server agent but client need a motan response
// Contrast to request convert, we put all headers to meta, an body maybe just use it with type []byte
func FasthttpResponseToMotanResponse(motanResponse core.Response, fasthttpResponse *fasthttp.Response) {
	fasthttpResponse.Header.VisitAll(func(k, v []byte) {
		motanResponse.SetAttachment(string(k), string(v))
	})
	if resp, ok := motanResponse.(*core.MotanResponse); ok {
		httpResponseBody := fasthttpResponse.Body()
		if httpResponseBody != nil {
			motanResponseBody := make([]byte, len(httpResponseBody))
			copy(motanResponseBody, httpResponseBody)
			resp.Value = motanResponseBody
		}
	}
}
