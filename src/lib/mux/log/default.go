package log

import (
	"fmt"
	"io"
	"os"
	"time"
)

// Default defines a default logger to stdout
type Default struct {

	// PrefixTimeFormat is used to prefix any log lines emitted with a time.
	PrefixTimeFormat string

	// Writer is the output of this logger.
	Writer io.Writer
}

// Printf prints the format to writer using args and a time prefix
func (d *Default) Printf(format string, args ...interface{}) {
	if d.PrefixTimeFormat != "" {
		d.WriteString(time.Now().UTC().Format(d.PrefixTimeFormat))
	}

	d.WriteString(fmt.Sprintf(format, args...))
	d.WriteString("\n")
}

// WriteString writes the string to the Writer.
func (d *Default) WriteString(s string) {
	d.Writer.Write([]byte(s))
}

// NewStdErr returns a new PrintLogger which outputs to stderr
func NewStdErr() (*Default, error) {
	d := &Default{
		PrefixTimeFormat: PrefixDateTime,
		Writer:           os.Stderr,
	}
	return d, nil
}
