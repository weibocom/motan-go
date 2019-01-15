package http

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/config"
	"github.com/weibocom/motan-go/core"
)

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
`

func TestNewLocationMatcher(t *testing.T) {
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
}
