// cli.go contains types, functions and methods for creating
// and updating the command line interface (CLI) to the doctor program

package main

import (
	"fmt"
	"log"
	"time"
)

// CLIConfig wraps values
// used to configure the CLI
// display mode of the doctor program
type CLIConfig struct {
	Logger *log.Logger
}

// CLI controls the display
// mode of the doctor program
// using either stdout or file based
// output devices
type CLI struct {
	*log.Logger
}

// Watch watches for new measurements and log messages for the kava node with the
// specified rpc api url, outputting them to the cli device in the desired format
func (c *CLI) Watch(kavaNodeReadings <-chan int64, logMessages <-chan string, kavaNodeRPCURL string) error {
	// event handlers for non-interactive mode
	// loop over events
	for {
		select {
		case newBlockHeight := <-kavaNodeReadings:
			// TODO: log to configured backends (stdout, file and or cloudwatch)
			// for now log new monitoring data to stdout by default
			fmt.Printf("%s is synched up to block %d @ %d\n", kavaNodeRPCURL, newBlockHeight, time.Now().Unix())
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
	return &CLI{
		config.Logger,
	}, nil
}
