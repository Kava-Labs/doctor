package main

import "errors"

const (
	DefaultMetricSamplesToKeepPerNode = 10000
)

var (
	ErrNodeMetricsNotFound = errors.New("no metrics found for requested node")
)

// NodeMetrics wrap a collection of
// metric samples for a single node
type NodeMetrics struct {
	SyncStatusMetrics []SyncStatusMetrics
}

// Represents a collection of one or more distinct
// (by node id) kava nodes that back a given endpoint
// e.g. the nodes that serve traffic for rpc.data.kava.io
// and the metric samples that have been taken by the doctor
// for those nodes (aggregated by node id)
type Endpoint struct {
	PerNodeMetrics             map[string][]NodeMetrics
	URL                        string
	MetricSamplesToKeepPerNode int
}

// EndpointConfig wraps config values
// for an Endpoint
type EndpointConfig struct {
	URL                        string
	MetricSamplesToKeepPerNode int
}

// NewEndpoint returns a new endpoint for tracking
// metrics related to all nodes behind the endpoint
func NewEndpoint(config EndpointConfig) *Endpoint {
	metricSamplesToKeepPerNode := DefaultMetricSamplesToKeepPerNode

	if config.MetricSamplesToKeepPerNode > 0 {
		metricSamplesToKeepPerNode = config.MetricSamplesToKeepPerNode
	}

	return &Endpoint{
		PerNodeMetrics:             make(map[string][]NodeMetrics),
		URL:                        config.URL,
		MetricSamplesToKeepPerNode: metricSamplesToKeepPerNode,
	}

}

// AddNodeMetrics adds metrics for a node to the collection of
// metrics for that node, pruning the oldest metrics until only
// MetricSamplesToKeepPerNode are present
func (e *Endpoint) AddNodeMetrics(nodeId string, newMetrics NodeMetrics) {
	currentMetrics, exists := e.PerNodeMetrics[nodeId]

	if !exists {
		e.PerNodeMetrics[nodeId] = []NodeMetrics{newMetrics}
		return
	}

	if len(currentMetrics) == e.MetricSamplesToKeepPerNode {
		// prune the oldest metric
		e.PerNodeMetrics[nodeId] = currentMetrics[1:]
	}

	e.PerNodeMetrics[nodeId] = append(e.PerNodeMetrics[nodeId], newMetrics)
}

// CalculateNodeCatchupSeconds attempts to calculate the number of seconds
// until a given node in the endpoint will catch up to live, based on the most
// recent (up to 5 samples) of sync metrics for the node
// if no metrics for the node exists, `ErrNodeMetricsNotFound` is returned
func (e *Endpoint) CalculateNodeCatchupSeconds(nodeId string) (int64, error) {
	_, exists := e.PerNodeMetrics[nodeId]

	if !exists {
		return 0, ErrNodeMetricsNotFound
	}

	return 0, nil
}
