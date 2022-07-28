package main

import (
	"errors"

	"github.com/kava-labs/doctor/metric"
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
	SyncStatusMetrics *metric.SyncStatusMetrics
	UptimeMetric      *metric.UptimeMetric
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

// returns up to the most recent metrics that match the given predicate
// TODO: probably not going to ever hit a scaling issue, but would be more efficient
// to have AddSample store up to MetricSamplesForSyntheticMetricCalculation
// per metric type in a separate data structure to avoid having to iterate
// through ALL metrics for each synthetic metric calculation
// see reverseNodeMetrics comment for other optimization ideas
func takeUpToNMostRecentMetrics(metrics *[]NodeMetrics, take int, predicate func(*NodeMetrics) bool) *[]NodeMetrics {
	var takenMetrics []NodeMetrics
	var taken int
	newestToOldestMetrics := reverseNodeMetrics(metrics)

	for _, metric := range *newestToOldestMetrics {
		if taken == take {
			break
		}

		if predicate(&metric) {
			takenMetrics = append(takenMetrics, metric)
			taken++
		}
	}

	return &takenMetrics
}

// memory optimized but naive implementation
// TODO: only reverse in chunks, e.g. take the 100 most recent
// metrics and look for matches, if less matches found than desired
// take the next 100
// can speed up performance using goroutines as well
// https://golangprojectstructure.com/reversing-go-slice-array/
func reverseNodeMetrics(input *[]NodeMetrics) *[]NodeMetrics {
	inputLen := len(*input)
	output := make([]NodeMetrics, inputLen)

	for i, n := range *input {
		j := inputLen - i - 1

		output[j] = n
	}

	return &output
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

	syncStatusMetricMatcher := func(metric *NodeMetrics) bool {
		var match bool
		if metric.SyncStatusMetrics != nil {
			match = true
		}
		return match
	}

	samples := takeUpToNMostRecentMetrics(&metricSamples, e.MetricSamplesForSyntheticMetricCalculation, syncStatusMetricMatcher)

	numSamples := len(*samples)

	// need at least two samples to calculate hash rate
	if numSamples <= 1 {
		return 0, ErrInsufficientMetricSamples
	}

	// calculate running average for hash rate
	var sumBlockRates float32
	startingBlockHeight := (*samples)[0].SyncStatusMetrics.SyncStatus.LatestBlockHeight
	startingBlockTime := (*samples)[0].SyncStatusMetrics.SampledAt

	// remove the first sample so it isn't double counted
	*samples = (*samples)[1:]

	for _, sample := range *samples {
		// calculate how many blocks were hashed in between the two samples
		newBlocks := sample.SyncStatusMetrics.SyncStatus.LatestBlockHeight - startingBlockHeight
		secondsBetweenSamples := sample.SyncStatusMetrics.SampledAt.Sub(startingBlockTime).Seconds()

		blockRate := float32(newBlocks) / float32(secondsBetweenSamples)
		sumBlockRates += float32(blockRate)

		// update iteration values for next loop
		startingBlockHeight = sample.SyncStatusMetrics.SyncStatus.LatestBlockHeight
		startingBlockTime = sample.SyncStatusMetrics.SampledAt
	}

	// subtract 1 for the average because we are always
	// taking the delta between at least two samples
	return sumBlockRates / float32(numSamples-1), nil
}

// CalculateUptime attempts to calculate the overall availability
// for a given endpoint (which may be backed by multiple nodes)
// if no metrics (of any kind) for the endpoint exists,
// `ErrNodeMetricsNotFound` is returned
// if less than one uptime metrics exist for the node,
// `ErrInsufficientMetricSamples` is returned
func (e *Endpoint) CalculateUptime(endpointURL string) (float32, error) {
	metricSamples, exists := e.PerNodeMetrics[endpointURL]

	if !exists {
		return 0, ErrNodeMetricsNotFound
	}

	uptimeMetricMatcher := func(metric *NodeMetrics) bool {
		var match bool
		if metric.UptimeMetric != nil {
			match = true
		}
		return match
	}

	samples := takeUpToNMostRecentMetrics(&metricSamples, e.MetricSamplesForSyntheticMetricCalculation, uptimeMetricMatcher)

	numSamples := len(*samples)

	// need at least one samples to calculate uptime
	if numSamples == 0 {
		return 0, ErrInsufficientMetricSamples
	}

	// count the total number of times the endpoint
	// was "up"
	var availabilityPeriods float32

	for _, sample := range *samples {
		if sample.UptimeMetric.Up {
			availabilityPeriods += 1
		}
	}

	return availabilityPeriods / float32(numSamples), nil
}
