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
	AutohealSyncToLiveToleranceSeconds  int
	AutohealRestartDelaySeconds         int
	HealthChecksTimeoutSeconds          int
	NoNewBlocksRestartThresholdSeconds int
	DowntimeRestartThresholdSeconds    int
}

// NodeClient provides methods
// for interacting with the kava node
// API and OS shell for a given node
type NodeClient struct {
	*kava.Client
	config NodeClientConfig
}

// NewNodeCLient creates and returns a new node client
// using the provided configuration
func NewNodeClient(config NodeClientConfig) (*NodeClient, error) {
	kavaClient, err := kava.New(kava.ClientConfig{
		JSONRPCURL:             config.RPCEndpoint,
		HTTPReadTimeoutSeconds: config.HealthChecksTimeoutSeconds,
	})

	if err != nil {
		panic(fmt.Errorf("%w: could not initialize kava client", err))
	}

	return &NodeClient{
		config: config,
		Client: kavaClient,
	}, nil
}

// WatchSyncStatus watches  (until the context is cancelled)
// the sync status for the node and sends any new data to the provided channel.
func (nc *NodeClient) WatchSyncStatus(ctx context.Context, syncStatusMetrics chan<- metric.SyncStatusMetrics, uptimeMetrics chan<- metric.UptimeMetric, logMessages chan<- string) {
	// create channel that will emit
	// an event every DefaultMonitoringIntervalSeconds seconds
	ticker := time.NewTicker(time.Duration(nc.config.DefaultMonitoringIntervalSeconds) * time.Second).C

	var autohealingInProgress bool

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
				go func() {
					logMessages <- fmt.Sprintf("AutoHeal: node %s is %d seconds behind live, AutohealSyncLatencyToleranceSeconds %d, ", nodeState.NodeInfo.Id, secondsBehindLive, int64(nc.config.AutohealSyncLatencyToleranceSeconds))
				}()
				if secondsBehindLive > int64(nc.config.AutohealSyncLatencyToleranceSeconds) {
					go func() {
						logMessages <- fmt.Sprintf("node %s is more than %d seconds behind live: %d, checking to see if it is already being healed", nodeState.NodeInfo.Id, nc.config.AutohealSyncLatencyToleranceSeconds, secondsBehindLive)
					}()

					// check to see if there is already a healer working on this issue
					if autohealingInProgress {
						go func() {
							logMessages <- fmt.Sprintf("AutoHeal: node %s is currently being autohealed", nodeState.NodeInfo.Id)
						}()
						continue
					}

					autohealingInProgress = true

					go func() {
						logMessages <- fmt.Sprintf("node %s is more than %d seconds behind live: %d, attempting autohealing actions", nodeState.NodeInfo.Id, nc.config.AutohealSyncLatencyToleranceSeconds, secondsBehindLive)
					}()

					// node, heal thyself
					go func() {
						defer func() {
							go func() {
								logMessages <- "AutoHeal: releasing lock"
							}()
							autohealingInProgress = false
							go func() {
								logMessages <- "AutoHeal: released lock"
							}()
						}()

						heal.StandbyNodeUntilCaughtUp(logMessages, nc.Client, heal.HealerConfig{
							AutohealSyncToLiveToleranceSeconds: nc.config.AutohealSyncToLiveToleranceSeconds,
						})
					}()
				} else {
					logMessages <- fmt.Sprintf("node %s is less than %d seconds behind live, doesn't need to be auto healed", nodeState.NodeInfo.Id, nc.config.AutohealSyncLatencyToleranceSeconds)
				}
			} else {
				logMessages <- fmt.Sprintf("auto heal not enabled for node %s, skipping autoheal checks", nodeState.NodeInfo.Id)
			}
		}
	}
}
