package http

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/weibocom/motan-go/config"
	"github.com/weibocom/motan-go/core"
	"testing"
)

type APITestSuite struct {
	suite.Suite
}

func TestAPITestSuite(t *testing.T) {
	suite.Run(t, new(APITestSuite))
}

func (s *APITestSuite) TestNewLocationMatcher() {
	const locationTestData = `
http-locations:
  test.domain:
  - match: /
    type: start
    upstream: test1
    rewriteRules:
    - 'exact /Test2/1 /(.*) /test'

  - match: ^/test2/.*
    type: regexp
    upstream: test2
    rewriteRules:
    - '!iregexp ^/Test2/1/.* ^/test2/(.*) /test/$1'

  - match: ^/test3/.*
    type: iregexp
    upstream: test3
    rewriteRules:
    - 'start / ^/(.*) /test/$1'

  - match: ^(/|/2/)(p1|p2).*
    type: regexp
    upstream: test4
    rewriteRules:
    - 'start / ^/(p1|p2)/(.*) /2/$1/$2'

  - match: /test/mytest
    type: exact
    upstream: test5

  - match: /test/mytest
    type: exact
    upstream: test5
    rewriteRules:
    - 'regexp / ^/(p1|p2)/(.*) /2/$1/$2'

  - match: ^/p3.*
    type: regexp
    upstream: test6
    rewriteRules:
    - 'regexpVar / ^/p3/(.*) /p3/{a}/$1'
`

	context := &core.Context{}
	context.Config, _ = config.NewConfigFromReader(bytes.NewReader([]byte(locationTestData)))
	matcher := NewLocationMatcherFromContext("test.domain", context)
	service := ""
	rewritePath := ""
	service, rewritePath, _ = matcher.Pick("/Test3/1", nil, true)
	assert.Equal(s.T(), "test3", service)
	assert.Equal(s.T(), "/test/Test3/1", rewritePath)
	service, rewritePath, _ = matcher.Pick("/test3/1", nil, true)
	assert.Equal(s.T(), "test3", service)
	service, rewritePath, _ = matcher.Pick("/test2/1/1", nil, true)
	assert.Equal(s.T(), "test2", service)
	assert.Equal(s.T(), "/test2/1/1", rewritePath)
	service, rewritePath, _ = matcher.Pick("/test2/2/1", nil, true)
	assert.Equal(s.T(), "test2", service)
	assert.Equal(s.T(), "/test/2/1", rewritePath)
	service, rewritePath, _ = matcher.Pick("/Test2/1", nil, true)
	assert.Equal(s.T(), "test1", service)
	assert.Equal(s.T(), "/test", rewritePath)
	service, rewritePath, _ = matcher.Pick("/p1/test", nil, true)
	assert.Equal(s.T(), "test4", service)
	assert.Equal(s.T(), "/2/p1/test", rewritePath)
	service, rewritePath, _ = matcher.Pick("/p2/test", nil, true)
	assert.Equal(s.T(), "test4", service)
	assert.Equal(s.T(), "/2/p2/test", rewritePath)
	service, rewritePath, _ = matcher.Pick("/2/p1/test", nil, true)
	assert.Equal(s.T(), "test4", service)
	assert.Equal(s.T(), "/2/p1/test", rewritePath)
	service, rewritePath, _ = matcher.Pick("/2/p2/test", nil, true)
	assert.Equal(s.T(), "test4", service)
	assert.Equal(s.T(), "/2/p2/test", rewritePath)
	service, rewritePath, _ = matcher.Pick("/test/mytest", nil, true)
	assert.Equal(s.T(), "test5", service)
	service, rewritePath, _ = matcher.Pick("/test/mytest", nil, false)
	assert.Equal(s.T(), "test5", service)
	serviceName := matcher.URIToServiceName("/test/mytest", nil)
	assert.Equal(s.T(), "test5", serviceName)
	service, rewritePath, b := matcher.Pick("abcde", nil, false)
	assert.Equal(s.T(), false, b)
	serviceName = matcher.URIToServiceName("abcde", nil)
	assert.Equal(s.T(), "", serviceName)
	service, rewritePath, _ = matcher.Pick("/p3/test", []byte("a=b&c=d"), true)
	assert.Equal(s.T(), "test6", service)
	assert.Equal(s.T(), "/p3/b/test", rewritePath)
}

func (s *APITestSuite) TestNewLocationMatcherError() {
	locationTestErrorData := []string{`
`,
		`
http-locations:
`,
		`
http-locations:
 test.domain:
`,
		`
http-locations:
 test.domain:
 - match: ^(/|/2/)(p1|p2).*
   type: regexp
   upstream: test1
   rewriteRules:
   - 'exact / ^/(p1|p2)/(.*) /2/$1/$2 /^../'
`,
		`
http-locations:
 test.domain:
 - match: ^(/|/2/)(p1|p2).*
   type: abcd
   upstream: test1
   rewriteRules:
   - 'exact ^/(p1|p2)/(.*) /2/$1/$2 /^../'
`,
		`
http-locations:
 test.domain:
 - match: ^(/|/2/)(p1|p2).*
   type: regexp
   upstream: test1
   rewriteRules:
   - 'regexp [ /2/$1/$2 /^../'
`,
		`
http-locations:
 test.domain:
 - match: '['
   type: iregexp
   upstream: test1
   rewriteRules:
   - '!iregexp [ /2/$1/$2 /^../'
`,
		`
http-locations:
 test.domain:
 - match: '['
   type: regexp
   upstream: test1
   rewriteRules:
   - 'exact ^/(p1|p2)/(.*) [ /^../'
`,
		`
http-locations:
 test.domain:
 - match: ^(/|/2/)(p1|p2).*
   type: ''
   upstream: test1
   rewriteRules:
   - ''
`,
	}
	for _, j := range locationTestErrorData {
		context := &core.Context{}
		context.Config, _ = config.NewConfigFromReader(bytes.NewReader([]byte(j)))
		matcher := NewLocationMatcherFromContext("test.domain", context)
		assert.Equal(s.T(), 0, len(matcher.exactLocations))
	}
}

func (s *APITestSuite) TestNewRewriteRuleError() {
	wrongRules := []string{
		`exact /Test2/1 /(.*) /test /test`,
		`abcd /Test2/1 /(.*) /test`,
		`regexp [ /(.*) /test`,
		`!iregexp [ ^/test2/(.*) /test/$1`,
		`exact /Test2/1 [ /test`,
	}
	for _, j := range wrongRules {
		r, err := newRewriteRule(j)
		assert.Nil(s.T(), r)
		assert.NotNil(s.T(), err)
	}
}

func (a *APITestSuite) TestString() {
	var r ProxyMatchType = 1
	assert.Equal(a.T(), r.String(), "regexp")
	var i ProxyMatchType = 2
	assert.Equal(a.T(), i.String(), "iregexp")
	var s ProxyMatchType = 3
	assert.Equal(a.T(), s.String(), "start")
	var e ProxyMatchType = 4
	assert.Equal(a.T(), e.String(), "exact")
	var u ProxyMatchType = 5
	assert.Equal(a.T(), u.String(), "unknown")
}

func (a *APITestSuite) TestRewriteString() {
	var r ProxyRewriteType = 1
	assert.Equal(a.T(), r.String(), "regexp")
	var i ProxyRewriteType = 2
	assert.Equal(a.T(), i.String(), "iregexp")
	var s ProxyRewriteType = 3
	assert.Equal(a.T(), s.String(), "start")
	var e ProxyRewriteType = 4
	assert.Equal(a.T(), e.String(), "exact")
	var ev ProxyRewriteType = 5
	assert.Equal(a.T(), ev.String(), "regexpVar")
	var u ProxyRewriteType = 6
	assert.Equal(a.T(), u.String(), "unknown")
}
