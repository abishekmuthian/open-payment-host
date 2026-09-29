package subscriptions

import (
	"errors"
	"fmt"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"net/url"
	"strconv"
	"strings"
)

func BuildRedirectURL(base string, params map[string]string) string {
	u, err := url.Parse(base)
	if err != nil {
		return ""
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func redirectOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Opaque != "" || u.Hostname() == "" || strings.ContainsAny(u.Host, "\\\r\n\t ") || (u.Scheme != "https" && u.Scheme != "http") {
		return "", errors.New("invalid redirect URL")
	}
	if config.Production() && u.Scheme != "https" {
		return "", errors.New("HTTPS redirect required")
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), nil
}
func ValidateRedirectURI(p *products.Story, raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if p == nil {
		return "", errors.New("missing product")
	}
	origin, err := redirectOrigin(raw)
	if err != nil {
		return "", err
	}
	allowed := strings.FieldsFunc(p.AllowedRedirectOrigins, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
	if p.WebhookURL != "" {
		allowed = append(allowed, p.WebhookURL)
	}
	for _, a := range allowed {
		o, e := redirectOrigin(strings.TrimSpace(a))
		if e == nil && o == origin {
			return raw, nil
		}
	}
	return "", errors.New("redirect origin is not configured for this product")
}
func currencyMinorDecimals(currency string) int {
	switch strings.ToUpper(currency) {
	case "BHD", "JOD", "KWD", "OMR", "TND":
		return 3
	case "JPY", "KRW", "VND", "CLP", "KMF", "XAF", "XOF", "XPF", "BIF", "DJF", "GNF", "PYG", "RWF", "UGX", "VUV":
		return 0
	default:
		return 2
	}
}
func majorValueToMinor(value, currency string) (int64, error) {
	if len(currency) != 3 || value == "" {
		return 0, errors.New("missing amount or currency")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, errors.New("invalid decimal amount")
	}
	digits := parts[0]
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if fraction == "" {
			return 0, errors.New("invalid decimal amount")
		}
	}
	for _, c := range digits + fraction {
		if c < '0' || c > '9' {
			return 0, errors.New("amount must be an unsigned decimal")
		}
	}
	decimals := currencyMinorDecimals(currency)
	if len(fraction) > decimals {
		return 0, errors.New("amount has excess precision")
	}
	return strconv.ParseInt(digits+fraction+strings.Repeat("0", decimals-len(fraction)), 10, 64)
}
func configuredMinor(value interface{}, currency string, alreadyMinor bool) (int64, error) {
	// Preserve decimal semantics when reading existing numeric configuration;
	// conversion and tax calculation use decimal strings, not float arithmetic.
	raw := fmt.Sprint(value)
	if f, ok := value.(float64); ok {
		raw = strconv.FormatFloat(f, 'f', -1, 64)
	}
	if alreadyMinor {
		if strings.ContainsAny(raw, ".-+eE") {
			return 0, errors.New("minor amount must be an integer")
		}
		return strconv.ParseInt(raw, 10, 64)
	}
	return majorValueToMinor(raw, currency)
}
func minorDecimal(amount int64, currency string) string {
	d := currencyMinorDecimals(currency)
	if d == 0 {
		return strconv.FormatInt(amount, 10)
	}
	divisor := int64(1)
	for i := 0; i < d; i++ {
		divisor *= 10
	}
	return fmt.Sprintf("%d.%0*d", amount/divisor, d, amount%divisor)
}
func formatMinorUnits(amount int64, currency string) string {
	return minorDecimal(amount, currency) + " " + currency
}

// FormatConfiguredPrice formats stored amounts using the currency's minor units.
func FormatConfiguredPrice(value interface{}, currency string, alreadyMinor bool) (string, error) {
	amount, err := configuredMinor(value, currency, alreadyMinor)
	if err != nil {
		return "", err
	}
	return formatMinorUnits(amount, currency), nil
}
