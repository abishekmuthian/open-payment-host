// Package influx sends values to an influxdb database
package influx

import (
	"fmt"
	"strings"
	"time"

	"github.com/fragmenta/mux/log"

	"github.com/influxdata/influxdb/client/v2"
)

// Usage
// l,err := influx.New(influx.Config{})
// log.AddValueLogger(l)
// ...
// log.Values(map[string]interface{}{"key",value})

// Config represents the config for an influx.Logger instance
type Config struct {
	Host         string        // The influxdb hostname
	Database     string        // The influxdb database name
	User         string        // The influxdb user name
	Password     string        // The influxdb user password
	WriteTimeout time.Duration // Timeout for influxdb writes
}

// New returns a new influx db logger
func New(config Config) (log.ValuesLogger, error) {
	// Set a default timeout if none set
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 30 * time.Second
	}

	// Create a new client with these credentials
	clientConfig := client.HTTPConfig{
		Addr:     config.Host,
		Username: config.User,
		Password: config.Password,
		Timeout:  config.WriteTimeout,
	}

	client, err := client.NewHTTPClient(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("stats: error creating client:%s", err)
	}

	// Pass the config and client to a new logger
	l := &Logger{
		config:    config,
		client:    client,
		errLogger: StdErrLogger{},
	}
	return l, nil
}

// Logger logs values to a specified influxdb database
type Logger struct {
	// Config stores the configuration for connections
	config Config
	// Client is used for connections to the database
	client client.Client

	errLogger log.PrintLogger
}

// Values sends a single set of values to influxdb as a batch
func (l *Logger) Values(values map[string]interface{}) {
	l.ValuesBatch([]map[string]interface{}{values})
}

// ValuesBatch sends multiple set of values to influxdb as a batch
func (l *Logger) ValuesBatch(valuesArray []map[string]interface{}) {

	// Create a batch to accumulate points
	points, err := l.CreateBatch()
	if err != nil {
		l.errLogger.Printf("log values: error creating batch:%s", err)
	}

	// Create a point for each entry in values and add to batch
	for _, values := range valuesArray {
		point, err := l.CreatePoint(values)
		if err != nil {
			l.errLogger.Printf("log values: error creating batch:%s", err)
		}
		points.AddPoint(point)
	}

	// Write the batch
	l.WriteBatch(points)
}

// WriteBatch writes the points to the influxdb connection
// within a goroutine to avoid blockng the caller
func (l *Logger) WriteBatch(points client.BatchPoints) {
	// Always perform requests in a goroutine to avoid blocking caller
	go func() {
		// Call client.Write to send the points over the wire
		err := l.client.Write(points)
		if err != nil {
			l.errLogger.Printf("log values: error writing batch:%s", err)
		}
	}()
}

// CreatePoint creates a batch point for influx from a set of values
// If you need more control you can create points and use WriteBatch
func (l *Logger) CreatePoint(values map[string]interface{}) (*client.Point, error) {

	// Read the bucket value from the key log.SeriesName (stats_series_name)
	// and if present remove that key from values
	bucketName := "data"
	_, ok := values[log.SeriesName]
	if ok {
		bucketName, ok = values[log.SeriesName].(string)
		if !ok {
			l.errLogger.Printf("log values: error - bucket name is not a string")
		}
		// Remove the key so we don't send it as a field
		delete(values, log.SeriesName)
	}

	// Set the default time to now, this can be overriden by using log.KeyNameTime
	t := time.Now().UTC()

	_, ok = values[log.KeyNameTime]
	if ok {
		t, ok = values[log.KeyNameTime].(time.Time)
		if !ok {
			l.errLogger.Printf("log values: error - time value is not a time")
		}
		// Remove the key so we don't send it as a field
		delete(values, log.KeyNameTime)
	}

	// By default tags are empty, tags may be set using log.AddTag
	// they are then removed from values and added to tags instead
	tags := map[string]string{}

	for k, v := range values {
		if strings.HasPrefix(k, log.TagPrefix) {
			s, ok := v.(string)
			if ok {
				delete(values, k)
				key := strings.Replace(k, log.TagPrefix, "", 1)
				tags[key] = s
			} else {
				l.errLogger.Printf("log values: error - tag value is not a string")
			}
		}

	}

	// Create a point and add it to the batch
	return client.NewPoint(bucketName, tags, values, t)
}

// CreateBatch creates a batch attached to the db to add points to
func (l *Logger) CreateBatch() (client.BatchPoints, error) {

	// We do not set the RetentionPolicy by default to avoid losing data
	batchPointsConfig := client.BatchPointsConfig{
		Database:  l.config.Database,
		Precision: "ms",
	}

	// Create a new point batch
	points, err := client.NewBatchPoints(batchPointsConfig)
	if err != nil {
		return nil, err
	}

	return points, nil
}

// SetErrorLogger sets the error logger for this influx.Logger
func (l *Logger) SetErrorLogger(errLogger log.PrintLogger) {
	l.errLogger = errLogger
}

// StdErrLogger prints to stdout using fmt.Printf
// and is used as the default error logger for stats errors
type StdErrLogger struct{}

// Printf prints to stdout using fmt.Printf
func (l StdErrLogger) Printf(f string, args ...interface{}) {
	fmt.Printf(f, args...)
}
