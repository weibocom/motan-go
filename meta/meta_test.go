package meta

import (
	"errors"
	"github.com/weibocom/motan-go/endpoint"
	"github.com/weibocom/motan-go/serialize"
	"testing"

	"github.com/weibocom/motan-go/core"
)

func TestIsSupport(t *testing.T) {
	tests := []struct {
		name     string
		cacheKey string
		url      *core.URL
		want     bool
	}{
		{
			name:     "Supported Protocol and Serializer",
			cacheKey: "test1",
			url: &core.URL{
				Protocol: "motan2",
				Parameters: map[string]string{
					core.DynamicMetaKey:   "true",
					core.SerializationKey: "breeze",
				},
			},
			want: true,
		},
		{
			name:     "Unsupported Protocol",
			cacheKey: "test2",
			url: &core.URL{
				Protocol: "motan",
				Parameters: map[string]string{
					core.DynamicMetaKey:   "true",
					core.SerializationKey: "breeze",
				},
			},
			want: false,
		},
		{
			name:     "Unsupported Serializer",
			cacheKey: "test3",
			url: &core.URL{
				Protocol: "motan2",
				Parameters: map[string]string{
					core.DynamicMetaKey:   "true",
					core.SerializationKey: "protobuf",
				},
			},
			want: false,
		},
		{
			name:     "Dynamic Meta Disabled",
			cacheKey: "test4",
			url: &core.URL{
				Protocol: "motan2",
				Parameters: map[string]string{
					core.DynamicMetaKey:   "false",
					core.SerializationKey: "breeze",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSupport(tt.cacheKey, tt.url); got != tt.want {
				t.Errorf("isSupport() = %v, want %v", got, tt.want)
			}
			notSupportCache.Flush()
		})
	}
}

func TestGetEpStaticMeta(t *testing.T) {
	tests := []struct {
		name     string
		endpoint core.EndPoint
		want     map[string]string
	}{
		{
			name: "With DefaultMetaPrefix",
			endpoint: &endpoint.MockEndpoint{
				URL: &core.URL{
					Parameters: map[string]string{
						core.DefaultMetaPrefix + "key1": "value1",
						"otherKey":                      "value2",
					},
				},
			},
			want: map[string]string{
				core.DefaultMetaPrefix + "key1": "value1",
			},
		},
		{
			name: "With EnvPrefix",
			endpoint: &endpoint.MockEndpoint{
				URL: &core.URL{
					Parameters: map[string]string{
						envPrefix + "key2": "value3",
						"otherKey":         "value4",
					},
				},
			},
			want: map[string]string{
				envPrefix + "key2": "value3",
			},
		},
		{
			name: "No Matching Prefix",
			endpoint: &endpoint.MockEndpoint{
				URL: &core.URL{
					Parameters: map[string]string{
						"key3": "value5",
					},
				},
			},
			want: map[string]string{},
		},
		{
			name:     "Nil URL",
			endpoint: &endpoint.MockEndpoint{URL: nil},
			want:     map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetEpStaticMeta(tt.endpoint); !equalMaps(got, tt.want) {
				t.Errorf("GetEpStaticMeta() = %v, want %v", got, tt.want)
			}
		})
	}
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestGetRemoteDynamicMeta(t *testing.T) {
	breezeSerialization := &serialize.BreezeSerialization{}
	resultWant := map[string]string{"key": "value"}
	resultBody, _ := breezeSerialization.Serialize(resultWant)
	tests := []struct {
		name      string
		cacheKey  string
		available bool
		endpoint  *endpoint.MockEndpoint
		want      map[string]string
		wantErr   error
	}{
		{
			name:      "Service Not Supported",
			cacheKey:  "test1",
			available: true,
			endpoint: &endpoint.MockEndpoint{
				URL: &core.URL{
					Protocol: "motan2",
					Parameters: map[string]string{
						core.DynamicMetaKey:   "true",
						core.SerializationKey: "breeze",
					},
				},
				MockResponse: &core.MotanResponse{
					Exception: &core.Exception{ErrMsg: core.ServiceNotSupport},
				},
			},
			want:    nil,
			wantErr: ServiceNotSupportError,
		},
		{
			name:      "Endpoint Unavailable",
			cacheKey:  "test2",
			available: false,
			endpoint: &endpoint.MockEndpoint{
				URL: &core.URL{
					Protocol: "motan2",
					Parameters: map[string]string{
						core.DynamicMetaKey:   "true",
						core.SerializationKey: "breeze",
					},
				},
			},
			want:    nil,
			wantErr: errors.New("endpoint unavailable"),
		},
		{
			name:      "Successful Call",
			cacheKey:  "test3",
			available: true,
			endpoint: &endpoint.MockEndpoint{
				URL: &core.URL{
					Protocol: "motan2",
					Parameters: map[string]string{
						core.DynamicMetaKey:   "true",
						core.SerializationKey: "breeze",
					},
				},
				MockResponse: &core.MotanResponse{
					Value: &core.DeserializableValue{
						Body: resultBody,
					},
				},
			},
			want:    resultWant, // expected a deserialized map
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.endpoint.SetAvailable(tt.available)
			got, err := getRemoteDynamicMeta(tt.cacheKey, tt.endpoint)
			if err != nil && err.Error() != tt.wantErr.Error() {
				t.Errorf("getRemoteDynamicMeta() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !equalMaps(got, tt.want) {
				t.Errorf("getRemoteDynamicMeta() = %v, want %v", got, tt.want)
			}
			notSupportCache.Flush()
		})
	}
}
