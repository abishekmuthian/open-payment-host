// Package schedule provides a simple way to schedule functions at a time or interval
package schedule

// Logger Interface for a simple logger (the stdlib log pkg and the fragmenta log pkg conform)
type Logger interface {
	Printf(format string, args ...interface{})
}

// Config Interface to retreive configuration details of the server
type Config interface {
	Production() bool
	Config(string) string
}

// ActionContext is the scheduled action context,
// a simplified version of a web request context
type ActionContext struct {

	// The context log passed from router
	logger Logger

	// The app config usually loaded from fragmenta.json
	config Config

	// Arbitrary user data stored in a map
	data map[string]interface{}
}

// NewContext returns a new context initialised with the given interfaces
func NewContext(l Logger, c Config) *ActionContext {
	return &ActionContext{
		logger: l,
		config: c,
		data:   make(map[string]interface{}),
	}
}

// Logf logs the given message and arguments using our logger
func (c *ActionContext) Logf(format string, v ...interface{}) {
	c.logger.Printf(format, v...)
}

// Log logs the given message using our logger
func (c *ActionContext) Log(message string) {
	c.Logf(message)
}

// Config returns a key from the context config
func (c *ActionContext) Config(key string) string {
	return c.config.Config(key)
}

// Production returns whether this context is running in production
func (c *ActionContext) Production() bool {
	return c.config.Production()
}

// Set saves arbitrary data for this request
func (c *ActionContext) Set(key string, data interface{}) {
	c.data[key] = data
}

// Get retreives arbitrary data for this request
func (c *ActionContext) Get(key string) interface{} {
	return c.data[key]
}
