package collect

import (
	"github.com/kava-labs/doctor/metric"
)

// Collector allows for collecting a metric to an
// arbitrary metric sink (e.g. a file or AWS CloudWatch)
// for historical and real time monitoring purposes
type Collector interface {
	Collect(metric metric.Metric) error
}
