package provider

import (
	"testing"

	"github.com/weibocom/motan-go/core"
)

func TestMetaProvider_Call(t *testing.T) {
	tests := []struct {
		name    string
		request core.Request
		wantErr string
	}{
		{
			name: "Motan1 Protocol Not Supported",
			request: &core.MotanRequest{
				RPCContext: &core.RPCContext{IsMotanV1: true},
			},
			wantErr: "meta provider not support motan1 protocol",
		},
		{
			name: "Successful Serialization",
			request: &core.MotanRequest{
				RPCContext: &core.RPCContext{IsMotanV1: false},
				RequestID:  12345,
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &MetaProvider{}
			resp := provider.Call(tt.request)

			if tt.wantErr != "" {
				if resp.GetException() == nil || resp.GetException().ErrMsg != tt.wantErr {
					t.Errorf("MetaProvider.Call() error = %v, wantErr %v", resp.GetException(), tt.wantErr)
				}
			} else {
				if resp.GetException() != nil {
					t.Errorf("MetaProvider.Call() unexpected error = %v", resp.GetException())
				}
				if resp.GetRequestID() != tt.request.GetRequestID() {
					t.Errorf("MetaProvider.Call() requestID = %v, want %v", resp.GetRequestID(), tt.request.GetRequestID())
				}
				if !resp.GetRPCContext(true).Serialized {
					t.Errorf("MetaProvider.Call() response not serialized")
				}
			}
		})
	}
}
