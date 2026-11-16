package subscriptions

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/stripe/stripe-go/v72"
	"net/http"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/invoices"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

func invoiceContext(p *products.Story, base int64, country string) (string, error) {
	if p.GSTEnabled == 0 {
		return "", nil
	}
	s, err := invoices.LoadSettings()
	if err != nil {
		return "", err
	}
	if err = s.Validate(); err != nil {
		return "", err
	}
	if err = invoices.ValidateProduct(p.GST()); err != nil {
		return "", err
	}
	if country != "IN" && country != "DF" {
		if !p.GST().Export || s.LUT == "" || s.LUTYear != invoices.FinancialYear(time.Now()) {
			return "", errors.New("configure product export eligibility and a valid current-year LUT before accepting export payments")
		}
	}
	return invoices.JSON(invoices.Context{Seller: s, Product: p.GST(), Description: strings.TrimSpace(p.NameDisplay()), Base: base, Country: country}), nil
}
func setInvoiceBase(a *PaymentAttempt, p *products.Story, base int64, country string) error {
	ctx, err := invoiceContext(p, base, country)
	if err != nil {
		return err
	}
	if ctx == "" {
		return nil
	}
	_, err = query.Exec("UPDATE payment_attempts SET invoice_context=? WHERE id=? AND status='pending'", ctx, a.Id)
	a.InvoiceContext = ctx
	return err
}
func HandleInvoiceBilling(w http.ResponseWriter, r *http.Request) error {
	paymentResponseHeaders(w)
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := session.CheckAuthenticity(w, r); err != nil {
		return err
	}
	p, err := mux.Params(r)
	if err != nil {
		return server.BadRequestError(err)
	}
	a, err := FindAttempt(p.Get("attempt_id"))
	if err != nil || !bindAttemptToRequest(r, a) || a.InvoiceContext == "" {
		return server.NotAuthorizedError(nil)
	}
	if err = invoices.SnapshotBilling(a.Id, invoices.BillingFromRequest(r)); err != nil {
		return server.BadRequestError(err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write([]byte(`{"saved":true}`))
	return err
}
func HandleInvoiceDownload(w http.ResponseWriter, r *http.Request) error {
	paymentResponseHeaders(w)
	p, err := mux.Params(r)
	if err != nil {
		return server.BadRequestError(err)
	}
	a, err := FindAttempt(p.Get("attempt_id"))
	if err != nil || !completionCookieValid(r, a) || (a.Status != "completed" && a.CompletedAt.IsZero()) {
		return server.NotAuthorizedError(nil)
	}
	if r.URL.Query().Get("status") == "1" {
		exists, ready, e := invoices.Ready(a.Id)
		if e != nil {
			return server.InternalError(e)
		}
		v := view.NewRenderer(w, r)
		v.Layout("")
		v.AddKey("invoiceExists", exists)
		v.AddKey("invoiceReady", ready)
		v.AddKey("invoiceAttempt", a.Id)
		v.Template("subscriptions/views/invoice_link.html.got")
		return v.Render()
	}
	return invoices.Download(w, "", a.Id)
}

// Local billing drafts are authorized by a random HttpOnly cookie. Neither the
// URL's draft ID nor posted amounts can authorize access or payment.
func storeBillingDraft(w http.ResponseWriter, r *http.Request, b invoices.Billing) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(secret))
	id := hex.EncodeToString(hash[:16])
	_, err := query.Exec("INSERT INTO invoice_billing(id,secret_hash,value,expires_at) VALUES(?,?,?,?)", id, hex.EncodeToString(hash[:]), invoices.JSON(b), time.Now().Add(time.Hour).Unix())
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{Name: "oph_billing", Value: secret, Path: "/subscriptions", HttpOnly: true, Secure: config.Production(), SameSite: http.SameSiteLaxMode, MaxAge: 3600})
	return id, nil
}
func loadBillingDraft(r *http.Request, id string) (invoices.Billing, error) {
	var b invoices.Billing
	cookie, err := r.Cookie("oph_billing")
	if err != nil {
		return b, err
	}
	hash := sha256.Sum256([]byte(cookie.Value))
	var body, stored string
	var expires int64
	err = query.Transaction(func(tx *query.Tx) error {
		return tx.QueryRow("SELECT value,secret_hash,expires_at FROM invoice_billing WHERE id=?", id).Scan(&body, &stored, &expires)
	})
	if err != nil {
		return b, err
	}
	if stored != hex.EncodeToString(hash[:]) || expires < time.Now().Unix() {
		return b, errors.New("billing details expired")
	}
	err = json.Unmarshal([]byte(body), &b)
	return b, err
}

func stripeBilling(name, email string, address *stripe.Address) invoices.Billing {
	b := invoices.Billing{Name: name, Email: email}
	if address != nil {
		b.Address = address.Line1 + " " + address.Line2
		b.City = address.City
		b.State = address.State
		b.PostalCode = address.PostalCode
		b.Country = string(address.Country)
	}
	return b
}

// ConfiguredGSTTax is the exact monetary tax breakdown displayed in product
// editing. Charging resolves it independently from trusted configuration.
func ConfiguredGSTTax(amount, currency, country string, p invoices.Product) (string, error) {
	if amount == "" || amount == "0" || country != "IN" {
		return "0", nil
	}
	base, err := majorValueToMinor(amount, currency)
	if err != nil {
		return "", err
	}
	if err = invoices.ValidateProduct(p); err != nil {
		return "", err
	}
	var tax int64
	if p.Inclusive {
		net, e := invoices.BaseFromGross(base, p.Rate)
		if e != nil {
			return "", e
		}
		tax = base - net
	} else {
		gross, e := invoices.Total(base, p, "IN")
		if e != nil {
			return "", e
		}
		tax = gross - base
	}
	return minorDecimal(tax, currency), nil
}
