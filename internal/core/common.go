package core

import "github.com/panjf2000/ants/v2"

var processV2Pool, _ = ants.NewPool(20 * 10000)

func GetProcessV2Pool() *ants.Pool {
	return processV2Pool
}
