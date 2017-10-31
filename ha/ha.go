package ha

import (
	motan "github.com/weibocom/motan-go/core"
)

// ext name
const (
	FailOver = "failover"
)

func RegistDefaultHa(extFactory motan.ExtentionFactory) {
	extFactory.RegistExtHa(FailOver, func(url *motan.Url) motan.HaStrategy {
		return &FailOverHA{url: url}
	})
}
