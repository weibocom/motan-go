package core

import (
	"flag"
	"testing"
)

func TestMain(m *testing.M) {
	flag.Parse()
	m.Run()
}

func TestGetContext(t *testing.T) {
	rs := &Context{ConfigFile: "../config/testconf.yaml"}
	rs.Initialize()

	if rs.RefersURLs == nil {
		t.Error("parse refers urls fail.")
	}

	if rs.RefersURLs["status-rpc-json"] == nil {
		t.Error("parse refer section fail.")
	}
	if rs.RefersURLs["status-rpc-json"].Group != "test-group" {
		t.Error("get refer key fail.")
	}
	if len(rs.ServiceURLs) == 0 {
		t.Error("parse service urls fail")
	}
	if rs.ServiceURLs["mytest-motan2"].Group != "motan-demo-rpc" {
		t.Error("parse serivce key fail")
	}
}
