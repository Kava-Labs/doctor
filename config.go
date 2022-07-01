// config.go contains types, functions and methods for finding
// reading, and setting configuration values used to run the doctor program

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	DefaultDoctorConfigDirectory          = "~/.kava/doctor"
	DoctorConfigEnvironmentVariablePrefix = "DOCTOR"
	ConfigFilepathFlagName                = "config-filepath"
)

var (
	// cli flags
	// note the majority of configuration values are
	// parsed from a json file and/or environment variables
	configFilepathFlag  = flag.String(ConfigFilepathFlagName, "~/.kava/doctor/config.json", "filepath to json config file to use for running doctor")
	debugModeFlag       = flag.Bool("debug", false, "controls whether debug logging is enabled")
	interactiveModeFlag = flag.Bool("interactive", false, "controls whether an interactive terminal UI is displayed")
)

// DoctorConfig wraps values used to configure
// the execution of the doctor program
type DoctorConfig struct {
	KavaNodeRPCURL  string
	InteractiveMode bool
	DebugMode       bool
	Logger          *log.Logger
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
	logger.Printf("doctor raw config %+v", viper.AllSettings())

	return &DoctorConfig{
		InteractiveMode: viper.GetBool("interactive"),
		KavaNodeRPCURL:  viper.GetString("KAVA_RPC_URL"),
		DebugMode:       debugMode,
		Logger:          logger,
	}, nil
}
