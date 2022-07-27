package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kava-labs/doctor/clients/kava"
	"github.com/stretchr/testify/assert"
)

const (
	DefaultTestKavaURL = "https://example.kava.io"
)

func TestAddSampleForNodeWithNoPreviousSamples(t *testing.T) {
	endpoint := createEndpoint()

	nodeId := uuid.New().String()

	endpoint.AddSample(nodeId, NodeMetrics{
		SyncStatusMetrics: &SyncStatusMetrics{
			NodeId: nodeId,
		},
	})

	nodeMetrics := endpoint.PerNodeMetrics[nodeId]

	assert.Equal(t, len(nodeMetrics), 1, "only one sample was added")

	assert.NotNil(t, nodeMetrics[0].SyncStatusMetrics)

	assert.Equal(t, nodeMetrics[0].SyncStatusMetrics.NodeId, nodeId, "sample node id should match test node id")
}

func TestAddSampleForNodeWithPreviousSamplesInOrder(t *testing.T) {
	endpoint := createEndpoint()

	nodeId := uuid.New().String()

	sample1 := NodeMetrics{
		SyncStatusMetrics: &SyncStatusMetrics{
			NodeId:            nodeId,
			SecondsBehindLive: 1,
		},
	}

	sample2 := NodeMetrics{
		SyncStatusMetrics: &SyncStatusMetrics{
			NodeId:            nodeId,
			SecondsBehindLive: 0,
		},
	}

	endpoint.AddSample(nodeId, sample1)
	endpoint.AddSample(nodeId, sample2)

	nodeMetrics := endpoint.PerNodeMetrics[nodeId]

	assert.Equal(t, len(nodeMetrics), 2, "only two samples were added")

	for _, nodeMetric := range nodeMetrics {
		assert.NotNil(t, nodeMetric.SyncStatusMetrics)

	}

	assert.Equal(t, nodeMetrics[0], sample1, "sample node id should match test node id")
	assert.Equal(t, nodeMetrics[1], sample2, "sample node id should match test node id")
}

func TestAddSamplePrunesOldestSample(t *testing.T) {
	maxSamplesToKeepPerNode := 1
	endpoint := NewEndpoint(EndpointConfig{URL: DefaultTestKavaURL, MetricSamplesToKeepPerNode: maxSamplesToKeepPerNode})

	nodeId := uuid.New().String()

	sample1 := NodeMetrics{
		SyncStatusMetrics: &SyncStatusMetrics{
			NodeId:            nodeId,
			SecondsBehindLive: 1,
		},
	}

	sample2 := NodeMetrics{
		SyncStatusMetrics: &SyncStatusMetrics{
			NodeId:            nodeId,
			SecondsBehindLive: 0,
		},
	}

	endpoint.AddSample(nodeId, sample1)
	endpoint.AddSample(nodeId, sample2)

	nodeMetrics := endpoint.PerNodeMetrics[nodeId]

	assert.Equal(t, len(nodeMetrics), maxSamplesToKeepPerNode, fmt.Sprintf("only %d should be kept per node", maxSamplesToKeepPerNode))

	assert.Equal(t, nodeMetrics[0], sample2, "oldest sample should be pruned")
}

func TestAddSampleAggregatesSamplesByNodeId(t *testing.T) {
	endpoint := createEndpoint()

	nodeId1 := uuid.New().String()
	nodeId2 := uuid.New().String()

	sample1 := NodeMetrics{
		SyncStatusMetrics: &SyncStatusMetrics{
			NodeId:            nodeId1,
			SecondsBehindLive: 1,
		},
	}

	sample2 := NodeMetrics{
		SyncStatusMetrics: &SyncStatusMetrics{
			NodeId:            nodeId2,
			SecondsBehindLive: 0,
		},
	}

	endpoint.AddSample(nodeId1, sample1)
	endpoint.AddSample(nodeId2, sample2)

	node1Metrics := endpoint.PerNodeMetrics[nodeId1]

	assert.Equal(t, len(node1Metrics), 1, "only one samples was added for this node")
	assert.NotNil(t, node1Metrics[0].SyncStatusMetrics)
	assert.Equal(t, node1Metrics[0], sample1, "sample node id should match test node id")

	node2Metrics := endpoint.PerNodeMetrics[nodeId2]

	assert.Equal(t, len(node2Metrics), 1, "only one samples was added for this node")
	assert.NotNil(t, node2Metrics[0].SyncStatusMetrics)
	assert.Equal(t, node2Metrics[0], sample2, "sample node id should match test node id")
}

func TestCalculateNodeHashRateReturnsErrWhenNoSamplesForNode(t *testing.T) {
	endpoint := createEndpoint()

	nodeId := uuid.New().String()

	_, err := endpoint.CalculateNodeHashRatePerSecond(nodeId)

	assert.NotNil(t, err)

	assert.EqualError(t, err, ErrNodeMetricsNotFound.Error())
}

func TestCalculateNodeHashRateReturnsErrWhenOnlyOneSamplesForNode(t *testing.T) {
	endpoint := createEndpoint()

	nodeId := uuid.New().String()

	endpoint.AddSample(nodeId, NodeMetrics{
		SyncStatusMetrics: &SyncStatusMetrics{
			NodeId: nodeId,
		},
	})

	_, err := endpoint.CalculateNodeHashRatePerSecond(nodeId)

	assert.NotNil(t, err)

	assert.EqualError(t, err, ErrInsufficientMetricSamples.Error())
}

// when multiple samples (3,,4,5)
func TestCalculateNodeHashRateCalculatesHashRateBasedOnSamples(t *testing.T) {
	endpoint := createEndpoint()

	nodeId := uuid.New().String()

	now := time.Now()

	sample1 := NodeMetrics{
		SyncStatusMetrics: &SyncStatusMetrics{
			NodeId:            nodeId,
			SecondsBehindLive: 1,
			SampledAt:         now,
			SyncStatus: kava.SyncInfo{
				LatestBlockHeight: 3,
			},
		},
	}

	sample2 := NodeMetrics{
		SyncStatusMetrics: &SyncStatusMetrics{
			NodeId:            nodeId,
			SecondsBehindLive: 1,
			SampledAt:         now.Add(1 * time.Second),
			SyncStatus: kava.SyncInfo{
				LatestBlockHeight: 6,
			},
		},
	}

	sample3 := NodeMetrics{
		SyncStatusMetrics: &SyncStatusMetrics{
			NodeId:            nodeId,
			SecondsBehindLive: 1,
			SampledAt:         now.Add(2 * time.Second),
			SyncStatus: kava.SyncInfo{
				LatestBlockHeight: 10,
			},
		},
	}

	sample4 := NodeMetrics{
		SyncStatusMetrics: &SyncStatusMetrics{
			NodeId:            nodeId,
			SecondsBehindLive: 1,
			SampledAt:         now.Add(3 * time.Second),
			SyncStatus: kava.SyncInfo{
				LatestBlockHeight: 15,
			},
		},
	}

	endpoint.AddSample(nodeId, sample1)
	endpoint.AddSample(nodeId, sample2)
	endpoint.AddSample(nodeId, sample3)
	endpoint.AddSample(nodeId, sample4)

	hashRatePerSecond, err := endpoint.CalculateNodeHashRatePerSecond(nodeId)

	assert.Nil(t, err)

	assert.Equal(t, float32(4.0), hashRatePerSecond)
}

func TestCalculateUptimeReturnsErrWhenNoSamplesForNode(t *testing.T) {
	endpoint := createEndpoint()

	endpointURL := uuid.New().String()

	_, err := endpoint.CalculateUptime(endpointURL)

	assert.NotNil(t, err)

	assert.EqualError(t, err, ErrNodeMetricsNotFound.Error())
}

func TestCalculateUptimeReturnsErrWhenNoUptimeSamplesForNode(t *testing.T) {
	endpoint := createEndpoint()

	endpointURL := uuid.New().String()

	sample1 := NodeMetrics{
		SyncStatusMetrics: &SyncStatusMetrics{},
	}

	endpoint.AddSample(endpointURL, sample1)

	_, err := endpoint.CalculateUptime(endpointURL)

	assert.NotNil(t, err)

	assert.EqualError(t, err, ErrInsufficientMetricSamples.Error())
}

func TestCalculateUptimeCalculatesUptimeBasedOnSamples(t *testing.T) {
	endpoint := createEndpoint()

	endpointURL := uuid.New().String()

	sample1 := NodeMetrics{
		UptimeMetric: &UptimeMetric{
			Up: true,
		},
	}

	sample2 := NodeMetrics{
		UptimeMetric: &UptimeMetric{
			Up: false,
		},
	}

	endpoint.AddSample(endpointURL, sample1)
	endpoint.AddSample(endpointURL, sample2)

	uptime, err := endpoint.CalculateUptime(endpointURL)

	assert.Nil(t, err)

	assert.Equal(t, float32(0.5), uptime)
}

func createEndpoint() *Endpoint {
	return NewEndpoint(EndpointConfig{URL: DefaultTestKavaURL})
}
