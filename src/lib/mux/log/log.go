// Package log provides logging interfaces for use in handlers
// and loggers for stdout, files, and time series databases
package log

import (
	"time"
)

// Date prefix constants
const (
	// PrefixDate constant for date prefixes
	PrefixDate = "2006-01-02 "
	// PrefixTime  constants for time prefix
	PrefixTime = "15:04:05 "
	// PrefixDateTime  constants for date + time prefix
	PrefixDateTime = "2006-01-02:15:04:05 "
)

// Color constants used for writing in colored output
const (
	ColorNone  = "\033[0m" // Use this to clear output
	ColorRed   = "\033[31m"
	ColorGreen = "\033[32m"
	ColorAmber = "\033[33m"
	ColorCyan  = "\033[1;36m"
)

var (
	// printLogs stores the loggers called by the log.Printf function below.
	printLogs []PrintLogger

	// valueLogs stores the loggers called by the log.Values function below.
	valueLogs []ValuesLogger
)

// Printf prints to the printLogs
func Printf(format string, args ...interface{}) {
	for _, l := range printLogs {
		l.Printf(format, args...)
	}
}

// Timef prints the time elapsed since start plus any message to the PrintLog
// to log time taken by a function, call at start with
// defer log.Timef(start,...).
func Timef(start time.Time, format string, args ...interface{}) {
	ms := time.Now().UTC().Sub(start)
	args = append(args, ms)
	Printf(format+" in %s", args...)
}

// Values values to the valueLogs which typically emit stats to a time series database.
func Values(values map[string]interface{}) {
	for _, l := range valueLogs {
		l.Values(values)
	}
}

// ValuesBatch sends an array of values to the valueLogs which typically emit stats to a time series database.
func ValuesBatch(values []map[string]interface{}) {
	for _, l := range valueLogs {
		l.ValuesBatch(values)
	}
}

// Add adds the given logger to the list of outputs,
// it should be called before logging commences
func Add(l PrintLogger) {
	printLogs = append(printLogs, l)
}

// AddValuesLog adds the given logger to the list of ValuesLoggers,
// it should be called before logging commences
func AddValuesLog(l ValuesLogger) {
	valueLogs = append(valueLogs, l)
}

// PrintLogger defines an interface for logging to a text log.
type PrintLogger interface {
	Printf(format string, args ...interface{})
}

// ValuesLogger defines an interface for logging to a stats service.
type ValuesLogger interface {
	Values(values map[string]interface{})
	ValuesBatch(values []map[string]interface{})
}
