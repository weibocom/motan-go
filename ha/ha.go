package ha

import (
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/filter"
)

// ext name
const (
	FailOver      = "failover"
	BackupRequest = "backupRequest"
)

func RegistDefaultHa(extFactory motan.ExtensionFactory) {
	extFactory.RegistExtHa(FailOver, func(url *motan.URL) motan.HaStrategy {
		return &FailOverHA{url: url}
	})
	extFactory.RegistExtHa(BackupRequest, func(url *motan.URL) motan.HaStrategy {
		if filters, ok := url.Parameters[motan.FilterKey]; ok {
			hasMetrics := false
			arr := motan.TrimSplit(filters, ",")
			for _, str := range arr {
				if str == filter.Metrics {
					hasMetrics = true
				}
			}
			if !hasMetrics {
				url.PutParam(motan.FilterKey, filters+","+filter.Metrics)
			}
		} else {
			url.PutParam(motan.FilterKey, filter.Metrics)
		}
		ha := &BackupRequestHA{}
		ha.SetURL(url)
		ha.Initialize()
		return ha
	})
}
