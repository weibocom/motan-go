package http

import (
	"fmt"
	"testing"
)

func TestProxyLocation_CompileScript(t *testing.T) {

	script := `
set $a 1
set $b 2
rmatch $request_uri ^(.*)
jt L1
ret
L1:
rewrite /test1/(.*) /backend/test1/$1
ret
`
	l := &ProxyLocation{
		Script: script,
	}
	l.CompileScript()
	path := l.DeterminePath("/test1/ceshi", true)
	fmt.Println(path)
}
