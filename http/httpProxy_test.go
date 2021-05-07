package http

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/config"
	"github.com/weibocom/motan-go/core"
	"testing"
)

func TestNewLocationMatcher(t *testing.T) {
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
`

	context := &core.Context{}
	context.Config, _ = config.NewConfigFromReader(bytes.NewReader([]byte(locationTestData)))
	matcher := NewLocationMatcherFromContext("test.domain", context)
	service := ""
	rewritePath := ""
	service, rewritePath, _ = matcher.Pick("/Test3/1", true)
	assert.Equal(t, "test3", service)
	assert.Equal(t, "/test/Test3/1", rewritePath)
	service, rewritePath, _ = matcher.Pick("/test3/1", true)
	assert.Equal(t, "test3", service)
	service, rewritePath, _ = matcher.Pick("/test2/1/1", true)
	assert.Equal(t, "test2", service)
	assert.Equal(t, "/test2/1/1", rewritePath)
	service, rewritePath, _ = matcher.Pick("/test2/2/1", true)
	assert.Equal(t, "test2", service)
	assert.Equal(t, "/test/2/1", rewritePath)
	service, rewritePath, _ = matcher.Pick("/Test2/1", true)
	assert.Equal(t, "test1", service)
	assert.Equal(t, "/test", rewritePath)
	service, rewritePath, _ = matcher.Pick("/p1/test", true)
	assert.Equal(t, "test4", service)
	assert.Equal(t, "/2/p1/test", rewritePath)
	service, rewritePath, _ = matcher.Pick("/p2/test", true)
	assert.Equal(t, "test4", service)
	assert.Equal(t, "/2/p2/test", rewritePath)
	service, rewritePath, _ = matcher.Pick("/2/p1/test", true)
	assert.Equal(t, "test4", service)
	assert.Equal(t, "/2/p1/test", rewritePath)
	service, rewritePath, _ = matcher.Pick("/2/p2/test", true)
	assert.Equal(t, "test4", service)
	assert.Equal(t, "/2/p2/test", rewritePath)
	service, rewritePath, _ = matcher.Pick("/test/mytest", true)
	assert.Equal(t, "test5", service)
	service, rewritePath, _ = matcher.Pick("/test/mytest", false)
	assert.Equal(t, "test5", service)
	serviceName := matcher.URIToServiceName("/test/mytest")
	assert.Equal(t, "test5", serviceName)
	service, rewritePath, b := matcher.Pick("abcde", false)
	assert.Equal(t, false, b)
	serviceName = matcher.URIToServiceName("abcde")
	assert.Equal(t, "", serviceName)
}

func TestNewLocationMatcherError(t *testing.T) {
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
		assert.Equal(t, 0, len(matcher.exactLocations))
	}
}

func TestNewRewriteRuleError(t *testing.T) {
	wrongRules := []string{
		`exact /Test2/1 /(.*) /test /test`,
		`abcd /Test2/1 /(.*) /test`,
		`regexp [ /(.*) /test`,
		`!iregexp [ ^/test2/(.*) /test/$1`,
		`exact /Test2/1 [ /test`,
	}
	for _, j := range wrongRules {
		r, err := newRewriteRule(j)
		assert.Nil(t, r)
		assert.NotNil(t, err)
	}
}

func TestString(t *testing.T) {
	var r ProxyMatchType = 1
	assert.Equal(t, r.String(), "regexp")
	var i ProxyMatchType = 2
	assert.Equal(t, i.String(), "iregexp")
	var s ProxyMatchType = 3
	assert.Equal(t, s.String(), "start")
	var e ProxyMatchType = 4
	assert.Equal(t, e.String(), "exact")
	var u ProxyMatchType = 5
	assert.Equal(t, u.String(), "unknown")
}
