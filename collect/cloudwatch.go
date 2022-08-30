package collect

import (
	"context"

	"github.com/kava-labs/doctor/metric"

	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	awsTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
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
	awsInstanceId    string
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

	var awsInstanceId string

	awsSession, err := session.NewSession()

	if err != nil {
		return nil, err
	}

	ec2MetadataClient := ec2metadata.New(awsSession)

	nodeEC2IdentityDocument, err := ec2MetadataClient.GetInstanceIdentityDocument()

	if err != nil {
		// fail gracefully if doctor is running in a non-aws environment
		// e.g. a laptop
		awsInstanceId = ""
	} else {
		awsInstanceId = nodeEC2IdentityDocument.InstanceID
	}

	return &CloudWatchCollector{
		ctx:              config.Ctx,
		cloudwatchClient: cloudwatchClient,
		metricNamespace:  config.MetricNamespace,
		awsInstanceId:    awsInstanceId,
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

	if cwc.awsInstanceId != "" {
		awsDimensions = append(awsDimensions, awsTypes.Dimension{
			Name:  aws.String("instance-id"),
			Value: &cwc.awsInstanceId,
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
