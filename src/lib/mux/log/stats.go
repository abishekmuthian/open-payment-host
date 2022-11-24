package log

import (
	"fmt"
)

// SeriesName specifies the key used to define which bucket/table to send values to
// thius is used by adapters to extract the table name from values supplied.
const SeriesName = "stats_series_name"

// KeyNameTime specifies a key used to define the time value for a given key
// the Value should be a time.Time value
const KeyNameTime = "stats_key_name_time"

// TagPrefix for a key name allows adding a tag instead of a field to influx
const TagPrefix = "stats_key_tag_"

// StatsLog conforms to the ValuesLogger interface
// It simply outputs the values to stdout, rather than logging to a specific service
// see the adapters folder for ValueLoggers which connect to time series databases.
type StatsLog struct {
}

// Values prints to the ValuesLog which typically emits stats to a time series database.
// This example logger prints values to stdout instead.
func (l *StatsLog) Values(values map[string]interface{}) {
	fmt.Printf("Values logged:%+s", values)
}

// AddTag adds a tag field to this
func AddTag(values map[string]interface{}, key string, value string) map[string]interface{} {
	values[TagPrefix+key] = value
	return values
}
