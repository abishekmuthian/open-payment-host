package invoices

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var numberPrefixRE = regexp.MustCompile(`^[A-Za-z0-9/-]{1,15}$`)

// sequenceDigits is a minimum width: sequence values may grow until the complete
// document number reaches GST's 16-character limit. Zero preserves legacy padding.
func (s Settings) sequenceDigits() int {
	if s.SequenceDigits == 0 {
		return 6
	}
	return s.SequenceDigits
}

func (s Settings) validateNumbering() error {
	if s.NumberingStyle != "" && s.NumberingStyle != "financial_year" && s.NumberingStyle != "financial_year_prefix" && s.NumberingStyle != "prefix" {
		return errors.New("select a supported invoice number format")
	}
	if !numberPrefixRE.MatchString(s.Prefix) || !numberPrefixRE.MatchString(s.CreditPrefix) || strings.EqualFold(s.Prefix, s.CreditPrefix) {
		return errors.New("use distinct invoice and credit-note prefixes containing only letters, digits, hyphens or slashes")
	}
	if s.Start < 1 || s.Start > 999999 || s.sequenceDigits() < 1 || s.sequenceDigits() > 6 {
		return errors.New("use a minimum next sequence from 1 to 999999 and sequence padding from 1 to 6 digits")
	}
	return nil
}

func (s Settings) documentNumber(kind string, sequence int64, at time.Time) (string, error) {
	if err := s.validateNumbering(); err != nil {
		return "", err
	}
	if sequence < 1 || sequence > 999999 {
		return "", errors.New("invoice sequence exhausted")
	}
	prefix := s.Prefix
	if kind == "credit_note" {
		prefix = s.CreditPrefix
	}
	switch s.NumberingStyle {
	case "financial_year_prefix":
		prefix = fmt.Sprintf("%04d-%s-", financialYearStart(at), prefix)
	case "prefix":
		// The configured prefix includes any desired year and separators.
	default:
		prefix += "/" + FinancialYear(at) + "/"
	}
	number := prefix + fmt.Sprintf("%0*d", s.sequenceDigits(), sequence)
	if len(number) > 16 {
		return "", errors.New("invoice and credit-note numbers must not exceed 16 characters; shorten the prefix or reduce sequence padding")
	}
	return number, nil
}
