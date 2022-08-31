// gui.go contains types, functions and methods for creating
// and updating the graphical and interactive UI to the doctor program

package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"

	"github.com/kava-labs/doctor/collect"
	dconfig "github.com/kava-labs/doctor/config"
	"github.com/kava-labs/doctor/metric"
	"github.com/spf13/viper"
)

// GUIConfig wraps values
// used to configure the GUI
// display mode of the doctor program
type GUIConfig struct {
	DebugLoggingEnabled                        bool
	KavaURL                                    string
	RefreshRateSeconds                         int
	MaxMetricSamplesToRetainPerNode            int
	MetricSamplesForSyntheticMetricCalculation int
	MetricCollectors                           []string
	AWSRegion                                  string
	MetricNamespace                            string
}

// GUI controls the display
// mode of the doctor program
// using asci interactive tty
// output devices
type GUI struct {
	grid               *ui.Grid
	updateParagraph    func(count int)
	draw               func(count int, paragraph string)
	newMessageFunc     func(message string)
	updateUptimeFunc   func(uptime float32)
	kavaEndpoint       *Endpoint
	metricCollectors   []collect.Collector
	refreshRateSeconds int
	debugMode          bool
	*log.Logger
}

// Watch watches for new measurements and log messages for the kava node with the
// specified rpc api url, outputting them to the gui device in the desired format
func (g *GUI) Watch(metricReadOnlyChannels MetricReadOnlyChannels, logMessages <-chan string, kavaNodeRPCURL string) error {
	tickerCount := 1

	// create channel to subscribe to
	// user input
	uiEvents := ui.PollEvents()

	// create channel that will emit
	// an event every second
	ticker := time.NewTicker(time.Second * time.Duration(g.refreshRateSeconds)).C

	// handle logging in separate go-routines to avoid
	// congestion with metric event emission
	go func() {
		// events triggered by debug worthy events
		for logMessage := range logMessages {
			// TODO: separate channels
			// for debug only log messages?
			if !g.debugMode {
				continue
			}
			g.newMessageFunc(logMessage)
		}
	}()

	for {
		select {
		// events triggered by user input
		// or action such as keyboard strokes
		// mouse movements or window changes
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				ui.Close()

				return nil
			case "c":
				updatedParagraph := fmt.Sprintf(
					`Current Config %+v
				`, viper.AllSettings())

				g.newMessageFunc(updatedParagraph)

				time.Sleep(1 * time.Second)
			case "l":
				// TODO: allow paging through metrics per node
				message := fmt.Sprintf("Accumulated Metrics %+v", g.kavaEndpoint.PerNodeMetrics)

				g.newMessageFunc(message)

				time.Sleep(3 * time.Second)
			case "<Resize>":
				payload := e.Payload.(ui.Resize)

				g.grid.SetRect(0, 0, payload.Width, payload.Height)

				ui.Clear()

				ui.Render(g.grid)
			}
		// events triggered by new metric data
		case syncStatusMetrics := <-metricReadOnlyChannels.SyncStatusMetrics:
			// record sample in-memory for use in synthetic metric calculation
			g.kavaEndpoint.AddSample(syncStatusMetrics.NodeId, NodeMetrics{
				SyncStatusMetrics: &syncStatusMetrics,
			})

			// calculate hash rate for this node
			nodeId := syncStatusMetrics.NodeId

			hashRatePerSecond, err := g.kavaEndpoint.CalculateNodeHashRatePerSecond(nodeId)

			if err != nil {
				g.newMessageFunc(fmt.Sprintf("error %s calculating hash rate for node %s\n", err, nodeId))
			}

			latestBlockHeight := syncStatusMetrics.SyncStatus.LatestBlockHeight
			secondsBehindLive := syncStatusMetrics.SecondsBehindLive
			syncStatusLatencyMilliseconds := syncStatusMetrics.SampleLatencyMilliseconds

			updatedParagraph := fmt.Sprintf(
				`Node %s
			Latest Block Height %d
			Seconds Behind Live %d
			Blocks Hashed (per second) %f
			Sync Status Latency (milliseconds) %d
			`, nodeId, latestBlockHeight, secondsBehindLive, hashRatePerSecond, syncStatusLatencyMilliseconds)

			g.draw(tickerCount, updatedParagraph)

			// collect metrics to external storage backends
			var metrics []metric.Metric

			hashRateMetric := metric.Metric{
				Name: "BlocksHashedPerSecond",
				Dimensions: map[string]string{
					"node_id": nodeId,
				},
				Data: metric.HashRateMetric{
					NodeId:          nodeId,
					BlocksPerSecond: hashRatePerSecond,
				},
				Value:               float64(hashRatePerSecond),
				Timestamp:           syncStatusMetrics.SampledAt,
				CollectToFile:       true,
				CollectToCloudwatch: true,
			}

			metrics = append(metrics, hashRateMetric)

			syncStatusMetric := metric.Metric{
				Name: "SyncStatus",
				Dimensions: map[string]string{
					"node_id": nodeId,
				},
				Data:                syncStatusMetrics,
				CollectToFile:       true,
				CollectToCloudwatch: false,
			}

			metrics = append(metrics, syncStatusMetric)

			latestBlockHeightMetric := metric.Metric{
				Name: "LatestBlockHeight",
				Dimensions: map[string]string{
					"node_id": nodeId,
				},
				Value:               float64(latestBlockHeight),
				Timestamp:           syncStatusMetrics.SampledAt,
				CollectToFile:       false,
				CollectToCloudwatch: true,
			}

			metrics = append(metrics, latestBlockHeightMetric)

			secondsBehindLiveMetric := metric.Metric{
				Name: "SecondsBehindLive",
				Dimensions: map[string]string{
					"node_id": nodeId,
				},
				Value:               float64(secondsBehindLive),
				Timestamp:           syncStatusMetrics.SampledAt,
				CollectToFile:       false,
				CollectToCloudwatch: true,
			}

			metrics = append(metrics, secondsBehindLiveMetric)

			statusCheckMillisecondLatencyMetric := metric.Metric{
				Name: "StatusCheckLatencyMilliseconds",
				Dimensions: map[string]string{
					"node_id": nodeId,
				},
				Value:               float64(syncStatusLatencyMilliseconds),
				Timestamp:           syncStatusMetrics.SampledAt,
				CollectToFile:       false,
				CollectToCloudwatch: true,
			}

			metrics = append(metrics, statusCheckMillisecondLatencyMetric)

			for _, collector := range g.metricCollectors {
				for _, metric := range metrics {
					err := collector.Collect(metric)

					if err != nil {
						g.newMessageFunc(fmt.Sprintf("error %s collecting metric %+v\n", err, metric))
					}
				}

			}

		// events triggered by new metric data
		case uptimeMetric := <-metricReadOnlyChannels.UptimeMetrics:
			endpointURL := uptimeMetric.EndpointURL
			// record sample in-memory for use in synthetic metric calculation
			g.kavaEndpoint.AddSample(uptimeMetric.EndpointURL, NodeMetrics{
				UptimeMetric: &uptimeMetric,
			})

			// calculate uptime
			uptime, err := g.kavaEndpoint.CalculateUptime(uptimeMetric.EndpointURL)

			if err != nil {
				g.newMessageFunc(fmt.Sprintf("error %s calculating uptime for %s\n", err, uptimeMetric.EndpointURL))
				continue
			}

			// update uptime gauge
			g.updateUptimeFunc(uptime)

			// collect metrics to external storage backends
			var metrics []metric.Metric

			uptimeMetric.RollingAveragePercentAvailable = uptime * 100
			uptimeMetricForCollection := metric.Metric{
				Name: "Uptime",
				Dimensions: map[string]string{
					"endpoint_url": endpointURL,
				},
				Data:                uptimeMetric,
				Value:               float64(uptimeMetric.RollingAveragePercentAvailable),
				Timestamp:           uptimeMetric.SampledAt,
				CollectToFile:       true,
				CollectToCloudwatch: true,
			}

			metrics = append(metrics, uptimeMetricForCollection)

			for _, collector := range g.metricCollectors {
				for _, metric := range metrics {
					err := collector.Collect(metric)

					if err != nil {
						g.newMessageFunc(fmt.Sprintf("error %s collecting metric %+v\n", err, metric))
					}
				}

			}
		// events triggered on a regular time based interval
		case <-ticker:
			g.updateParagraph(tickerCount)

			g.draw(tickerCount, "")

			tickerCount++
		}
	}
}

func NewGUI(config GUIConfig) (*GUI, error) {
	if err := ui.Init(); err != nil {
		panic(fmt.Errorf("failed to initialize termui: %v", err))
	}

	// upper left box
	syncMetrics := widgets.NewParagraph()
	syncMetrics.Title = "Sync Metrics"
	syncMetrics.Text = `PRESS q TO QUIT
	PRESS c TO VIEW CONFIG
	PRESS l TO LIST SAMPLES
	`
	syncMetrics.SetRect(0, 0, 50, 5)
	syncMetrics.TextStyle.Fg = ui.ColorWhite
	syncMetrics.BorderStyle.Fg = ui.ColorCyan

	updateParagraph := func(count int) {
		if count%2 == 0 {
			syncMetrics.TextStyle.Fg = ui.ColorRed
		} else {
			syncMetrics.TextStyle.Fg = ui.ColorWhite
		}
	}

	// upper right box
	messages := widgets.NewParagraph()
	messages.Title = "Messages"
	messages.Text = ""
	messages.SetRect(0, 0, 50, 5)
	messages.TextStyle.Fg = ui.ColorYellow
	messages.BorderStyle.Fg = ui.ColorMagenta

	// lower left box
	uptimeMetric := widgets.NewGauge()
	uptimeMetric.Title = "Uptime Metric"
	uptimeMetric.Percent = 0
	uptimeMetric.SetRect(0, 12, 50, 15)
	uptimeMetric.BarColor = ui.ColorRed
	uptimeMetric.BorderStyle.Fg = ui.ColorWhite
	uptimeMetric.TitleStyle.Fg = ui.ColorCyan

	sparklineData := []float64{4, 2, 1, 6, 3, 9, 1, 4, 2, 15, 14, 9, 8, 6, 10, 13, 15, 12, 10, 5, 3, 6, 1, 7, 10, 10, 14, 13, 6, 4, 2, 1, 6, 3, 9, 1, 4, 2, 15, 14, 9, 8, 6, 10, 13, 15, 12, 10, 5, 3, 6, 1, 7, 10, 10, 14, 13, 6, 4, 2, 1, 6, 3, 9, 1, 4, 2, 15, 14, 9, 8, 6, 10, 13, 15, 12, 10, 5, 3, 6, 1, 7, 10, 10, 14, 13, 6, 4, 2, 1, 6, 3, 9, 1, 4, 2, 15, 14, 9, 8, 6, 10, 13, 15, 12, 10, 5, 3, 6, 1, 7, 10, 10, 14, 13, 6}

	sl := widgets.NewSparkline()
	sl.Title = "srv 0:"
	sl.Data = sparklineData
	sl.LineColor = ui.ColorCyan
	sl.TitleStyle.Fg = ui.ColorWhite

	sl2 := widgets.NewSparkline()
	sl2.Title = "srv 1:"
	sl2.Data = sparklineData
	sl2.TitleStyle.Fg = ui.ColorWhite
	sl2.LineColor = ui.ColorRed

	slg := widgets.NewSparklineGroup(sl, sl2)
	slg.Title = "Sparkline"
	slg.SetRect(25, 5, 50, 12)

	sinData := (func() []float64 {
		n := 220
		ps := make([]float64, n)
		for i := range ps {
			ps[i] = 1 + math.Sin(float64(i)/5)
		}
		return ps
	})()

	lc := widgets.NewPlot()
	lc.Title = "dot-marker Line Chart"
	lc.Data = make([][]float64, 1)
	lc.Data[0] = sinData
	lc.SetRect(0, 15, 50, 25)
	lc.AxesColor = ui.ColorWhite
	lc.LineColors[0] = ui.ColorRed
	lc.Marker = widgets.MarkerDot

	barchartData := []float64{3, 2, 5, 3, 9, 5, 3, 2, 5, 8, 3, 2, 4, 5, 3, 2, 5, 7, 5, 3, 2, 6, 7, 4, 6, 3, 6, 7, 8, 3, 6, 4, 5, 3, 2, 4, 6, 4, 8, 5, 9, 4, 3, 6, 5, 3, 6}

	bc := widgets.NewBarChart()
	bc.Title = "Bar Chart"
	bc.SetRect(50, 0, 75, 10)
	bc.Labels = []string{"S0", "S1", "S2", "S3", "S4", "S5"}
	bc.BarColors[0] = ui.ColorGreen
	bc.NumStyles[0] = ui.NewStyle(ui.ColorBlack)

	// lower right box
	lc2 := widgets.NewPlot()
	lc2.Title = "braille-mode Line Chart"
	lc2.Data = make([][]float64, 1)
	lc2.Data[0] = sinData
	lc2.SetRect(50, 15, 75, 25)
	lc2.AxesColor = ui.ColorWhite
	lc2.LineColors[0] = ui.ColorYellow

	// set up 4 x 4 grid
	grid := ui.NewGrid()
	termWidth, termHeight := ui.TerminalDimensions()
	grid.SetRect(0, 0, termWidth, termHeight)

	grid.Set(
		ui.NewRow(1.0/2,
			ui.NewCol(1.0/2, syncMetrics),
			ui.NewCol(1.0/2, messages),
		),
		ui.NewRow(1.0/2,
			ui.NewCol(1.0/4, uptimeMetric),
			ui.NewCol(1.0/4,
				ui.NewRow(.9/3, slg),
				ui.NewRow(.9/3, lc),
				ui.NewRow(1.2/3, bc),
			),
			ui.NewCol(1.0/2, lc2),
		),
	)

	// setup function to call whenever
	// there is new data
	draw := func(count int, paragraph string) {
		slg.Sparklines[0].Data = sparklineData[:30+count%50]
		slg.Sparklines[1].Data = sparklineData[:35+count%50]
		lc.Data[0] = sinData[count/2%220:]
		lc2.Data[0] = sinData[2*count%220:]
		bc.Data = barchartData[count/2%10:]
		if paragraph != "" {
			syncMetrics.Text = paragraph
		}
		ui.Render(grid)
	}

	// setup function to call whenever
	// the uptime metric needs to be updated
	updateUptime := func(uptime float32) {
		uptimeMetric.Percent = int(math.Round(float64(uptime * 100)))
		ui.Render(grid)
	}

	// setup function to call whenever there
	// is new debug / log messages to show
	newMessage := func(message string) {
		messages.Text = message
		ui.Render(grid)
	}

	// show the initial ui to the user
	ui.Render(grid)

	endpoint := NewEndpoint(EndpointConfig{URL: config.KavaURL,
		MetricSamplesToKeepPerNode:                 config.MaxMetricSamplesToRetainPerNode,
		MetricSamplesForSyntheticMetricCalculation: config.MetricSamplesForSyntheticMetricCalculation,
	})

	collectors := []collect.Collector{}

	for _, collector := range config.MetricCollectors {
		switch collector {
		case dconfig.FileMetricCollector:
			fileCollector, err := collect.NewFileCollector(collect.FileCollectorConfig{})

			if err != nil {
				return nil, err
			}

			collectors = append(collectors, fileCollector)
		case dconfig.CloudwatchMetricCollector:
			cloudwatchConfig := collect.CloudWatchCollectorConfig{
				Ctx:             context.Background(),
				AWSRegion:       config.AWSRegion,
				MetricNamespace: config.MetricNamespace,
			}

			cloudwatchCollector, err := collect.NewCloudWatchCollector(cloudwatchConfig)

			if err != nil {
				return nil, err
			}

			collectors = append(collectors, cloudwatchCollector)
		}
	}

	return &GUI{
		refreshRateSeconds: config.RefreshRateSeconds,
		debugMode:          config.DebugLoggingEnabled,
		grid:               grid,
		updateParagraph:    updateParagraph,
		updateUptimeFunc:   updateUptime,
		draw:               draw,
		newMessageFunc:     newMessage,
		kavaEndpoint:       endpoint,
		metricCollectors:   collectors,
	}, nil
}
