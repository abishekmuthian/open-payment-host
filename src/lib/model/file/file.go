package file

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Save the file represented by io.Reader to disk at path
func Save(r io.Reader, path string) error {

	// Write out to the desired file path
	w, err := os.Create(path)
	if err != nil {
		fmt.Printf("Error - %s", err)
	}
	defer w.Close()

	_, err = io.Copy(w, r)
	return err

}

// CreatePathTo creates all directories enclosing this file path (which may not yet exist)
func CreatePathTo(s string) error {

	if len(s) == 0 {
		return errors.New("Null path")
	}

	// Ignore the end of path, which is assumed to be a file
	s = filepath.Dir(s)
	s = filepath.Clean(s)

	//	fmt.Printf("Creating dirs to path %s\n", s)

	// Create all directories up to path
	return os.MkdirAll(s, 0774)
}

// Exists returns true if this file exists and can be accessed
// for more complex requirements use os.Stat, this is a shortcut
func Exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil // only return true if no error
}
