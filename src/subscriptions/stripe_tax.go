package subscriptions

import (
	"errors"
	"github.com/abishekmuthian/open-payment-host/src/invoices"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/taxrate"
	"math/big"
	"strconv"
)

func stripeExpectedAmount(base int64, id string) (int64, error) {
	if id == "" {
		return base, nil
	}
	client := taxrate.Client{B: stripe.GetBackend(stripe.APIBackend), Key: config.Get("stripe_secret")}
	rate, err := client.Get(id, nil)
	if err != nil {
		return 0, err
	}
	if !rate.Active {
		return 0, errors.New("inactive tax rate")
	}
	if rate.Inclusive {
		return base, nil
	}
	pct, ok := new(big.Rat).SetString(strconv.FormatFloat(rate.Percentage, 'f', -1, 64))
	if !ok || pct.Sign() < 0 {
		return 0, errors.New("invalid tax percentage")
	}
	tax := new(big.Rat).Mul(new(big.Rat).SetInt64(base), pct)
	tax.Quo(tax, big.NewRat(100, 1))
	tax.Add(tax, big.NewRat(1, 2))
	rounded := new(big.Int).Quo(tax.Num(), tax.Denom())
	rounded.Add(rounded, big.NewInt(base))
	if !rounded.IsInt64() {
		return 0, errors.New("amount overflow")
	}
	return rounded.Int64(), nil
}

func validateStripeGST(p invoices.Product, base, gross int64, id, country string) error {
	expected, err := invoices.Total(base, p, country)
	if err != nil {
		return err
	}
	if expected != gross {
		return errors.New("Stripe charged total must match product GST mode")
	}
	if country != "IN" {
		if id != "" {
			return errors.New("LUT export Stripe price must not collect GST")
		}
		return nil
	}
	if id == "" {
		if p.Rate == "0" {
			return nil
		}
		return errors.New("configure a matching Stripe tax-rate ID")
	}
	client := taxrate.Client{B: stripe.GetBackend(stripe.APIBackend), Key: config.Get("stripe_secret")}
	r, err := client.Get(id, nil)
	if err != nil {
		return err
	}
	pct, ok := new(big.Rat).SetString(strconv.FormatFloat(r.Percentage, 'f', -1, 64))
	wanted, wok := new(big.Rat).SetString(p.Rate)
	if !ok || !wok || pct.Cmp(wanted) != 0 || r.Inclusive != p.Inclusive {
		return errors.New("Stripe tax rate percentage/inclusive setting must match product GST")
	}
	return nil
}
