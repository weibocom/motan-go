module github.com/weibocom/motan-go

go 1.11

require (
	git.intra.weibo.com/openapi_rd/weibo-motan-go v0.1.11
	github.com/StackExchange/wmi v0.0.0-20181212234831-e0a55b97c705 // indirect
	github.com/afex/hystrix-go v0.0.0-20180502004556-fa1af6a1f4f5
	github.com/beberlei/fastcgi-serve v0.0.0-20151230120321-4676005f65b7
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-ole/go-ole v1.2.4 // indirect
	github.com/golang/protobuf v1.2.0
	github.com/juju/ratelimit v1.0.1
	github.com/kr/pretty v0.1.0 // indirect
	github.com/mitchellh/mapstructure v1.1.2
	github.com/opentracing/opentracing-go v1.0.2
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20181016184325-3113b8401b8a
	github.com/samuel/go-zookeeper v0.0.0-20180130194729-c4fab1ac1bec
	github.com/shirou/gopsutil v2.18.12+incompatible
	github.com/shirou/w32 v0.0.0-20160930032740-bb4de0191aa4 // indirect
	github.com/smartystreets/goconvey v0.0.0-20190330032615-68dc04aab96a // indirect
	github.com/stretchr/testify v1.2.2
	github.com/valyala/fasthttp v1.2.0
	github.com/weibreeze/breeze-go v0.1.1
	go.uber.org/atomic v1.3.2 // indirect
	go.uber.org/multierr v1.1.0 // indirect
	go.uber.org/zap v0.0.0-00010101000000-000000000000
	golang.org/x/net v0.0.0-20181005035420-146acd28ed58
	golang.org/x/sys v0.0.0-20180903190138-2b024373dcd9 // indirect
	google.golang.org/genproto v0.0.0-20180831171423-11092d34479b // indirect
	google.golang.org/grpc v1.15.0
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gopkg.in/yaml.v2 v2.2.1
)

replace (
	cloud.google.com/go => github.com/GoogleCloudPlatform/google-cloud-go v0.30.0
	go.uber.org/atomic => github.com/uber-go/atomic v1.4.0
	go.uber.org/multierr => github.com/uber-go/multierr v1.1.1-0.20180122172545-ddea229ff1df
	go.uber.org/zap => github.com/uber-go/zap v1.9.1
	golang.org/x/crypto => github.com/golang/crypto v0.0.0-20181203042331-505ab145d0a9
	golang.org/x/lint => github.com/golang/lint v0.0.0-20181011164241-5906bd5c48cd
	golang.org/x/net => github.com/golang/net v0.0.0-20181017193950-04a2e542c03f
	golang.org/x/oauth2 => github.com/golang/oauth2 v0.0.0-20181017192945-9dcd33a902f4
	golang.org/x/sync => github.com/golang/sync v0.0.0-20180314180146-1d60e4601c6f
	golang.org/x/sys => github.com/golang/sys v0.0.0-20181011152604-fa43e7bc11ba
	golang.org/x/text => github.com/golang/text v0.3.0
	golang.org/x/time => github.com/golang/time v0.0.0-20181108054448-85acf8d2951c
	golang.org/x/tools => github.com/golang/tools v0.0.0-20181017214349-06f26fdaaa28
	google.golang.org/appengine => github.com/golang/appengine v1.2.0
	google.golang.org/genproto => github.com/google/go-genproto v0.0.0-20181016170114-94acd270e44e
	google.golang.org/grpc => github.com/grpc/grpc-go v1.15.0
)
