package main

import (
	"errors"
)

const (
	DefaultMetricSamplesToKeepPerNode                 = 10000
	DefaultMetricSamplesForSyntheticMetricCalculation = 60
)

var (
	ErrNodeMetricsNotFound       = errors.New("no metrics found for requested node")
	ErrInsufficientMetricSamples = errors.New("insufficient metric samples")
)

// NodeMetrics wrap a collection of
// metric samples for a single node
type NodeMetrics struct {
	SyncStatusMetrics *SyncStatusMetrics
}

// Represents a collection of one or more distinct
// (by node id) kava nodes that back a given endpoint
// e.g. the nodes that serve traffic for rpc.data.kava.io
// and the metric samples that have been taken by the doctor
// for those nodes (aggregated by node id)
type Endpoint struct {
	PerNodeMetrics                             map[string][]NodeMetrics
	URL                                        string
	MetricSamplesToKeepPerNode                 int
	MetricSamplesForSyntheticMetricCalculation int
}

// EndpointConfig wraps config values
// for an Endpoint
type EndpointConfig struct {
	URL                                        string
	MetricSamplesToKeepPerNode                 int
	MetricSamplesForSyntheticMetricCalculation int
}

// NewEndpoint returns a new endpoint for tracking
// metrics related to all nodes behind the endpoint
func NewEndpoint(config EndpointConfig) *Endpoint {
	metricSamplesToKeepPerNode := DefaultMetricSamplesToKeepPerNode
	metricSamplesForSyntheticMetricCalculation := DefaultMetricSamplesForSyntheticMetricCalculation

	if config.MetricSamplesToKeepPerNode > 0 {
		metricSamplesToKeepPerNode = config.MetricSamplesToKeepPerNode
	}

	if config.MetricSamplesForSyntheticMetricCalculation > 0 {
		metricSamplesForSyntheticMetricCalculation = config.MetricSamplesForSyntheticMetricCalculation
	}

	return &Endpoint{
		PerNodeMetrics:             make(map[string][]NodeMetrics),
		URL:                        config.URL,
		MetricSamplesToKeepPerNode: metricSamplesToKeepPerNode,
		MetricSamplesForSyntheticMetricCalculation: metricSamplesForSyntheticMetricCalculation,
	}

}

// AddSample adds metrics for a node to the collection of
// metrics for that node, pruning the oldest metrics until only
// MetricSamplesToKeepPerNode are present
func (e *Endpoint) AddSample(nodeId string, newMetrics NodeMetrics) {
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

// CalculateNodeHashRatePerSecond attempts to calculate the average number of blocks
// hashed per second by the specified node based on the most recent
// (up to DefaultMetricSamplesForSyntheticMetricCalculation) samples
// of sync metrics for the node
// if no sync metrics for the node exists, `ErrNodeMetricsNotFound` is returned
// if less than two sync metrics exist for the node, `ErrInsufficientMetricSamples`
// is returned
func (e *Endpoint) CalculateNodeHashRatePerSecond(nodeId string) (float32, error) {
	metricSamples, exists := e.PerNodeMetrics[nodeId]

	if !exists {
		return 0, ErrNodeMetricsNotFound
	}

	var numSamples int
	var samples []*SyncStatusMetrics

	for _, metricSample := range metricSamples {
		// only use up to DefaultMetricSamplesForSyntheticMetricCalculation
		if numSamples == e.MetricSamplesForSyntheticMetricCalculation {
			break
		}
		//  need SyncStatusMetrics to calculate hash rate
		if metricSample.SyncStatusMetrics == nil {
			continue
		}

		samples = append(samples, metricSample.SyncStatusMetrics)

		numSamples++
	}

	// need at least two samples to calculate hash rate
	if numSamples <= 1 {
		return 0, ErrInsufficientMetricSamples
	}

	// calculate running average for hash rate
	var sumBlockRates float32
	startingBlockHeight := samples[0].SyncStatus.LatestBlockHeight
	startingBlockTime := samples[0].SampledAt

	// remove the first sample so it isn't double counted
	samples = samples[1:]

	for _, sample := range samples {
		// calculate how many blocks were hashed in between the two samples
		newBlocks := sample.SyncStatus.LatestBlockHeight - startingBlockHeight
		secondsBetweenSamples := sample.SampledAt.Sub(startingBlockTime).Seconds()

		blockRate := float32(newBlocks) / float32(secondsBetweenSamples)
		sumBlockRates += float32(blockRate)

		// update iteration values for next loop
		startingBlockHeight = sample.SyncStatus.LatestBlockHeight
		startingBlockTime = sample.SampledAt
	}

	// subtract 1 for the average because we are always
	// taking the delta between at least two samples
	return sumBlockRates / float32(numSamples-1), nil
}
