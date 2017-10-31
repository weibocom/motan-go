package metrics

import "github.com/rcrowley/go-metrics"

type statWriter interface {
	Write(snap metrics.Registry) error
}
