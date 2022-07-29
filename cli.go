// cli.go contains types, functions and methods for creating
// and updating the command line interface (CLI) to the doctor program

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/kava-labs/doctor/collect"
	"github.com/kava-labs/doctor/metric"
)

// CLIConfig wraps values
// used to configure the CLI
// display mode of the doctor program
type CLIConfig struct {
	KavaURL                                    string
	MaxMetricSamplesToRetainPerNode            int
	MetricSamplesForSyntheticMetricCalculation int
	MetricCollectors                           []string
	AWSRegion                                  string
	MetricNamespace                            string
	Logger                                     *log.Logger
}

// CLI controls the display
// mode of the doctor program
// using either stdout or file based
// output devices
type CLI struct {
	kavaEndpoint *Endpoint
	*log.Logger
	metricCollectors []collect.Collector
}

// Watch watches for new measurements and log messages for the kava node with the
// specified rpc api url, outputting them to the cli device in the desired format
func (c *CLI) Watch(metricReadOnlyChannels MetricReadOnlyChannels, logMessages <-chan string, kavaNodeRPCURL string) error {
	// event handlers for non-interactive mode
	// loop over events
	for {
		select {
		case syncStatusMetrics := <-metricReadOnlyChannels.SyncStatusMetrics:
			// record sample in-memory for use in synthetic metric calculation
			c.kavaEndpoint.AddSample(syncStatusMetrics.NodeId, NodeMetrics{
				SyncStatusMetrics: &syncStatusMetrics,
			})

			// calculate hash rate for this node
			nodeId := syncStatusMetrics.NodeId

			hashRatePerSecond, err := c.kavaEndpoint.CalculateNodeHashRatePerSecond(nodeId)
			if err != nil {
				c.Printf("error %s calculating hash rate for node %s\n", err, nodeId)
			}

			latestBlockHeight := syncStatusMetrics.SyncStatus.LatestBlockHeight
			secondsBehindLive := syncStatusMetrics.SecondsBehindLive
			syncStatusLatencyMilliseconds := syncStatusMetrics.SampleLatencyMilliseconds

			// log to stdout
			fmt.Printf("%s node %s is synched up to block %d, %d seconds behind live, hashing %f blocks per second, status check took %d milliseconds\n", kavaNodeRPCURL, nodeId, latestBlockHeight, secondsBehindLive, hashRatePerSecond, syncStatusLatencyMilliseconds)

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
				Timestamp:           syncStatusMetrics.SampledAt,
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

			for _, collector := range c.metricCollectors {
				for _, metric := range metrics {
					err := collector.Collect(metric)

					if err != nil {
						c.Printf("error %s collecting metric %+v\n", err, metric)
					}
				}

			}
		case uptimeMetric := <-metricReadOnlyChannels.UptimeMetrics:
			endpointURL := uptimeMetric.EndpointURL
			// record sample in-memory for use in synthetic metric calculation
			c.kavaEndpoint.AddSample(endpointURL, NodeMetrics{
				UptimeMetric: &uptimeMetric,
			})

			// calculate uptime
			uptime, err := c.kavaEndpoint.CalculateUptime(endpointURL)

			if err != nil {
				c.Printf(fmt.Sprintf("error %s calculating uptime for %s\n", err, endpointURL))
				continue
			}

			// log to stdout
			fmt.Printf("%s uptime %f%% \n", endpointURL, uptime*100)

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

			for _, collector := range c.metricCollectors {
				for _, metric := range metrics {
					err := collector.Collect(metric)

					if err != nil {
						c.Printf("error %s collecting metric %+v\n", err, metric)
					}
				}

			}
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

	collectors := []collect.Collector{}

	for _, collector := range config.MetricCollectors {
		switch collector {
		case FileMetricCollector:
			fileCollector, err := collect.NewFileCollector(collect.FileCollectorConfig{})

			if err != nil {
				return nil, err
			}

			collectors = append(collectors, fileCollector)
		case CloudwatchMetricCollector:
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

	return &CLI{
		kavaEndpoint:     endpoint,
		Logger:           config.Logger,
		metricCollectors: collectors,
	}, nil
}
