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
)

// NodeClientConfig wraps config
// used for creating a NodeClient
type NodeClientConfig struct {
	RPCEndpoint                      string
	DefaultMonitoringIntervalSeconds int
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
		JSONRPCURL: config.RPCEndpoint,
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
func (nc *NodeClient) WatchSyncStatus(ctx context.Context, observedBlockHeights chan<- int64, logMessages chan<- string) {
	// create channel that will emit
	// an event every 10 seconds
	ticker := time.NewTicker(time.Duration(nc.config.DefaultMonitoringIntervalSeconds) * time.Second).C

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker:
			nodeState, err := nc.GetNodeState()

			if err != nil {
				// log error, but don't block the monitoring
				// routine if the logMessage channel is full
				go func() {
					logMessages <- fmt.Sprintf("error %s getting node status", err)
				}()

				// keep watching
				continue
			}

			go func() {
				logMessages <- fmt.Sprintf("node state %+v", nodeState)
				observedBlockHeights <- nodeState.SyncInfo.LatestBlockHeight
			}()
		}
	}
}
