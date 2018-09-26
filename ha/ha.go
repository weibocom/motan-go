package ha

import (
	motan "github.com/weibocom/motan-go/core"
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
		ha := &BackupRequestHA{}
		ha.SetURL(url)
		ha.Initialize()
		return ha
	})
}
