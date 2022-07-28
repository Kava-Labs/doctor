package collect

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/kava-labs/doctor/metric"
)

const (
	DefaultMetricFileNameSuffix = "doctor-metrics.json"
	DefaultFileRotationInterval = 1 * time.Hour
)

// FileCollectorConfig wraps values
// for configuring a FileCollector
type FileCollectorConfig struct {
	MetricFileNameSuffix string
	FileRotationInterval *time.Duration
}

// FileCollector implements the Collector interface,
// collecting metrics to a file
type FileCollector struct {
	currentFile          *os.File
	currentFileOpenedAt  time.Time
	fileRotationInterval time.Duration
	fileLock             *sync.Mutex
	metricFileNameSuffix string
}

// NewFileCollector attempts to create a new FileCollector
// using the specified config (or default values where appropriate)
// returning the FileCollector and error (if any)
func NewFileCollector(config FileCollectorConfig) (*FileCollector, error) {
	metricFileNameSuffix := DefaultMetricFileNameSuffix

	if config.MetricFileNameSuffix != "" {
		metricFileNameSuffix = config.MetricFileNameSuffix
	}

	fileRotationInterval := DefaultFileRotationInterval

	if config.FileRotationInterval != nil {
		fileRotationInterval = *config.FileRotationInterval
	}

	now := time.Now()

	fileName := fmt.Sprintf("%d-%s", now.Unix(), metricFileNameSuffix)

	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		return nil, err
	}

	return &FileCollector{
		metricFileNameSuffix: metricFileNameSuffix,
		currentFile:          file,
		currentFileOpenedAt:  now,
		fileRotationInterval: fileRotationInterval,
		fileLock:             &sync.Mutex{},
	}, nil
}

// Collect collects metric to a file, returning error (if any)
// Collect will ensure that the file is rotated at most
// `fileRotationInterval`
// (rotation is only triggered when a metric is collection)
// Collect is safe to call across go-routines, and will block
// to ensure an in progress collection completes cleanly before
// a new metric is collected
func (fc *FileCollector) Collect(metric metric.Metric) error {
	// grab the lock
	fc.fileLock.Lock()

	// ensure lock is released
	defer fc.fileLock.Unlock()

	// check if we need to rotate the current file
	if time.Since(fc.currentFileOpenedAt) >= fc.fileRotationInterval {
		fc.rotateFile()
	}

	// encode metric to json
	marshalledMetric, err := json.Marshal(metric)

	if err != nil {
		return err
	}

	// collect the metric
	_, err = fc.currentFile.Write(marshalledMetric)

	if err != nil {
		return err
	}

	return nil
}

// rotateFile attempts to close the current
// collection file and open a new one for use,
// returning error (if any)
func (fc *FileCollector) rotateFile() error {
	now := time.Now()

	fileName := fmt.Sprintf("%d-%s", now.Unix(), fc.metricFileNameSuffix)

	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		return err
	}

	fc.currentFile = file
	fc.currentFileOpenedAt = now

	return nil
}
