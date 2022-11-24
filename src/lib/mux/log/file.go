package log

import (
	"errors"
	"os"
)

// File logs to a local file as well as stdout
type File struct {
	Default // File embeds default
	Path    string
}

const (
	// FileFlags serts the flags for OpenFile on the log file
	FileFlags = os.O_WRONLY | os.O_APPEND | os.O_CREATE

	// FilePermissions serts the perms for OpenFile on the log file
	FilePermissions = 0640
)

// NewFile creates a new file logger for the given path.
func NewFile(path string) (*File, error) {
	if path == "" {
		return nil, errors.New("log: null file path for file log")
	}

	// Create a writer for the given file
	logFile, err := os.OpenFile(path, FileFlags, FilePermissions)
	if err != nil {
		return nil, err
	}

	f := &File{
		Default: Default{
			PrefixTimeFormat: PrefixDateTime,
			Writer:           logFile,
		},
	}

	return f, nil
}
