package storyactions

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

// selectGateway picks the gateway and price country used for a visitor:
// a price for the visitor's country wins (stripe > square > paypal > razorpay),
// then the default "DF" price in the same order. gateway is "" when no
// gateway has a usable price, in which case country is returned unchanged.
func selectGateway(story *products.Story, country string) (gateway, priceCountry string) {
	for _, c := range []string{country, "DF"} {
		switch {
		case story.StripePrice[c] != "":
			return "stripe", c
		case story.SquarePrice[c] != nil && story.SquarePrice[c]["amount"] != nil:
			return "square", c
		case hasAmountOrPlan(story.PaypalPrice[c]):
			return "paypal", c
		case hasAmountOrPlan(story.RazorpayPrice[c]):
			return "razorpay", c
		}
	}
	return "", country
}

func hasAmountOrPlan(price map[string]interface{}) bool {
	return price != nil && (price["amount"] != nil || price["plan_id"] != nil)
}

// priceMaps holds the per-country gateway prices submitted on a product form.
type priceMaps struct {
	Stripe   map[string]string                 // country -> price ID
	Square   map[string]map[string]interface{} // country -> {amount, currency}
	Paypal   map[string]map[string]interface{} // country -> {amount, currency, tax, plan_id}
	Razorpay map[string]map[string]interface{} // country -> {amount, currency, plan_id}
}

// buildPriceMaps reads the {gateway}_country_{n} rows of a product form.
// Rows are taken in index order; a row is skipped when its country is missing
// or already used, when an amount or tax is not a number, or when it lacks what
// its gateway needs (a price ID for Stripe, amount and currency for Square and
// one-time PayPal/Razorpay, a plan ID for recurring PayPal/Razorpay).
func buildPriceMaps(values url.Values, schedule string) priceMaps {
	recurring := schedule == "monthly" || schedule == "yearly"
	prices := priceMaps{
		Stripe:   map[string]string{},
		Square:   map[string]map[string]interface{}{},
		Paypal:   map[string]map[string]interface{}{},
		Razorpay: map[string]map[string]interface{}{},
	}
	for _, row := range priceRows(values, "stripe") {
		if planID := row.get("plan_id"); planID != "" {
			prices.Stripe[row.country] = planID
		}
	}
	for _, row := range priceRows(values, "square") {
		if price, ok := row.price("currency"); ok && price["amount"] != nil && price["currency"] != nil {
			prices.Square[row.country] = price
		}
	}
	for gateway, target := range map[string]map[string]map[string]interface{}{"paypal": prices.Paypal, "razorpay": prices.Razorpay} {
		for _, row := range priceRows(values, gateway) {
			price, ok := row.price("currency", "plan_id")
			if !ok {
				continue
			}
			if gateway == "paypal" {
				tax, ok := row.number("tax")
				if !ok {
					continue
				}
				if tax != nil {
					price["tax"] = *tax
				}
			}
			if recurring && price["plan_id"] == nil {
				continue
			}
			if !recurring && (price["amount"] == nil || price["currency"] == nil) {
				continue
			}
			target[row.country] = price
		}
	}
	return prices
}

// priceRow is one {gateway}_country_{index} row of a product form.
type priceRow struct {
	gateway, index, country string
	values                  url.Values
}

func (r priceRow) get(field string) string {
	return r.values.Get(r.gateway + "_" + field + "_" + r.index)
}

// number parses an optional numeric field; ok is false for a malformed value.
func (r priceRow) number(field string) (*float64, bool) {
	raw := r.get(field)
	if raw == "" {
		return nil, true
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		log.Error(log.V{"Product prices, skipping row with invalid " + field: raw, "gateway": r.gateway, "country": r.country})
		return nil, false
	}
	return &n, true
}

// price returns the amount plus the given non-empty text fields of the row.
func (r priceRow) price(fields ...string) (map[string]interface{}, bool) {
	price := map[string]interface{}{}
	amount, ok := r.number("amount")
	if !ok {
		return nil, false
	}
	if amount != nil {
		price["amount"] = *amount
	}
	for _, field := range fields {
		if v := r.get(field); v != "" {
			price[field] = v
		}
	}
	return price, true
}

var priceCountryKey = regexp.MustCompile(`^(stripe|square|paypal|razorpay)_country_(\d+)$`)

// priceRows returns the rows for a gateway in numeric index order, keeping
// the first row for each country.
func priceRows(values url.Values, gateway string) []priceRow {
	var rows []priceRow
	for key, v := range values {
		m := priceCountryKey.FindStringSubmatch(key)
		if m == nil || m[1] != gateway || len(v) == 0 {
			continue
		}
		rows = append(rows, priceRow{gateway: gateway, index: m[2], country: v[0], values: values})
	}
	sort.Slice(rows, func(i, j int) bool {
		a, _ := strconv.Atoi(rows[i].index)
		b, _ := strconv.Atoi(rows[j].index)
		return a < b
	})
	seen := map[string]bool{}
	kept := rows[:0]
	for _, row := range rows {
		// "Select Country" is the placeholder option, which has no value.
		if row.country == "" || row.country == "Select Country" || seen[row.country] {
			continue
		}
		seen[row.country] = true
		kept = append(kept, row)
	}
	return kept
}
