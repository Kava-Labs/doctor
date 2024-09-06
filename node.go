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
	AutohealBlockchainServiceName       string
	AutohealSyncLatencyToleranceSeconds int
	AutohealSyncToLiveToleranceSeconds  int
	AutohealRestartDelaySeconds         int
	AutohealInitialAllowedDelaySeconds  int
	HealthChecksTimeoutSeconds          int
	NoNewBlocksRestartThresholdSeconds  int
	DowntimeRestartThresholdSeconds     int
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

	var outOfSyncAutohealingInProgress bool
	var lastRestartedByAutohealingAt *time.Time
	lastNewBlockObservedAt := time.Now()
	var lastSynchedBlockNumber int64
	var currentDowntimeStartedAt *time.Time

	earliestAllowedRestartTime := time.Now().Add(time.Duration(nc.config.AutohealInitialAllowedDelaySeconds) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker:
			// get the current sync status of the node
			// timing how long it takes for the node
			// to respond to the request as well
			statusCheckStartedAt := time.Now()
			nodeState, err := nc.GetNodeState()
			statusCheckEndedAt := time.Now()

			uptimeMetric := metric.UptimeMetric{
				EndpointURL: nc.config.RPCEndpoint,
				SampledAt:   statusCheckStartedAt,
				Up:          true,
			}

			if err != nil {
				// send uptime metric to metric collector
				// for aggregation and storage
				uptimeMetric.Up = false
				// log error, but don't block the monitoring
				// routine if the logMessage channel is full
				go func() {
					logMessages <- fmt.Sprintf("error %s getting node status", err)
					uptimeMetrics <- uptimeMetric
				}()

				// if this is the first time the api was unavailable
				// or it went down after being restarted
				// set the start of the downtime window
				if currentDowntimeStartedAt == nil {
					logMessages <- fmt.Sprintf("node went offline at %+v", statusCheckStartedAt)
					downtimeStartedAt := statusCheckStartedAt
					currentDowntimeStartedAt = &downtimeStartedAt
				}
				// TODO: refactor into node.AutohealOfflineNode()
				if nc.config.Autoheal {
					// check if the downtime deserves a restart
					downtimeDuration := statusCheckStartedAt.Sub(*currentDowntimeStartedAt)
					logMessages <- fmt.Sprintf("node has been down for %+v downtime threshold seconds %v, restart delay seconds %d", downtimeDuration, nc.config.DowntimeRestartThresholdSeconds, nc.config.AutohealRestartDelaySeconds)

					// if the node was previously restarted
					// don't restart until AutohealRestartDelaySeconds have passed
					if lastRestartedByAutohealingAt != nil {
						if downtimeDuration < time.Duration(time.Duration(nc.config.AutohealRestartDelaySeconds)*time.Second) {
							logMessages <- fmt.Sprintf("not restarting offline node, current downtime %v last restarted %f seconds ago at %v restart delay seconds %d", downtimeDuration, time.Since(*lastRestartedByAutohealingAt).Seconds(), lastRestartedByAutohealingAt, nc.config.AutohealRestartDelaySeconds)

							// keep checking the health of the endpoint
							continue
						}

						// restart the node
						err = nc.RestartBlockchainService()

						if err != nil {
							logMessages <- fmt.Sprintf("error %s restarting node", err)
							// keep checking the health of the endpoint
							continue
						}

						// update the last restarted at time
						now := time.Now()
						lastRestartedByAutohealingAt = &now

						logMessages <- fmt.Sprintf("restarted node at %v", lastRestartedByAutohealingAt)

						// reset downtime clock
						currentDowntimeStartedAt = nil

						// keep checking the health of the endpoint
						continue
					}

					// otherwise only restart the node if it's been down long enough
					if downtimeDuration > time.Duration(time.Duration(nc.config.DowntimeRestartThresholdSeconds)*time.Second) {
						// this is the first time the node is being restarted
						// for the current downtime window
						// restart the node
						err = nc.RestartBlockchainService()

						if err != nil {
							logMessages <- fmt.Sprintf("error %s restarting node", err)
							// keep checking the health of the endpoint
							continue
						}

						// update the last restarted at time
						now := time.Now()
						lastRestartedByAutohealingAt = &now

						logMessages <- fmt.Sprintf("restarted node at %v", lastRestartedByAutohealingAt)

						// reset downtime clock
						currentDowntimeStartedAt = nil

						// keep checking the health of the endpoint
						continue
					}

					logMessages <- fmt.Sprintf("not restarting node, down for %v seconds, downtime threshold seconds %v", downtimeDuration, nc.config.DowntimeRestartThresholdSeconds)

				}

				// keep watching
				continue
			}

			var secondsBehindLive int64
			currentSyncTime := nodeState.SyncInfo.LatestBlockTime
			currentBlockNumber := nodeState.SyncInfo.LatestBlockHeight
			secondsBehindLive = int64(time.Since(currentSyncTime).Seconds())

			metrics := metric.SyncStatusMetrics{
				SampledAt:                 statusCheckStartedAt,
				NodeId:                    nodeState.NodeInfo.Id,
				SyncStatus:                nodeState.SyncInfo,
				SampleLatencyMilliseconds: statusCheckEndedAt.Sub(statusCheckStartedAt).Milliseconds(),
				SecondsBehindLive:         secondsBehindLive,
			}

			go func() {
				logMessages <- fmt.Sprintf("node state %+v", nodeState)
				syncStatusMetrics <- metrics
				uptimeMetrics <- uptimeMetric
			}()

			// if the node has synched any new blocks since the last block
			if currentBlockNumber > lastSynchedBlockNumber {
				// update frozen node health indicator
				lastNewBlockObservedAt = statusCheckEndedAt
				logMessages <- "node has synched new blocks since last check"
			} else {
				logMessages <- fmt.Sprintf("node has been frozen for %f seconds since %v\n NoNewBlocksRestartThresholdSeconds %d", statusCheckEndedAt.Sub(lastNewBlockObservedAt).Seconds(), lastNewBlockObservedAt, nc.config.NoNewBlocksRestartThresholdSeconds)
			}

			// TODO: refactor into node.AutohealOutOfSyncNode()
			if nc.config.Autoheal {
				go func() {
					logMessages <- fmt.Sprintf("AutoHeal: node %s is %d seconds behind live, AutohealSyncLatencyToleranceSeconds %d, ", nodeState.NodeInfo.Id, secondsBehindLive, int64(nc.config.AutohealSyncLatencyToleranceSeconds))
				}()
				if secondsBehindLive > int64(nc.config.AutohealSyncLatencyToleranceSeconds) {
					go func() {
						logMessages <- fmt.Sprintf("node %s is more than %d seconds behind live: %d, checking to see if it is already being healed", nodeState.NodeInfo.Id, nc.config.AutohealSyncLatencyToleranceSeconds, secondsBehindLive)
					}()

					// check to see if there is already a healer working on this issue
					if outOfSyncAutohealingInProgress {
						go func() {
							logMessages <- fmt.Sprintf("AutoHeal: node %s is currently being autohealed", nodeState.NodeInfo.Id)
						}()
						goto AutohealFrozenNodeBegin
					}

					outOfSyncAutohealingInProgress = true

					go func() {
						logMessages <- fmt.Sprintf("node %s is more than %d seconds behind live: %d, attempting autohealing actions", nodeState.NodeInfo.Id, nc.config.AutohealSyncLatencyToleranceSeconds, secondsBehindLive)
					}()

					// node, heal thyself
					go func() {
						defer func() {
							go func() {
								logMessages <- "AutoHeal: releasing lock"
							}()
							outOfSyncAutohealingInProgress = false
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

		AutohealFrozenNodeBegin:

			// TODO: refactor into node.AutohealFrozenNode()
			if nc.config.Autoheal {
				// if configured, allow an initial buffer from service start to first autoheal restart
				// if we are still in that initial buffer. if so, continue checking the health
				if time.Now().Before(earliestAllowedRestartTime) {
					logMessages <- fmt.Sprintf("not restarting frozen node, still in initial restart delay buffer: buffer %d sec, first restart allowed at %s", nc.config.AutohealInitialAllowedDelaySeconds, earliestAllowedRestartTime)
					continue
				}

				// check if the node has been frozen long enough to deserve a restart
				frozenDuration := time.Since(lastNewBlockObservedAt)

				if frozenDuration > time.Duration(time.Duration(nc.config.NoNewBlocksRestartThresholdSeconds)*time.Second) {
					// if the node was previously restarted
					// don't restart until AutohealRestartDelaySeconds have passed
					if lastRestartedByAutohealingAt != nil {
						if frozenDuration < time.Duration(time.Duration(nc.config.AutohealRestartDelaySeconds)*time.Second) {
							logMessages <- fmt.Sprintf("not restarting frozen node, current freezetime %v last restarted %f seconds ago at %v restart delay seconds %d", frozenDuration, time.Since(*lastRestartedByAutohealingAt).Seconds(), lastRestartedByAutohealingAt, nc.config.AutohealRestartDelaySeconds)

							// keep checking the health of the endpoint
							continue
						}

						// restart the node
						err = nc.RestartBlockchainService()

						if err != nil {
							logMessages <- fmt.Sprintf("error %s restarting node", err)
							// keep checking the health of the endpoint
							continue
						}

						// update the last restarted at time
						now := time.Now()
						lastRestartedByAutohealingAt = &now

						logMessages <- fmt.Sprintf("restarted node at %v", lastRestartedByAutohealingAt)

						// reset frozen clock
						lastNewBlockObservedAt = time.Now()

						// keep checking the health of the endpoint
						continue
					}

					logMessages <- fmt.Sprintf("autohealing frozen node, last block synched at %v,NoNewBlocksRestartThresholdSeconds %d", lastNewBlockObservedAt, nc.config.NoNewBlocksRestartThresholdSeconds)

					// restart the node
					err = nc.RestartBlockchainService()

					if err != nil {
						logMessages <- fmt.Sprintf("error %s restarting node", err)
						// keep checking the health of the endpoint
						continue
					}

					// update the last restarted at time
					now := time.Now()
					lastRestartedByAutohealingAt = &now

					logMessages <- fmt.Sprintf("restarted node at %v", lastRestartedByAutohealingAt)

					// reset frozen clock
					lastNewBlockObservedAt = time.Now()

					// keep checking the health of the endpoint
					continue
				}

				logMessages <- fmt.Sprintf("not restarting node, frozen for %v seconds, frozen threshold seconds %v", frozenDuration.Seconds(), nc.config.NoNewBlocksRestartThresholdSeconds)
			}

			// update frozen node health indicator
			lastSynchedBlockNumber = currentBlockNumber
		}
	}
}

// RestartBlockchainService restarts the blockchain's systemd service
// returning error (if any)
func (nc *NodeClient) RestartBlockchainService() error {
	return heal.RestartSystemdService(nc.config.AutohealBlockchainServiceName)
}
