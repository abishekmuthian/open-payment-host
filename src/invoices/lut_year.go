package invoices

import (
	"errors"
	"strconv"
	"strings"
)

// normaliseLUTYear accepts the readable year and legacy compact values. Keep
// storage compact so frozen snapshots, eligibility checks and counters agree.
func normaliseLUTYear(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	invalid := errors.New("enter a consecutive LUT financial year such as 2026-27 (starting year 2000-2099)")
	if len(value) == 7 && strings.HasPrefix(value, "20") && value[4] == '-' {
		value = value[2:4] + value[5:7]
	}
	if len(value) != 4 {
		return "", invalid
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return "", invalid
		}
	}
	start, _ := strconv.Atoi(value[:2])
	end, _ := strconv.Atoi(value[2:])
	if end != (start+1)%100 {
		return "", invalid
	}
	return value, nil
}

// LUTYearDisplay formats a transient copy for settings and newly rendered PDFs.
// Leave invalid legacy data visible for correction instead of inventing a year.
func (s Settings) LUTYearDisplay() string {
	year, err := normaliseLUTYear(s.LUTYear)
	if err != nil {
		return s.LUTYear
	}
	if year == "" {
		return ""
	}
	return "20" + year[:2] + "-" + year[2:]
}
