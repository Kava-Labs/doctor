// cli.go contains types, functions and methods for creating
// and updating the command line interface (CLI) to the doctor program

package main

import (
	"fmt"
	"log"
)

// CLIConfig wraps values
// used to configure the CLI
// display mode of the doctor program
type CLIConfig struct {
	KavaURL                                    string
	MaxMetricSamplesToRetainPerNode            int
	MetricSamplesForSyntheticMetricCalculation int
	Logger                                     *log.Logger
}

// CLI controls the display
// mode of the doctor program
// using either stdout or file based
// output devices
type CLI struct {
	kavaEndpoint *Endpoint
	*log.Logger
}

// Watch watches for new measurements and log messages for the kava node with the
// specified rpc api url, outputting them to the cli device in the desired format
func (c *CLI) Watch(metricReadOnlyChannels MetricReadOnlyChannels, logMessages <-chan string, kavaNodeRPCURL string) error {
	// event handlers for non-interactive mode
	// loop over events
	for {
		select {
		case syncStatusMetrics := <-metricReadOnlyChannels.SyncStatusMetrics:
			// TODO: log to configured backends (stdout, file and or cloudwatch)
			// for now log new monitoring data to stdout by default

			nodeId := syncStatusMetrics.NodeId

			hashRatePerSecond, err := c.kavaEndpoint.CalculateNodeHashRatePerSecond(nodeId)
			if err != nil {
				fmt.Printf("error %s calculating hash rate for node %s\n", err, nodeId)
			}

			fmt.Printf("%s node %s is synched up to block %d, %d seconds behind live, hashing %f blocks per second, status check took %d milliseconds\n", kavaNodeRPCURL, nodeId, syncStatusMetrics.SyncStatus.LatestBlockHeight, syncStatusMetrics.SecondsBehindLive, hashRatePerSecond, syncStatusMetrics.SampleLatencyMilliseconds)

			// TODO: check to see if we should log this to a file
			// TODO: check to see if we should this to cloudwatch
			c.kavaEndpoint.AddSample(syncStatusMetrics.NodeId, NodeMetrics{
				SyncStatusMetrics: &syncStatusMetrics,
			})
		case logMessage := <-logMessages:
			c.Println(logMessage)
		}
	}
}

// NewCLI creates and returns a new cli
// using the provided configuration and error (if any)
func NewCLI(config CLIConfig) (*CLI, error) {
	endpoint := NewEndpoint(EndpointConfig{URL: config.KavaURL,
		MetricSamplesToKeepPerNode:                 config.MaxMetricSamplesToRetainPerNode,
		MetricSamplesForSyntheticMetricCalculation: config.MetricSamplesForSyntheticMetricCalculation,
	})

	return &CLI{
		kavaEndpoint: endpoint,
		Logger:       config.Logger,
	}, nil
}
