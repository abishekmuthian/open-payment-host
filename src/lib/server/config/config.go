// Package config offers utilities for parsing a json config file.
// Values are read as strings, and can be fetched with Get, GetInt or GetBool.
// The caller is expected to parse them for more complex types.
package config

import (
	"os"
	"strconv"
)

const (
	// DefaultPath is where our config is normally found for fragmenta apps.
	DefaultPath = "data/secrets/fragmenta.json"
)

// Config modes are set when creating a new config
const (
	ModeDevelopment = iota
	ModeProduction
	ModeTest
)

// Current is the current configuration object for
var Current *Config

// Config represents a set of key/value pairs for each mode of the app,
// production, development and test. Which set of values is used
// is set by Mode.
type Config struct {
	Mode int
}

// New returns a new config, which defaults to development
func New() *Config {
	return &Config{
		Mode: ModeDevelopment,
	}
}

// Load our json config file from the path
func (c *Config) Load() {

	mode := os.Getenv("FRAG_ENV")

	switch mode {
	case "production":
		c.Mode = ModeProduction
	case "test":
		c.Mode = ModeTest
	default:
		c.Mode = ModeDevelopment
	}

}

// Production returns true if current config is production.
func (c *Config) Production() bool {
	return c.Mode == ModeProduction
}

// Configuration returns all the configuration key/values for a given mode.
func (c *Config) Configuration() []string {
	// For simplicity, we'll return all environment variables
	return os.Environ()
}

// Get returns a specific value or "" if no value
func (c *Config) Get(key string) string {
	return os.Getenv(key)
}

// GetInt returns the current configuration value as int64, or 0 if no value
func (c *Config) GetInt(key string) int64 {
	v := c.Get(key)
	if v != "" {
		i, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return i
		}
	}
	return 0
}

// GetBool returns the current configuration value as bool
// (yes=true, no=false), or false if no value
func (c *Config) GetBool(key string) bool {
	v := c.Get(key)
	return v == "yes"
}

// Config (Get) returns a specific value or "" if no value
// For compatability with older server config, we wrap this function
// Deprecated
func (c *Config) Config(key string) string {
	return c.Get(key)
}

// These convenience functions wrap the Current pkg global

// Production returns true if current config is production.
func Production() bool {
	return Current.Production()
}

// Configuration returns all the configuration key/values for a given mode.
func Configuration(m int) []string {
	return Current.Configuration()
}

// Get returns a specific value or "" if no value
func Get(key string) string {
	return Current.Get(key)
}

// GetInt returns the current configuration value as int64, or 0 if no value
func GetInt(key string) int64 {
	return Current.GetInt(key)
}

// GetBool returns the current configuration value as bool
// (yes=true, no=false), or false if no value
func GetBool(key string) bool {
	return Current.GetBool(key)
}
