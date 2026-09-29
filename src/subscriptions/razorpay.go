package subscriptions

import (
	"errors"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"net/http"
	"net/url"
	"time"
)

func HandleRazorpayShow(w http.ResponseWriter, r *http.Request) error {
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
	country := checkoutCountry(r)
	price := p.RazorpayPrice[country]
	field := "amount"
	if p.Schedule != "onetime" {
		field = "plan_id"
	}
	if price == nil || price[field] == nil {
		country = "DF"
		price = p.RazorpayPrice[country]
	}
	if price == nil || price[field] == nil {
		return server.BadRequestError(errors.New("Razorpay price not configured"))
	}
	var amount int64
	var currency, planID string
	if p.Schedule == "onetime" {
		currency, _ = price["currency"].(string)
		amount, err = configuredMinor(price["amount"], currency, false)
		if err != nil {
			return server.BadRequestError(err)
		}
		if price["tax"] != nil {
			tax, err := configuredMinor(price["tax"], currency, false)
			if err != nil {
				return server.BadRequestError(err)
			}
			if tax < 0 || amount > int64(^uint64(0)>>1)-tax {
				return server.BadRequestError(errors.New("invalid total"))
			}
			amount += tax
		}
	} else {
		planID, _ = price["plan_id"].(string)
		var plan struct {
			ID       string `json:"id"`
			Period   string `json:"period"`
			Interval int    `json:"interval"`
			Item     struct {
				Amount   int64  `json:"amount"`
				Currency string `json:"currency"`
			} `json:"item"`
		}
		if err := providerJSON("razorpay", http.MethodGet, "/plans/"+url.PathEscape(planID), nil, &plan); err != nil {
			return server.InternalError(err)
		}
		if plan.ID != planID || plan.Period != p.Schedule || plan.Interval != 1 {
			return server.BadRequestError(errors.New("Razorpay plan schedule mismatch"))
		}
		amount = plan.Item.Amount
		currency = plan.Item.Currency
	}
	a, err := newAttempt(p.ID, "razorpay", p.Schedule, country, amount, currency, planID, params.Get("custom_id"), redirect)
	if err != nil {
		return server.InternalError(err)
	}
	var resource struct {
		ID string `json:"id"`
	}
	if p.Schedule == "onetime" {
		if err := providerJSON("razorpay", http.MethodPost, "/orders", map[string]interface{}{"amount": amount, "currency": currency, "receipt": a.Id}, &resource); err != nil {
			return server.InternalError(err)
		}
		if resource.ID == "" {
			return server.InternalError(errors.New("missing order id"))
		}
		if err := a.SetProviderIds(resource.ID, "", ""); err != nil {
			return server.InternalError(err)
		}
	} else {
		total := 120
		if p.Schedule == "yearly" {
			total = 30
		}
		if err := providerJSON("razorpay", http.MethodPost, "/subscriptions", map[string]interface{}{"plan_id": planID, "quantity": 1, "total_count": total, "notes": map[string]string{"attempt_id": a.Id}}, &resource); err != nil {
			return server.InternalError(err)
		}
		if resource.ID == "" {
			return server.InternalError(errors.New("missing subscription id"))
		}
		if err := a.SetProviderIds("", "", resource.ID); err != nil {
			return server.InternalError(err)
		}
	}
	setAttemptCookie(w, r, a)
	v := view.NewRenderer(w, r)
	v.AddKey("currentUser", session.CurrentUser(w, r))
	v.AddKey("story", p)
	v.AddKey("name", config.Get("name"))
	v.AddKey("year", time.Now().Year())
	v.AddKey("clientCountry", checkoutCountry(r))
	v.AddKey("customerName", params.Get("customer_name"))
	v.AddKey("customerEmail", params.Get("customer_email"))
	v.AddKey("loadRazorpayScript", true)
	v.AddKey("loadHypermedia", true)
	v.AddKey("loadSweetAlert", true)
	v.AddKey("meta_product_id", p.ID)
	v.AddKey("meta_product_title", p.Name)
	v.AddKey("meta_razorpay_key_id", config.Get("razorpay_key_id"))
	if p.Schedule == "onetime" {
		v.AddKey("meta_payment_script_type", "checkout")
		v.AddKey("meta_product_order_id", resource.ID)
		v.AddKey("meta_product_amount", amount)
		v.AddKey("meta_product_currency", currency)
	} else {
		v.AddKey("meta_payment_script_type", "subscription")
		v.AddKey("meta_product_subscription_ID", resource.ID)
	}
	return v.Render()
}
func CancelRazorpaySubscription(id string) error {
	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := providerJSON("razorpay", http.MethodPost, "/subscriptions/"+url.PathEscape(id)+"/cancel", map[string]bool{"cancel_at_cycle_end": true}, &result); err != nil {
		return err
	}
	if result.ID != id || (result.Status != "active" && result.Status != "cancelled") {
		return errors.New("cancellation not accepted")
	}
	return nil
}
