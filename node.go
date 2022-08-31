// node.go contains types, functions and methods
// for interacting with kava nodes to determine
// their application and infrastructure health
// based off various metrics such as time behind live
// and memory consumption or i/o latency

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/kava-labs/doctor/clients/kava"
	"github.com/kava-labs/doctor/heal"
	"github.com/kava-labs/doctor/metric"
)

// NodeClientConfig wraps config
// used for creating a NodeClient
type NodeClientConfig struct {
	RPCEndpoint                         string
	DefaultMonitoringIntervalSeconds    int
	Autoheal                            bool // whether doctor should take active measures to attempt to heal the kava process (e.g. place on standby if it falls significantly behind live)
	AutohealSyncLatencyToleranceSeconds int
}

// NodeClient provides methods
// for interacting with the kava node
// API and OS shell for a given node
type NodeClient struct {
	*kava.Client
	config      NodeClientConfig
	healCounter *heal.ActiveHealerCounter
}

// NewNodeCLient creates and returns a new node client
// using the provided configuration
func NewNodeClient(config NodeClientConfig) (*NodeClient, error) {
	kavaClient, err := kava.New(kava.ClientConfig{
		JSONRPCURL: config.RPCEndpoint,
	})

	if err != nil {
		panic(fmt.Errorf("%w: could not initialize kava client", err))
	}

	healCounter := heal.ActiveHealerCounter{}

	return &NodeClient{
		config:      config,
		Client:      kavaClient,
		healCounter: &healCounter,
	}, nil
}

// WatchSyncStatus watches  (until the context is cancelled)
// the sync status for the node and sends any new data to the provided channel.
func (nc *NodeClient) WatchSyncStatus(ctx context.Context, syncStatusMetrics chan<- metric.SyncStatusMetrics, uptimeMetrics chan<- metric.UptimeMetric, logMessages chan<- string) {
	// create channel that will emit
	// an event every DefaultMonitoringIntervalSeconds seconds
	ticker := time.NewTicker(time.Duration(nc.config.DefaultMonitoringIntervalSeconds) * time.Second).C

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker:
			// get the current sync status of the node
			// timing how long it takes for the node
			// to respond to the request as well
			startTime := time.Now()
			nodeState, err := nc.GetNodeState()
			endTime := time.Now()

			uptimeMetric := metric.UptimeMetric{
				EndpointURL: nc.config.RPCEndpoint,
				SampledAt:   startTime,
				Up:          true,
			}

			if err != nil {
				uptimeMetric.Up = false
				// log error, but don't block the monitoring
				// routine if the logMessage channel is full
				go func() {
					logMessages <- fmt.Sprintf("error %s getting node status", err)
					uptimeMetrics <- uptimeMetric
				}()

				// keep watching
				continue
			}

			var secondsBehindLive int64
			currentSyncTime := nodeState.SyncInfo.LatestBlockTime
			secondsBehindLive = int64(time.Since(currentSyncTime).Seconds())

			metrics := metric.SyncStatusMetrics{
				SampledAt:                 startTime,
				NodeId:                    nodeState.NodeInfo.Id,
				SyncStatus:                nodeState.SyncInfo,
				SampleLatencyMilliseconds: endTime.Sub(startTime).Milliseconds(),
				SecondsBehindLive:         secondsBehindLive,
			}

			go func() {
				logMessages <- fmt.Sprintf("node state %+v", nodeState)
				syncStatusMetrics <- metrics
				uptimeMetrics <- uptimeMetric
			}()

			if nc.config.Autoheal {
				if secondsBehindLive > int64(nc.config.AutohealSyncLatencyToleranceSeconds) {
					// check to see if there is already a healer working on this issue
					nc.healCounter.Lock()
					defer nc.healCounter.Unlock()

					// only have one healer working on the same issue at once
					if nc.healCounter.Count != 0 {
						return
					}

					go func() {
						logMessages <- fmt.Sprintf("node %s is more than %d seconds behind live: %d, attempting autohealing actions", nodeState.NodeInfo.Id, nc.config.AutohealSyncLatencyToleranceSeconds, secondsBehindLive)
					}()

					// node, heal thyself
					go heal.StandbyNodeUntilCaughtUp(logMessages, nc.healCounter)
				}
			}
		}
	}
}
