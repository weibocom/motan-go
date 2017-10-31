package endpoint

import (
	"testing"

	motan "github.com/weibocom/motan-go/core"
)

func TestGetUrl(t *testing.T) {

	url := &motan.Url{}
	grpcEndpoint := &GrpcEndPoint{url: url}
	if grpcEndpoint.GetUrl() != url {
		t.Error("GetName Error")
	}
}
