package resource

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Methods for validating params passed from the database row as interface{} types
// perhaps this should be a sub-package for clarity?

// ValidateFloat returns the float value of param or 0.0
func ValidateFloat(param interface{}) float64 {
	var v float64
	if param != nil {
		switch param.(type) {
		case float64:
			v = param.(float64)
		case float32:
			v = float64(param.(float32))
		case int:
			v = float64(param.(int))
		case int64:
			v = float64(param.(int64))
		}
	}
	return v
}

// ValidateBoolean returns the bool value of param or false
func ValidateBoolean(param interface{}) bool {
	var v bool
	if param != nil {
		switch param.(type) {
		case bool:
			v = param.(bool)
		}
	}
	return v
}

// ValidateInt returns the int value of param or 0
func ValidateInt(param interface{}) int64 {
	var v int64
	if param != nil {
		switch param.(type) {
		case int64:
			v = param.(int64)
		case float64:
			v = int64(param.(float64))
		case int:
			v = int64(param.(int))
		}
	}
	return v
}

// ValidateInt64Array ValidateIntArray returns the int array value of param or 0
func ValidateInt64Array(param interface{}) []int64 {
	var v []int64
	if param != nil {
		switch param.(type) {
		case string:
			e := param.(string)
			trimmed := strings.Trim(e, "{}")
			strings := strings.Split(trimmed, ",")
			v = make([]int64, len(strings))
			for i, s := range strings {
				v[i], _ = strconv.ParseInt(s, 10, 64)
			}

		}
	}
	return v
}

// ValidateString returns the string value of param or ""
func ValidateString(param interface{}) string {
	var v string
	if param != nil {
		switch param.(type) {
		case string:
			v = param.(string)
		}
	}
	return v
}

// ValidateStringArray returns the string array value of param or 0
func ValidateStringArray(param interface{}) []string {
	var v []string
	if param != nil {
		switch param.(type) {
		case string:
			e := param.(string)
			trimmed := strings.Trim(e, "{}")
			strings := strings.Split(trimmed, ",")
			v = make([]string, len(strings))
			for i, s := range strings {
				v[i] = trimQuote(s)
			}

		}
	}
	return v
}

// trimQuote removes the extra quotes from the string
func trimQuote(s string) string {
	if len(s) > 0 && s[0] == '"' {
		s = s[1:]
	}
	if len(s) > 0 && s[len(s)-1] == '"' {
		s = s[:len(s)-1]
	}
	return s
}

// ValidateTime returns the time value of param or the zero value of time.Time
func ValidateTime(param interface{}) time.Time {
	var v time.Time
	if param != nil {
		switch param.(type) {
		case time.Time:
			v = param.(time.Time)
		case string:
			// Attempt to parse the time in custom format format for SQLite
			d, err := time.Parse("2006-01-02 15:04:05.000 +0000", param.(string))
			if err != nil {
				return v // return zero time on failure
			}
			v = d
		}
	}
	return v
}

// ValidateMap returns the map value of the JSON string
func ValidateMap(param interface{}) map[string]string {
	var v map[string]string
	if param != nil {
		switch param.(type) {
		case string:
			json.Unmarshal([]byte(param.(string)), &v)
		}
	}
	return v
}

// ValidateNestedMap returns the nested map value of the JSON string
func ValidateNestedMap(param interface{}) map[string]map[string]interface{} {
	var v map[string]map[string]interface{}
	if param != nil {
		switch param.(type) {
		case string:
			json.Unmarshal([]byte(param.(string)), &v)
		}
	}
	return v
}
