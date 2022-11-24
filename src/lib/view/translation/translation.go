// Package translation provides a simple in-memory translation service - it may soon be renamed translate
package translation

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

// DefaultLanguage defines a default language to fall back to
var DefaultLanguage = "en"

// data holds the translated data in memory (at present not split by language)
var data map[string]string

// mu guards the translations during load and access
var mu sync.RWMutex

var setupComplete bool

// Setup sets up the initial map
func Setup() error {
	mu.Lock()
	defer mu.Unlock()
	data = make(map[string]string)
	setupComplete = true
	return nil
}

// Load scans the path for translation lang.json files
// if called twice the second translations will overwrite any common keys
func Load(root string) error {
	// Ensure setup was called
	if !setupComplete {
		err := Setup()
		if err != nil {
			return err
		}
	}

	// Lock data for load operation
	mu.Lock()
	defer mu.Unlock()

	// Scan the files in the given dir, looking for our suffix
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {

		if err != nil {
			return err
		}

		// Deal with files, directories we return nil error to recurse on them
		if canParseFile(p) {
			err = parseFile(p)
			if err != nil {
				return err
			}
		}

		return nil
	})

	return err
}

// Get returns the translation for a given language and key
// if no result, it falls back to DefaultLanguage + key
func Get(lang, key string) string {
	mu.RLock()
	defer mu.RUnlock()

	// First try the language specified
	t := data[lang+key]
	if t != "" {
		return t
	}

	// Fall back to default language if no result (in our case english)
	t = data[DefaultLanguage+key]
	if t != "" {
		return t
	}

	// If still no result, return key
	return key
}

// canParseFile returns true if we can parse this file
func canParseFile(p string) bool {
	return !strings.HasPrefix(p, ".") && strings.HasSuffix(p, ".lang.json")
}

// parseFile opens the file and fills in our translations,
// returning error if a project is encountered.
func parseFile(p string) error {

	// For each file, load all strings in the json file,
	//  and add them to our list of translations
	file, err := ioutil.ReadFile(p)
	if err != nil {
		return fmt.Errorf("Error opening file %s %v", p, err)
	}

	var langData map[string]string
	err = json.Unmarshal(file, &langData)
	if err != nil {
		return fmt.Errorf("Error reading language file %s %v", p, err)
	}

	lang := path.Base(p)
	lang = strings.Replace(lang, ".lang.json", "", -1)

	for k, v := range langData {
		data[lang+k] = v
	}

	return nil
}
