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
	KavaURL string
	Logger  *log.Logger
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
			fmt.Printf("%s is synched up to block %d, %d seconds behind live, status check took %d milliseconds\n", kavaNodeRPCURL, syncStatusMetrics.SyncStatus.LatestBlockHeight, syncStatusMetrics.SecondsBehindLive, syncStatusMetrics.MeasurementLatencyMilliseconds)
			// TODO: check to see if we should log this to a file
			// TODO: check to see if we should this to cloudwatch
		case logMessage := <-logMessages:
			c.Println(logMessage)
		}
	}
}

// NewCLI creates and returns a new cli
// using the provided configuration and error (if any)
func NewCLI(config CLIConfig) (*CLI, error) {
	endpoint := NewEndpoint(EndpointConfig{URL: config.KavaURL})

	return &CLI{
		kavaEndpoint: endpoint,
		Logger:       config.Logger,
	}, nil
}
