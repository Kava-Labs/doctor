// config.go contains types, functions and methods for finding
// reading, and setting configuration values used to run the doctor program

package config

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	DefaultDoctorConfigDirectory          = "~/.kava/doctor"
	DoctorConfigEnvironmentVariablePrefix = "DOCTOR"
	// use snake_casing to match json or
	// environment variable provided configuration
	ConfigFilepathFlagName                             = "config_filepath"
	DefaultMonitoringIntervalSecondsFlagName           = "default_monitoring_interval_seconds"
	KavaAPIAddressFlagName                             = "kava_api_address"
	MaxMetricSamplesToRetainPerNodeFlagName            = "max_metric_samples_to_retain_per_node"
	MetricSamplesForSyntheticMetricCalculationFlagName = "metric_samples_to_use_for_synthetic_metrics"
	MetricCollectorsFlagName                           = "metric_collectors"
	DefaultMetricCollector                             = "file"
	FileMetricCollector                                = "file"
	CloudwatchMetricCollector                          = "cloudwatch"
	AWSRegionFlagName                                  = "aws_region"
	MetricNamespaceFlagName                            = "metric_namespace"
	AutohealFlagName                                   = "autoheal"
	AutohealSyncLatencyToleranceSecondsFlagName        = "autoheal_sync_latency_tolerance_seconds"
	AutohealSyncToLiveToleranceSecondsFlagName         = "autoheal_sync_to_live_tolerance_seconds"
	DowntimeRestartThresholdSecondsFlagName            = "downtime_restart_threshold_seconds"
	// 5 minutes
	DefaultDowntimeRestartThresholdSeconds     = 300
	NoNewBlocksRestartThresholdSecondsFlagName = "no_new_blocks_restart_threshold_seconds"
	// 5 minutes
	DefaultNoNewBlocksRestartThresholdSeconds = 300
	AutohealChecksStartupDelaySecondsFlagName = "autoheal_checks_startup_delay_seconds"
	// 45 minutes
	DefaultAutohealChecksStartupDelaySecondsFlag = 2700
	HealthChecksTimeoutSecondsFlagName           = "health_check_timeout_seconds"
	DefaultHealthChecksTimeoutSecondsFlagName    = 10
	AutohealRestartDelaySecondsFlagName          = "autoheal_restart_delay_seconds"
	// 45 minutes
	DefaultAutohealRestartDelaySeconds = 2700
)

const (
	DefaultMetricSamplesToKeepPerNode                 = 10000
	DefaultMetricSamplesForSyntheticMetricCalculation = 60
)

var (
	ValidMetricCollectors = []string{
		FileMetricCollector,
		CloudwatchMetricCollector,
	}
	// cli flags
	// while the majority of time configuration values will be
	// parsed from a json file and/or environment variables
	// specifying these allows setting default values and
	// auto populates help text in the output of --help
	configFilepathFlag                             = flag.String(ConfigFilepathFlagName, "~/.kava/doctor/config.json", "filepath to json config file to use")
	kavaAPIAddressFlag                             = flag.String(KavaAPIAddressFlagName, "https://rpc.data.kava.io", "URL of the endpoint that doctor should monitor")
	debugModeFlag                                  = flag.Bool("debug", false, "controls whether debug logging is enabled")
	interactiveModeFlag                            = flag.Bool("interactive", false, "controls whether an interactive terminal UI is displayed")
	defaultMonitoringIntervalSecondsFlag           = flag.Int(DefaultMonitoringIntervalSecondsFlagName, 5, "default interval doctor will use for the various monitoring routines")
	maxMetricSamplesToRetainPerNodeFlag            = flag.Int(MaxMetricSamplesToRetainPerNodeFlagName, DefaultMetricSamplesToKeepPerNode, "maximum number of metric samples that will be kept in memory per node")
	metricSamplesForSyntheticMetricCalculationFlag = flag.Int(MetricSamplesForSyntheticMetricCalculationFlagName, DefaultMetricSamplesForSyntheticMetricCalculation, "number of metric samples to use when calculating synthetic metrics such as the node hash rate")
	metricCollectorsFlag                           = flag.String(MetricCollectorsFlagName, DefaultMetricCollector, fmt.Sprintf("where to send collected metrics to, multiple collectors can be specified as a comma separated list, supported collectors are %v", ValidMetricCollectors))
	awsRegionFlag                                  = flag.String(AWSRegionFlagName, "us-east-1", "aws region to use for sending metrics to CloudWatch")
	metricNamespaceFlag                            = flag.String(MetricNamespaceFlagName, "kava", "top level namespace to use for grouping all metrics sent to cloudwatch")
	autohealFlag                                   = flag.Bool(AutohealFlagName, false, "whether doctor should take active measures to attempt to heal the kava process (e.g. place on standby if it falls significantly behind live)")
	autohealSyncLatencyToleranceSecondsFlag        = flag.Int(AutohealSyncLatencyToleranceSecondsFlagName, 120, "how far behind live the node is allowed to fall before autohealing actions are attempted")
	autohealSyncToLiveToleranceSecondsFlag         = flag.Int(AutohealSyncToLiveToleranceSecondsFlagName, 12, "how close to the current time the node must resync to before being considered in sync again")
	downtimeRestartThresholdSecondsFlag            = flag.Int(DowntimeRestartThresholdSecondsFlagName, DefaultDowntimeRestartThresholdSeconds, "how many continuous seconds the endpoint being monitored has to be offline or unresponsive before autohealing will be attempted")
	noNewBlocksRestartThresholdSecondsFlag         = flag.Int(NoNewBlocksRestartThresholdSecondsFlagName, DefaultNoNewBlocksRestartThresholdSeconds, "how many continuous seconds the endpoint being monitored has not produce a new bloc before autohealing will be attempted")
	healthChecksTimeoutSecondsFlag                 = flag.Int(HealthChecksTimeoutSecondsFlagName, DefaultHealthChecksTimeoutSecondsFlagName, "max number of seconds doctor will wait for a health check response from the endpoint")
	autohealRestartDelaySecondsFlag                = flag.Int(AutohealRestartDelaySecondsFlagName, DefaultAutohealRestartDelaySeconds, fmt.Sprintf("number of seconds autohealing routines will wait to restart the endpoint, effective from the last time it was restarted and over riding the values %s %s", DowntimeRestartThresholdSecondsFlagName, NoNewBlocksRestartThresholdSecondsFlagName))
)

// DoctorConfig wraps values used to configure
// the execution of the doctor program
type DoctorConfig struct {
	KavaNodeRPCURL                             string
	InteractiveMode                            bool
	DebugMode                                  bool
	DefaultMonitoringIntervalSeconds           int
	MaxMetricSamplesToRetainPerNode            int
	MetricSamplesForSyntheticMetricCalculation int
	MetricCollectors                           []string
	AWSRegion                                  string
	MetricNamespace                            string
	Logger                                     *log.Logger
	Autoheal                                   bool
	AutohealSyncLatencyToleranceSeconds        int
	AutohealSyncToLiveToleranceSeconds         int
	AutohealRestartDelaySeconds                int
	HealthChecksTimeoutSeconds                 int
	NoNewBlocksRestartThresholdSeconds         int
	DowntimeRestartThresholdSeconds            int
}

// GetDoctorConfig gets an instance of DoctorConfig
// populated with values provided via the command line
// environment, and or config files
func GetDoctorConfig() (*DoctorConfig, error) {
	config := &DoctorConfig{}

	// set default configuration settings
	viper.SetEnvPrefix(DoctorConfigEnvironmentVariablePrefix)
	viper.SetConfigType("json")

	// allow viper to merge in config provided via command-line flags
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	// allow for overriding any configuration using environment variables
	// prefixed with `DoctorConfigEnvironmentVariablePrefix`
	viper.AutomaticEnv()

	// get the absolute path to the configuration file
	configFilepath, err := homedir.Expand(viper.GetString(ConfigFilepathFlagName))

	if err != nil {
		return config, fmt.Errorf("error %s trying to expand home directory for path %s", err, *configFilepathFlag)
	}

	// best effort attempt to load a config file
	// but allow running with only command line flags
	// or environment variables
	configFile, err := os.Open(configFilepath)

	if err != nil {
		fmt.Printf("failed to open config file @ %s\n", configFilepath)
	} else {
		err = viper.ReadConfig(configFile)
		if err != nil {
			return config, fmt.Errorf("error %s parsing config file %s", err, configFilepath)
		}
	}

	// setup default logger
	var logger *log.Logger
	debugMode := viper.GetBool("debug")

	if debugMode {
		logger = log.New(os.Stdout, "doctor ", log.LstdFlags|log.Lshortfile)
		logger.Print("debug logging enabled")
	} else {
		// log to dev null
		logger = log.New(ioutil.Discard, "doctor ", log.LstdFlags|log.Lshortfile)
	}

	// there may be more configuration values provided
	// then were parsed above
	logger.Printf("doctor raw config %+v\n", viper.AllSettings())

	// validate requested metric collectors
	// need to manually parse string slice because
	// https://github.com/spf13/viper/issues/380
	requestedCollectors := strings.Split(viper.GetString(MetricCollectorsFlagName), ",")
	validCollectors := []string{}

	for _, requestedCollector := range requestedCollectors {
		for _, validCollector := range ValidMetricCollectors {
			if requestedCollector == validCollector {
				validCollectors = append(validCollectors, requestedCollector)

				break
			}
		}
	}

	// if no valid collector specified default to "file"
	if len(validCollectors) == 0 {
		logger.Printf("no valid collectors %v specified, defaulting to %s\n", requestedCollectors, DefaultMetricCollector)

		validCollectors = append(validCollectors, DefaultMetricCollector)
	}

	return &DoctorConfig{
		InteractiveMode:                  viper.GetBool("interactive"),
		KavaNodeRPCURL:                   viper.GetString(KavaAPIAddressFlagName),
		DefaultMonitoringIntervalSeconds: viper.GetInt(DefaultMonitoringIntervalSecondsFlagName),
		DebugMode:                        debugMode,
		Logger:                           logger,
		MetricCollectors:                 validCollectors,
		MaxMetricSamplesToRetainPerNode:  viper.GetInt(MaxMetricSamplesToRetainPerNodeFlagName),
		MetricSamplesForSyntheticMetricCalculation: viper.GetInt(MetricSamplesForSyntheticMetricCalculationFlagName),
		AWSRegion:                           viper.GetString(AWSRegionFlagName),
		MetricNamespace:                     viper.GetString(MetricNamespaceFlagName),
		Autoheal:                            viper.GetBool(AutohealFlagName),
		AutohealSyncLatencyToleranceSeconds: viper.GetInt(AutohealSyncLatencyToleranceSecondsFlagName),
		AutohealSyncToLiveToleranceSeconds:  viper.GetInt(AutohealSyncToLiveToleranceSecondsFlagName),
		AutohealRestartDelaySeconds:         viper.GetInt(AutohealRestartDelaySecondsFlagName),
		HealthChecksTimeoutSeconds:          viper.GetInt(HealthChecksTimeoutSecondsFlagName),
		NoNewBlocksRestartThresholdSeconds:  viper.GetInt(NoNewBlocksRestartThresholdSecondsFlagName),
		DowntimeRestartThresholdSeconds:     viper.GetInt(DowntimeRestartThresholdSecondsFlagName),
	}, nil
}
