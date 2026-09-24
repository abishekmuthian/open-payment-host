package subscriptions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"io"
	"net/http"
	"strings"
)

// providerJSON centralizes bounded, authenticated API calls. Identifiers are
// path-escaped by callers. No provider errors or credentials enter browser URLs.
func providerJSON(gateway, method, path string, payload, result interface{}, requestID ...string) error {
	base := ""
	switch gateway {
	case "paypal":
		base = config.Get("paypal_api_domain")
	case "square":
		base = config.Get("square_domain")
	case "razorpay":
		base = "https://api.razorpay.com/v1"
	default:
		return fmt.Errorf("unknown gateway")
	}
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequest(method, strings.TrimRight(base, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if len(requestID) > 0 {
		req.Header.Set("PayPal-Request-Id", requestID[0])
	}
	switch gateway {
	case "paypal":
		token, err := GetPaypalAuthorizationToken()
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	case "square":
		req.Header.Set("Authorization", "Bearer "+config.Get("square_access_token"))
		req.Header.Set("Square-Version", "2023-04-19")
	case "razorpay":
		req.SetBasicAuth(config.Get("razorpay_key_id"), config.Get("razorpay_key_secret"))
	}
	resp, err := paymentHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s API returned HTTP %d", gateway, resp.StatusCode)
	}
	if result == nil {
		return nil
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	dec.UseNumber()
	return dec.Decode(result)
}
