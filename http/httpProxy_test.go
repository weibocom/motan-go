package http

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLocationMatcher(t *testing.T) {
	matcher := NewLocationMatcher([]*ProxyLocation{
		{Upstream: "test1", Match: "/", Type: "start", RewriteRules: []string{"exact /Test2/1 /(.*) /test"}},
		{Upstream: "test2", Match: "/test2/.*", Type: "regexp", RewriteRules: []string{"!iregexp ^/Test2/1/.* ^/test2/(.*) /test/$1"}},
		{Upstream: "test3", Match: "/test3/.*", Type: "iregexp", RewriteRules: []string{"start / ^/(.*) /test/$1"}},
		{Upstream: "test4", Match: "^(/|/2/)(p1|p2).*", Type: "regexp", RewriteRules: []string{"start / ^/(p1|p2)/(.*) /2/$1/$2"}},
	})

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
	fmt.Println(matcher.Pick("/p1/test", true))
	fmt.Println(matcher.Pick("/p2/test", true))
	fmt.Println(matcher.Pick("/2/p1/test", true))
	fmt.Println(matcher.Pick("/2/p2/test", true))
}
