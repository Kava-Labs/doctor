package metric

import (
	"time"

	"github.com/kava-labs/doctor/clients/kava"
)

// MetricDimensions represent arbitrary
// key value tags to associate with a given metric
// during collection
type MetricDimensions = map[string]string

// Metric represents an arbitrary metric
type Metric struct {
	Name       string           `json:"name"`
	Dimensions MetricDimensions `json:"dimensions"`
	Data       interface{}      `json:"data"`
}

// SyncStatusMetrics wraps metrics collected
// by the doctor related to the nodes sync state
type SyncStatusMetrics struct {
	NodeId                    string        `json:"node_id"`
	SampleLatencyMilliseconds int64         `json:"sample_latency_milliseconds"`
	SyncStatus                kava.SyncInfo `json:"sync_status"`
	SecondsBehindLive         int64         `json:"seconds_behind_live"`
	SampledAt                 time.Time     `json:"sampled_at"`
}

// UptimeMetric wraps values used to calculate
// availability metrics for a given kava endpoint
type UptimeMetric struct {
	EndpointURL                    string    `json:"endpoint_url"`
	Up                             bool      `json:"up"`
	SampledAt                      time.Time `json:"sampled_at"`
	RollingAveragePercentAvailable float32   `json:"rolling_average_percent_available"`
}

// HashRateMetric wraps values for how fast
// a node is hashing blocks
type HashRateMetric struct {
	NodeId          string  `json:"node_id"`
	BlocksPerSecond float32 `json:"blocks_per_second"`
}
