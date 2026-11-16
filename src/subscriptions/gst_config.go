package subscriptions

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/abishekmuthian/open-payment-host/src/invoices"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/price"
)

// GSTPrices validates external tax settings without mutating prices or plans.
type GSTPrices struct {
	Stripe           map[string]string
	Paypal, Razorpay map[string]map[string]interface{}
}

func ValidateGSTPrices(g invoices.Product, schedule string, prices GSTPrices) error {
	if !g.Enabled {
		return nil
	}
	if config.GetBool("stripe") {
		for country, id := range prices.Stripe {
			client := price.Client{B: stripe.GetBackend(stripe.APIBackend), Key: config.Get("stripe_secret")}
			p, err := client.Get(id, nil)
			if err != nil {
				return fmt.Errorf("Stripe price %s: %w", country, err)
			}
			rate := config.Get("stripe_tax_rate_" + country)
			gross, err := stripeExpectedAmount(p.UnitAmount, rate)
			if err != nil {
				return err
			}
			if err = validateStripeGST(g, p.UnitAmount, gross, rate, country); err != nil {
				return fmt.Errorf("Stripe %s: %w", country, err)
			}
			if country == "DF" && prices.Stripe["IN"] == "" {
				rate = config.Get("stripe_tax_rate_IN")
				gross, err = stripeExpectedAmount(p.UnitAmount, rate)
				if err != nil {
					return err
				}
				if err = validateStripeGST(g, p.UnitAmount, gross, rate, "IN"); err != nil {
					return fmt.Errorf("Stripe India fallback: %w", err)
				}
			}
		}
	}
	if schedule == "onetime" {
		return nil
	}
	if config.GetBool("paypal") {
		for country, row := range prices.Paypal {
			id, _ := row["plan_id"].(string)
			policy := g
			if country != "IN" {
				policy.Rate = "0"
			}
			gross, currency, err := fetchPaypalPlanPrice(id, schedule, policy)
			if err != nil {
				return fmt.Errorf("PayPal %s: %w", country, err)
			}
			if row["amount"] != nil {
				base, err := configuredMinor(row["amount"], currency, false)
				if err != nil {
					return err
				}
				expected, err := invoices.Total(base, g, country)
				if err != nil || expected != gross {
					return fmt.Errorf("PayPal %s plan total must match the configured GST amount", country)
				}
			}
		}
	}
	if config.GetBool("razorpay") && !g.Inclusive {
		for country, row := range prices.Razorpay {
			if country != "IN" {
				continue
			}
			id, _ := row["plan_id"].(string)
			var plan struct {
				Item struct {
					Amount   int64  `json:"amount"`
					Currency string `json:"currency"`
				} `json:"item"`
			}
			if err := providerJSON("razorpay", http.MethodGet, "/plans/"+url.PathEscape(id), nil, &plan); err != nil {
				return err
			}
			base, err := configuredMinor(row["amount"], plan.Item.Currency, false)
			if err != nil {
				return errors.New("set Razorpay recurring base amount for exclusive GST")
			}
			gross, err := invoices.Total(base, g, "IN")
			if err != nil || gross != plan.Item.Amount {
				return errors.New("Razorpay plan must already charge base amount plus GST")
			}
		}
	}
	return nil
}
