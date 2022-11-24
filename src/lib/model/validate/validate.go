// Package validate provides methods for validating params passed from the database row as interface{} types
package validate

import (
	"fmt"
	"strconv"
	"time"
)

// Float returns the float value of param or 0.0
func Float(param interface{}) float64 {
	var v float64
	if param != nil {
		switch param.(type) {
		case int64:
			v = float64(param.(int64))
		default:
			v = param.(float64)
		}
	}
	return v
}

// Boolean returns the bool value of param or false
func Boolean(param interface{}) bool {
	var v bool
	if param != nil {
		v = param.(bool)
	}
	return v
}

// Int returns the int value of param or 0
func Int(param interface{}) int64 {
	var v int64
	if param != nil {
		switch param.(type) {
		case float64:
			v = int64(param.(float64))
		case int:
			v = int64(param.(int))
		default:
			v = param.(int64)
		}
	}
	return v
}

// String returns the string value of param or ""
func String(param interface{}) string {
	var v string
	if param != nil {
		v = param.(string)
	}
	return v
}

// Time returns the time value of param or the zero value of time.Time
func Time(param interface{}) time.Time {
	var v time.Time
	if param != nil {
		switch param.(type) {
		case time.Time:
			v = param.(time.Time)
		case string:
			// Attempt to parse the time in RFC3339 format
			d, err := time.Parse(time.RFC3339, param.(string))
			if err != nil {
				return v // return zero time on failure
			}
			v = d
		}

	}
	return v
}

// Length validates a param by min and max length
func Length(param string, min int, max int) error {
	length := len(param)
	if min != -1 && length < min {
		return fmt.Errorf("length of string %s %d, expected > %d", param, length, min)
	}
	if max != -1 && length > max {
		return fmt.Errorf("length of string %s %d, expected < %d", param, length, max)
	}
	return nil
}

// Within returns true if the param is an int with value between min and max inclusive
// Set min or max to -1 to ignore
func Within(param string, min float64, max float64) error {
	f, err := strconv.ParseFloat(param, 64)
	if err != nil {
		return fmt.Errorf("invalid float param %s", param)
	}
	if f < min {
		return fmt.Errorf("%0.2f is less than minimum %0.2f", f, min)
	}
	if f > max {
		return fmt.Errorf("%0.2f is more than maximum %0.2f", f, max)
	}
	return nil
}
