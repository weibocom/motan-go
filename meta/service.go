package meta

const (
	MetaServiceName = "com.weibo.api.motan.runtime.meta.MetaService"
	MetaMethodName  = "getDynamicMeta"
)

type MetaService struct {
}

func (s *MetaService) getDynamicMeta() map[string]string {
	return GetDynamicMeta()
}
