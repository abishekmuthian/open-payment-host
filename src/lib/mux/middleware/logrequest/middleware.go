package logrequest

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fragmenta/mux/log"
)

// TargetResponseTime sets the threshold for colorisation of response times
var TargetResponseTime = 1 * time.Second

// hostname is set on startup to the current host
var hostname string

func init() {
	// Load the hostname if possible
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
}

// Middleware logs after each request to record to log.Printf
// the method, the url, the status code and the response time
// e.g. GET / -> status 200 in 31.932146ms
// With coloration to indicate status and response time
// If ValueLoggers are set the values are also sent to log.Values
func Middleware(h http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		// Store the time prior to handling
		start := time.Now()

		// Wrap the response writer to record code
		// Ideally we'd instead take mux.HandlerFunc
		cw := newCodeResponseWriter(w)

		// Run the handler with our recording response writer
		h(cw, r)

		// Calculate method, url, code, response time
		method := r.Method
		url := r.URL.Path
		duration := time.Now().UTC().Sub(start)
		code := cw.StatusCode

		// Skip logging assets, favicon
		if strings.HasPrefix(url, "/assets") || strings.HasPrefix(url, "/favicon.ico") || strings.HasPrefix(url, "/icons") {
			return
		}

		// Pretty print to the standard loggers colorized
		logWithColor(method, url, code, duration)

		// Log the values to any value loggers (for export to monitoring services)
		values := map[string]interface{}{
			log.SeriesName: "requests",
			"host":         hostname,
			"method":       r.Method,
			"url":          r.URL.Path,
			"code":         code,
			"bot":          isBot(r),
			"duration":     duration.Nanoseconds(), // Store duration in nanoseconds in the db
		}
		log.Values(values)
	}

}

// MiddlewarePrint logs after each request to record to log.Printf
// the method, the url, the status code and the response time
// e.g. GET / -> status 200 in 31.932146ms
// With coloration to indicate status and response time
// No data is sent to value loggers
func MiddlewarePrint(h http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		// Store the time prior to handling
		start := time.Now()

		// Wrap the response writer to record code
		// Ideally we'd instead take mux.HandlerFunc
		cw := newCodeResponseWriter(w)

		// Run the handler with our recording response writer
		h(cw, r)

		// Calculate method, url, code, response time
		method := r.Method
		url := r.URL.Path
		duration := time.Now().UTC().Sub(start)
		code := cw.StatusCode

		// Skip logging assets, favicon
		if strings.HasPrefix(url, "/assets") || strings.HasPrefix(url, "/favicon.ico") || strings.HasPrefix(url, "/icons") {
			return
		}

		// Pretty print to the standard loggers colorized
		logWithColor(method, url, code, duration)
	}
}

// isBot returns true if it thinks this request came from a bot
// At present this is just a simplistic look at the user agent
// for keywords. It must be fast so as not to impact performance.
func isBot(r *http.Request) bool {
	ua := strings.ToLower(r.UserAgent())
	if strings.Contains(ua, "bot") || strings.Contains(ua, "crawl") || strings.Contains(ua, "spider") || strings.Contains(ua, "fetch") {
		return true
	}
	return false
}

// codeResponseWriter defines a responseWriter which stores the status code
type codeResponseWriter struct {
	http.ResponseWriter
	StatusCode int
}

// WriteHeader stores the code before writing
func (cw *codeResponseWriter) WriteHeader(code int) {
	cw.StatusCode = code
	cw.ResponseWriter.WriteHeader(code)
}

// newCodeResponseWriter initialises a codeResponseWriter
func newCodeResponseWriter(w http.ResponseWriter) *codeResponseWriter {
	return &codeResponseWriter{w, http.StatusOK}
}

// Format a string by wrapping in a given color code
func applyColor(f, s string) string {
	return f + s + log.ColorNone
}

// logWithColor formats the log string with color depending on the arguments
func logWithColor(method string, url string, code int, duration time.Duration) {

	// Start with all green, colorise output depending on values
	m := log.ColorGreen
	c := log.ColorGreen
	d := log.ColorGreen

	// Only GET is green
	if method != http.MethodGet {
		m = log.ColorAmber
	}

	// Only 200 is green
	switch code {
	case http.StatusOK:
		c = log.ColorGreen
	case http.StatusMovedPermanently:
		c = log.ColorAmber
	case http.StatusFound:
		c = log.ColorAmber
	default:
		c = log.ColorRed
	}

	// Only under TargetResponseTime is green
	if duration > TargetResponseTime {
		d = log.ColorRed
	}

	// Generate a format string using colors to wrap formats for values
	// The equivalent of the plain format "%s %s -> %d in %s"
	format := fmt.Sprintf("%s %%s %s %s in %s", applyColor(m, "%s"), applyColor(log.ColorCyan, "->"), applyColor(c, "%d"), applyColor(d, "%s"))

	// Print to the log with this colorised format
	log.Printf(format, method, url, code, duration)
}
