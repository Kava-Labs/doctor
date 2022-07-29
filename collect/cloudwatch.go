package collect

import (
	"context"

	"github.com/kava-labs/doctor/metric"

	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	awsTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
)

// CloudWatchCollectorConfig wraps values
// for configuring a CloudWatch
type CloudWatchCollectorConfig struct {
	Ctx             context.Context
	AWSRegion       string
	MetricNamespace string
}

// CloudWatchCollector implements the Collector interface,
// collecting metrics to a file
type CloudWatchCollector struct {
	cloudwatchClient *cloudwatch.Client
	ctx              context.Context
	metricNamespace  string
}

// NewCloudWatchCollector attempts to create a new CloudWatchCollector
// using the specified config
// returning the CloudWatchCollector and error (if any)
func NewCloudWatchCollector(config CloudWatchCollectorConfig) (*CloudWatchCollector, error) {
	// Using the SDK's default configuration, loading additional config
	// and credentials values from the environment variables, shared
	// credentials, and shared configuration files
	cfg, err := awsConfig.LoadDefaultConfig(config.Ctx,
		awsConfig.WithRegion(config.AWSRegion),
	)
	if err != nil {
		return nil, err
	}

	cloudwatchClient := cloudwatch.NewFromConfig(cfg)

	return &CloudWatchCollector{
		ctx:              config.Ctx,
		cloudwatchClient: cloudwatchClient,
		metricNamespace:  config.MetricNamespace,
	}, nil
}

// Collect collects metric to CloudWatch returning error (if any)
// (rotation is only triggered when a metric is collection)
// Collect is safe to call across go-routines
func (cwc *CloudWatchCollector) Collect(metric metric.Metric) error {
	if !metric.CollectToCloudwatch {
		// no-op
		return nil
	}

	// encode metric to AWS format
	awsDimensions := []awsTypes.Dimension{}
	for key, value := range metric.Dimensions {
		awsDimensions = append(awsDimensions, awsTypes.Dimension{
			Name:  &key,
			Value: &value,
		})
	}

	_, err := cwc.cloudwatchClient.PutMetricData(cwc.ctx, &cloudwatch.PutMetricDataInput{
		Namespace: aws.String(cwc.metricNamespace),
		MetricData: []awsTypes.MetricDatum{
			{
				MetricName: &metric.Name,
				Dimensions: awsDimensions,
				Timestamp:  &metric.Timestamp,
				Value:      &metric.Value,
				Unit:       awsTypes.StandardUnitNone,
			},
		},
	})

	if err != nil {
		return err
	}

	return nil
}
