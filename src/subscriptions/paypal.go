package subscriptions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func checkoutCountry(r *http.Request) string {
	if !config.Production() {
		return config.Get("subscription_client_country")
	}
	return r.Header.Get("CF-IPCountry")
}
func paypalConfiguredPrice(p *products.Story, country string) (map[string]interface{}, string, error) {
	field := "amount"
	if p.Schedule != "onetime" {
		field = "plan_id"
	}
	price := p.PaypalPrice[country]
	if price == nil || price[field] == nil {
		country = "DF"
		price = p.PaypalPrice[country]
	}
	if price == nil || price[field] == nil {
		return nil, "", errors.New("PayPal price not configured")
	}
	return price, country, nil
}
func HandlePaypalShow(w http.ResponseWriter, r *http.Request) error {
	paymentResponseHeaders(w)
	params, err := mux.Params(r)
	if err != nil {
		return server.BadRequestError(err)
	}
	p, err := products.Find(params.GetInt("product_id"))
	if err != nil {
		return server.NotFoundError(err)
	}
	redirect, err := ValidateRedirectURI(p, params.Get("redirect_uri"))
	if err != nil {
		return server.BadRequestError(err)
	}
	price, country, err := paypalConfiguredPrice(p, checkoutCountry(r))
	if err != nil {
		return server.BadRequestError(err)
	}
	var amount int64
	var currency, planID string
	if p.Schedule == "onetime" {
		currency, _ = price["currency"].(string)
		amount, err = configuredMinor(price["amount"], currency, false)
		if err != nil {
			return server.BadRequestError(err)
		}
		if tax := price["tax"]; tax != nil {
			taxMinor, err := configuredMinor(tax, currency, false)
			if err != nil {
				return server.BadRequestError(err)
			}
			if taxMinor < 0 || amount > int64(^uint64(0)>>1)-taxMinor {
				return server.BadRequestError(errors.New("invalid total"))
			}
			amount += taxMinor
		}
	} else {
		planID, _ = price["plan_id"].(string)
		amount, currency, err = fetchPaypalPlanPrice(planID, p.Schedule)
		if err != nil {
			return server.BadRequestError(err)
		}
	}
	a, err := newAttempt(p.ID, "paypal", p.Schedule, country, amount, currency, planID, params.Get("custom_id"), redirect)
	if err != nil {
		return server.InternalError(err)
	}
	setAttemptCookie(w, r, a)
	v := view.NewRenderer(w, r)
	v.AddKey("currentUser", session.CurrentUser(w, r))
	v.AddKey("story", p)
	v.AddKey("name", config.Get("name"))
	v.AddKey("year", time.Now().Year())
	v.AddKey("clientId", config.Get("paypal_client_id"))
	v.AddKey("currency", currency)
	v.AddKey("loadSweetAlert", true)
	v.AddKey("meta_product_id", p.ID)
	v.AddKey("meta_product_amount", minorDecimal(amount, currency))
	v.AddKey("price", formatMinorUnits(amount, currency))
	if p.Schedule == "onetime" {
		v.AddKey("loadPaypalOneTimeScript", true)
		v.AddKey("meta_payment_script_type", "checkout")
	} else {
		v.AddKey("loadPaypalSubscriptionScript", true)
		v.AddKey("meta_payment_script_type", "subscription")
	}
	if !config.Production() {
		v.AddKey("sandbox", true)
		v.AddKey("country", checkoutCountry(r))
	}
	return v.Render()
}
func paypalAttempt(r *http.Request, recurring bool) (*PaymentAttempt, error) {
	parts := strings.Split(attemptCookie(r), ".")
	if len(parts) != 2 {
		return nil, errors.New("missing checkout capability")
	}
	a, err := FindAttempt(parts[0])
	if err != nil {
		return nil, err
	}
	if !bindAttemptToRequest(r, a) || a.Gateway != "paypal" || (a.Schedule != "onetime") != recurring || a.Status != "pending" {
		return nil, errors.New("invalid checkout capability")
	}
	return a, nil
}
func HandlePaypalCreateOrder(w http.ResponseWriter, r *http.Request) error {
	if err := session.CheckAuthenticity(w, r); err != nil {
		return err
	}
	a, err := paypalAttempt(r, false)
	if err != nil {
		return server.NotAuthorizedError(err)
	}
	p, err := products.Find(a.ProductId)
	if err != nil {
		return server.NotFoundError(err)
	}
	if a.ProviderOrderId != "" {
		return paymentJSON(w, map[string]string{"id": a.ProviderOrderId})
	}
	// The single fixed-price item includes the frozen configured tax total.
	data := map[string]interface{}{"intent": "CAPTURE", "purchase_units": []interface{}{map[string]interface{}{
		"reference_id": a.Id, "custom_id": a.CustomId,
		"amount": map[string]interface{}{"currency_code": a.Currency, "value": minorDecimal(a.Amount, a.Currency), "breakdown": map[string]interface{}{"item_total": map[string]string{"currency_code": a.Currency, "value": minorDecimal(a.Amount, a.Currency)}}},
		"items":  []interface{}{map[string]interface{}{"name": p.Name, "sku": formatInt(a.ProductId), "quantity": "1", "unit_amount": map[string]string{"currency_code": a.Currency, "value": minorDecimal(a.Amount, a.Currency)}}},
	}}}
	var result PaypalCreateOrderResult
	if err := providerJSON("paypal", http.MethodPost, "/v2/checkout/orders", data, &result, a.Id); err != nil {
		return server.InternalError(err)
	}
	if result.ID == "" {
		return server.InternalError(errors.New("missing provider order id"))
	}
	if err := a.SetProviderIds(result.ID, "", ""); err != nil {
		return server.InternalError(err)
	}
	return paymentJSON(w, result)
}
func HandlePaypalCreateSubscription(w http.ResponseWriter, r *http.Request) error {
	if err := session.CheckAuthenticity(w, r); err != nil {
		return err
	}
	a, err := paypalAttempt(r, true)
	if err != nil {
		return server.NotAuthorizedError(err)
	}
	if a.ProviderSubscriptionId != "" {
		return paymentJSON(w, map[string]string{"id": a.ProviderSubscriptionId})
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := providerJSON("paypal", http.MethodPost, "/v1/billing/subscriptions", map[string]string{"plan_id": a.PriceId, "custom_id": a.Id, "quantity": "1"}, &result, a.Id); err != nil {
		return server.InternalError(err)
	}
	if result.ID == "" {
		return server.InternalError(errors.New("missing provider subscription id"))
	}
	if err := a.SetProviderIds("", "", result.ID); err != nil {
		return server.InternalError(err)
	}
	return paymentJSON(w, result)
}
func HandlePaypalCaptureOrder(w http.ResponseWriter, r *http.Request) error {
	if err := session.CheckAuthenticity(w, r); err != nil {
		return err
	}
	a, err := paypalAttempt(r, false)
	if err != nil {
		return server.NotAuthorizedError(err)
	}
	params, err := mux.Params(r)
	if err != nil {
		return server.BadRequestError(err)
	}
	if a.ProviderOrderId == "" || params.Get("id") != a.ProviderOrderId {
		return server.NotAuthorizedError(errors.New("order mismatch"))
	}
	var result PaypalCaptureOrderResult
	if err := providerJSON("paypal", http.MethodPost, "/v2/checkout/orders/"+url.PathEscape(a.ProviderOrderId)+"/capture", map[string]interface{}{}, &result, a.Id+"-capture"); err != nil {
		return server.InternalError(err)
	}
	return paymentJSON(w, result)
}
func paymentJSON(w http.ResponseWriter, v interface{}) error {
	paymentResponseHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(v)
}
func GetPaypalAuthorizationToken() (string, error) {
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(config.Get("paypal_api_domain"), "/")+"/v1/oauth2/token", bytes.NewBufferString("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(config.Get("paypal_client_id"), config.Get("paypal_client_secret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := paymentHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PayPal authorization HTTP %d", resp.StatusCode)
	}
	var result struct {
		Token string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return "", err
	}
	if result.Token == "" {
		return "", errors.New("missing PayPal access token")
	}
	return result.Token, nil
}
func fetchPaypalPlanPrice(id, schedule string) (int64, string, error) {
	var p struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Cycles []struct {
			Tenure    string `json:"tenure_type"`
			Frequency struct {
				Unit  string `json:"interval_unit"`
				Count int    `json:"interval_count"`
			} `json:"frequency"`
			Pricing struct {
				Price struct {
					Value    string `json:"value"`
					Currency string `json:"currency_code"`
				} `json:"fixed_price"`
			} `json:"pricing_scheme"`
		} `json:"billing_cycles"`
		Preferences struct {
			Setup struct {
				Value string `json:"value"`
			} `json:"setup_fee"`
		} `json:"payment_preferences"`
		Taxes struct {
			Percentage string `json:"percentage"`
			Inclusive  bool   `json:"inclusive"`
		} `json:"taxes"`
	}
	if err := providerJSON("paypal", http.MethodGet, "/v1/billing/plans/"+url.PathEscape(id), nil, &p); err != nil {
		return 0, "", err
	}
	if p.ID != id || p.Status != "ACTIVE" || len(p.Cycles) != 1 || p.Cycles[0].Tenure != "REGULAR" {
		return 0, "", errors.New("only a single fixed-price regular PayPal cycle is supported")
	}
	c := p.Cycles[0]
	unit := "MONTH"
	if schedule == "yearly" {
		unit = "YEAR"
	}
	if c.Frequency.Unit != unit || c.Frequency.Count != 1 {
		return 0, "", errors.New("PayPal schedule mismatch")
	}
	amount, err := majorValueToMinor(c.Pricing.Price.Value, c.Pricing.Price.Currency)
	if err != nil {
		return 0, "", err
	}
	if p.Preferences.Setup.Value != "" {
		setup, err := majorValueToMinor(p.Preferences.Setup.Value, c.Pricing.Price.Currency)
		if err != nil || setup != 0 {
			return 0, "", errors.New("PayPal setup fees require a separate payment policy")
		}
	}
	if !p.Taxes.Inclusive && p.Taxes.Percentage != "" {
		amount, err = addPercentage(amount, p.Taxes.Percentage)
	}
	return amount, c.Pricing.Price.Currency, err
}
func addPercentage(amount int64, pct string) (int64, error) {
	p, ok := new(big.Rat).SetString(pct)
	if !ok || p.Sign() < 0 {
		return 0, errors.New("invalid tax")
	}
	tax := new(big.Rat).Mul(big.NewRat(amount, 100), p)
	tax.Add(tax, big.NewRat(1, 2))
	total := new(big.Int).Quo(tax.Num(), tax.Denom())
	total.Add(total, big.NewInt(amount))
	if !total.IsInt64() {
		return 0, errors.New("tax total overflow")
	}
	return total.Int64(), nil
}
func CancelPaypalSubscription(id string) error {
	return providerJSON("paypal", http.MethodPost, "/v1/billing/subscriptions/"+url.PathEscape(id)+"/cancel", map[string]string{"reason": "User requested cancellation"}, nil)
}
