package meta

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/endpoint"
	vlog "github.com/weibocom/motan-go/log"
	mpro "github.com/weibocom/motan-go/protocol"
	"github.com/weibocom/motan-go/serialize"
)

const (
	defaultCacheExpireSecond    = 3
	notSupportCacheExpireSecond = 30
)

var (
	dynamicMeta            = core.NewStringMap(30)
	envMeta                = make(map[string]string)
	envPrefix              = core.DefaultMetaPrefix
	metaEmptyMap           = make(map[string]string)
	metaCache              = cache.New(time.Second*time.Duration(defaultCacheExpireSecond), 30*time.Second)
	notSupportCache        = cache.New(time.Second*time.Duration(notSupportCacheExpireSecond), 30*time.Second)
	ServiceNotSupportError = errors.New(core.ServiceNotSupport)
	notSupportSerializer   = map[string]bool{
		"protobuf":     true,
		"grpc-pb":      true,
		"grpc-pb-json": true,
	}
	supportProtocols = map[string]bool{
		"motan":  false,
		"motan2": true,
	}
	once = sync.Once{}
)

func Initialize(ctx *core.Context) {
	once.Do(func() {
		expireSecond := defaultCacheExpireSecond
		if ctx != nil && ctx.Config != nil {
			envPrefix = ctx.Config.GetStringWithDefault(core.EnvMetaPrefixKey, core.DefaultMetaPrefix)
			expireSecondStr := ctx.Config.GetStringWithDefault(core.MetaCacheExpireSecondKey, "")
			if expireSecondStr != "" {
				tempCacheExpireSecond, err := strconv.Atoi(expireSecondStr)
				if err == nil && tempCacheExpireSecond > 0 {
					expireSecond = tempCacheExpireSecond
				}
			}
		}
		vlog.Infof("meta cache expire time : %d(s)\n", expireSecond)
		metaCache = cache.New(time.Second*time.Duration(expireSecond), 30*time.Second)
		vlog.Infof("using meta prefix : %s\n", envPrefix)
		// load meta info from env variable
		for _, env := range os.Environ() {
			if strings.HasPrefix(env, envPrefix) {
				kv := strings.Split(env, "=")
				envMeta[kv[0]] = kv[1]
			}
		}
	})
}

func GetEnvMeta() map[string]string {
	return envMeta
}

func PutDynamicMeta(key, value string) {
	dynamicMeta.Store(key, value)
}

func RemoveDynamicMeta(key string) {
	dynamicMeta.Delete(key)
}

func GetDynamicMeta() map[string]string {
	return dynamicMeta.RawMap()
}

func GetMergedMeta() map[string]string {
	mergedMap := make(map[string]string)
	for k, v := range envMeta {
		mergedMap[k] = v
	}
	for k, v := range dynamicMeta.RawMap() {
		mergedMap[k] = v
	}
	return mergedMap
}

func GetMetaValue(meta map[string]string, keySuffix string) string {
	var res string
	if meta != nil {
		if v, ok := meta[envPrefix+keySuffix]; ok {
			res = v
		} else {
			res = meta[core.DefaultMetaPrefix+keySuffix]
		}
	}
	return res
}

func GetEpDynamicMeta(endpoint core.EndPoint) (map[string]string, error) {
	cacheKey := getCacheKey(endpoint.GetURL())
	if v, ok := metaCache.Get(cacheKey); ok {
		return v.(map[string]string), nil
	}
	res, err := getRemoteDynamicMeta(cacheKey, endpoint)
	if err != nil {
		return nil, err
	}
	metaCache.Set(cacheKey, res, cache.DefaultExpiration)
	return res, nil
}

// GetEpStaticMeta get remote static meta information from referer url attachments.
// the static meta is init at server start from env.
func GetEpStaticMeta(endpoint core.EndPoint) map[string]string {
	res := make(map[string]string)
	url := endpoint.GetURL()
	if url != nil {
		for k, v := range url.Parameters {
			if strings.HasPrefix(k, core.DefaultMetaPrefix) || strings.HasPrefix(k, envPrefix) {
				res[k] = v
			}
		}
	}
	return res
}

func getRemoteDynamicMeta(cacheKey string, endpoint core.EndPoint) (map[string]string, error) {
	if _, ok := notSupportCache.Get(cacheKey); ok || !isSupport(cacheKey, endpoint.GetURL()) {
		return nil, ServiceNotSupportError
	}
	if !endpoint.IsAvailable() {
		return nil, errors.New("endpoint unavailable")
	}
	resp := endpoint.Call(getMetaServiceRequest())
	if resp.GetException() != nil {
		if resp.GetException().ErrMsg == core.ServiceNotSupport {
			notSupportCache.Set(cacheKey, true, cache.DefaultExpiration)
			return nil, ServiceNotSupportError
		}
		return nil, errors.New(resp.GetException().ErrMsg)
	}
	if d, ok := resp.GetValue().(*core.DeserializableValue); ok {
		reply := make(map[string]string)
		// only support breeze serializer and motan2 protocol
		breezeSerialize := &serialize.BreezeSerialization{}
		_, err := breezeSerialize.DeSerialize(d.Body, &reply)
		if err != nil {
			return nil, err
		}
		return reply, nil
	}
	// multiple serialization might encode an empty map into interface{}, not map[string]string
	// in this case, return a public empty string map
	if res, ok := resp.GetValue().(map[string]string); ok && res != nil {
		return res, nil
	}
	return metaEmptyMap, nil
}

func getMetaServiceRequest() core.Request {
	req := &core.MotanRequest{
		RequestID:   endpoint.GenerateRequestID(),
		ServiceName: MetaServiceName,
		Method:      MetaMethodName,
		Attachment:  core.NewStringMap(core.DefaultAttachmentSize),
		Arguments:   []interface{}{},
		RPCContext: &core.RPCContext{
			Serialized:   true,
			SerializeNum: serialize.BreezeNumber,
		},
	}
	req.SetAttachment(mpro.MFrameworkService, "y")
	return req
}

func getCacheKey(url *core.URL) string {
	return url.Host + ":" + url.GetPortStr()
}

func isSupport(cacheKey string, url *core.URL) bool {
	// check dynamicMeta config, protocol and serializer
	if url.GetBoolValue(core.DynamicMetaKey, core.DefaultDynamicMeta) &&
		!notSupportSerializer[url.GetStringParamsWithDefault(core.SerializationKey, "")] &&
		supportProtocols[url.Protocol] {
		return true
	}
	notSupportCache.Set(cacheKey, true, cache.DefaultExpiration)
	return false
}

func ClearMetaCache() {
	metaCache.Flush()
	notSupportCache.Flush()
}
