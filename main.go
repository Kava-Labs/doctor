package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

var (
	// default context representing the lifetime
	// of a single invocation of the doctor program
	ctx = context.Background()
)

func main() {
	// set up channel for sending log messages
	// from async node health watching routines to
	// either the gui or cli output device
	logMessages := make(chan string)

	// set up channel for sending updated
	// node sync status to metric collection and display
	// endpoints
	observedBlockHeights := make(chan int64)

	// parse desired configuration
	config, err := GetDoctorConfig()

	if err != nil {
		panic(err)
	}

	// log the initial config
	go func() {
		logMessages <- fmt.Sprintf("doctor parsed config %+v", config)
	}()

	// setup client for talking to the rpc
	// api of the node to gather application
	// metrics such as current block height and time
	// for the doctor to use the watch the health of the node
	nodeConfig := NodeClientConfig{RPCEndpoint: config.KavaNodeRPCURL}

	nodeClient, err := NewNodeClient(nodeConfig)

	if err != nil {
		panic(fmt.Errorf("%w: could not initialize kava client using %+v", err, nodeConfig))
	}

	// watch the node's sync status endpoint
	// to measure it's block syncing performance
	go nodeClient.WatchSyncStatus(ctx, observedBlockHeights, logMessages)

	// setup event handlers for interactive mode
	if config.InteractiveMode {
		// create and draw the initial interface
		guiConfig := GUIConfig{}

		gui, err := NewGUI(guiConfig)

		if err != nil {
			panic(fmt.Errorf("error %s attempting to start interactive mode ", err))
		}

		// display new node health measurements as
		// they are received and evaluated
		// and allow the user to interactively
		// adjust the display and measurement
		err = gui.Watch(observedBlockHeights, logMessages, config.KavaNodeRPCURL)

		if err != nil {
			panic(fmt.Errorf("error %s attempting to watch node in interactive mode ", err))
		}
	} else {
		// setup plaintext or file cli interface
		cliConfig := CLIConfig{
			Logger: config.Logger,
		}

		cli, err := NewCLI(cliConfig)

		if err != nil {
			panic(fmt.Errorf("error %s attempting to start non-interactive mode ", err))
		}

		errChan := make(chan error, 1)

		// display new node health measurements as
		// they are received and evaluated to the cli
		// output device (stdout or file)
		go func() {
			defer close(errChan)

			err = cli.Watch(observedBlockHeights, logMessages, config.KavaNodeRPCURL)

			if err != nil {
				errChan <- fmt.Errorf("error %s attempting to watch node in non-interactive mode ", err)
			}
		}()

		// setup handling of os signals such as Ctrl ^C
		signals := make(chan os.Signal, 2)
		defer close(signals)

		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		// keep running the doctor program until the
		// watch is finished or the user sends the interrupt or stop
		// signals in the tty
		for {
			select {
			case <-signals:
				os.Exit(0)
			case err = <-errChan:
				if err != nil {
					panic(errChan)
				}
				os.Exit(0)
			}
		}
	}
}
