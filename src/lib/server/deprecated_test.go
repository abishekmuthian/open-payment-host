package server

import (
	"fmt"
	"testing"
)

type recordingLogger struct {
	lines []string
}

func (l *recordingLogger) Printf(format string, args ...interface{}) {
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func TestLogDoesNotFormatMessage(t *testing.T) {
	logger := &recordingLogger{}
	s := &Server{Logger: logger}

	s.Log("50% done")

	if len(logger.lines) != 1 || logger.lines[0] != "50% done" {
		t.Fatalf("Log wrote %q, want %q", logger.lines, "50% done")
	}
}
