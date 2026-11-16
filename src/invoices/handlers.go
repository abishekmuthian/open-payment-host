package invoices

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/listmonk"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/resource"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

func admin(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	if !session.CurrentUser(w, r).Admin() {
		return server.NotAuthorizedError(nil)
	}
	if r.Method == "POST" {
		return session.CheckAuthenticity(w, r)
	}
	return nil
}
func page(w http.ResponseWriter, r *http.Request, path string, values map[string]interface{}) error {
	v := view.NewRenderer(w, r)
	v.AddKey("currentUser", session.CurrentUser(w, r))
	v.AddKey("name", config.Get("name"))
	for k, value := range values {
		v.AddKey(k, value)
	}
	v.Template(path)
	return v.Render()
}
func HandleSettings(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	if err := admin(w, r); err != nil {
		return err
	}
	s, err := LoadSettings()
	if errors.Is(err, sql.ErrNoRows) {
		s = Settings{Prefix: "OPH", CreditPrefix: "CN", Start: 1}
	} else if err != nil {
		return server.InternalError(err)
	}
	if r.Method == "POST" {
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			return server.BadRequestError(err)
		}
		defer r.MultipartForm.RemoveAll()
		s.Name = strings.TrimSpace(r.FormValue("name"))
		s.Address = strings.TrimSpace(r.FormValue("address"))
		s.GSTIN = strings.ToUpper(strings.TrimSpace(r.FormValue("gstin")))
		s.State = StateCode(r.FormValue("state"))
		s.Email = r.FormValue("email")
		s.Signatory = r.FormValue("signatory")
		s.Prefix = r.FormValue("prefix")
		s.CreditPrefix = r.FormValue("credit_prefix")
		if r.PostForm.Has("numbering_style") {
			s.NumberingStyle = r.FormValue("numbering_style")
		}
		if r.PostForm.Has("sequence_digits") {
			s.SequenceDigits, err = strconv.Atoi(r.FormValue("sequence_digits"))
			if err != nil || s.SequenceDigits < 1 || s.SequenceDigits > 6 {
				return server.BadRequestError(errors.New("sequence padding must be between 1 and 6 digits"))
			}
		}
		s.Start, err = strconv.ParseInt(r.FormValue("start"), 10, 64)
		if err != nil {
			return server.BadRequestError(errors.New("enter a minimum next sequence from 1 to 999999"))
		}
		s.LUT = r.FormValue("lut")
		s.LUTYear, err = normaliseLUTYear(r.FormValue("lut_year"))
		if err != nil {
			return server.BadRequestError(err, "Check LUT financial year", err.Error())
		}
		s.DeliveryProvider = r.FormValue("delivery_provider")
		if s.DeliveryProvider == "listmonk" {
			invoiceTemplate, invoiceErr := strconv.Atoi(r.FormValue("listmonk_invoice_template"))
			creditTemplate, creditErr := strconv.Atoi(r.FormValue("listmonk_credit_template"))
			if invoiceErr != nil || creditErr != nil {
				return server.BadRequestError(errors.New("enter numeric Listmonk invoice and credit-note template IDs"))
			}
			s.ListmonkInvoiceTemplate = invoiceTemplate
			s.ListmonkCreditTemplate = creditTemplate
			s.ListmonkMessenger = strings.TrimSpace(r.FormValue("listmonk_messenger"))
			s.ListmonkFrom = strings.TrimSpace(r.FormValue("listmonk_from"))
		}
		if r.FormValue("remove_signature") == "on" {
			s.Signature = nil
		}
		f, _, fileErr := r.FormFile("signature")
		if fileErr == nil {
			defer f.Close()
			data, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
			if e != nil {
				return server.BadRequestError(e)
			}
			s.Signature, e = NormaliseSignature(data)
			if e != nil {
				return server.BadRequestError(e)
			}
		} else if fileErr != http.ErrMissingFile {
			return server.BadRequestError(fileErr)
		}
		if err = s.Validate(); err != nil {
			return server.BadRequestError(err, "Check invoicing settings", err.Error())
		}
		if err = s.ValidateDelivery(); err != nil {
			return server.BadRequestError(err)
		}
		if s.DeliveryProvider == "listmonk" {
			if err = listmonk.ValidateTransactionalConfig(config.Get("listmonk_URL"), config.Get("listmonk_API_token")); err != nil {
				return server.BadRequestError(err)
			}
		}
		_, err = query.Exec("INSERT INTO invoice_settings(id,value) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET value=excluded.value", JSON(s))
		if err != nil {
			return server.InternalError(err)
		}
		return server.Redirect(w, r, "/admin/invoicing")
	}
	return page(w, r, "invoices/views/settings.html.got", map[string]interface{}{"seller": s})
}

// HandleDeliveryFields renders only the selected provider's fields. Credentials
// stay in configuration and are never included in the HTML response.
func HandleDeliveryFields(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	if err := admin(w, r); err != nil {
		return err
	}
	s, err := LoadSettings()
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return server.InternalError(err)
	}
	s.DeliveryProvider = r.FormValue("delivery_provider")
	if s.DeliveryProvider != "default" && s.DeliveryProvider != "" && s.DeliveryProvider != "listmonk" {
		return server.BadRequestError(errors.New("unknown invoice email provider"))
	}
	v := view.NewRenderer(w, r)
	v.Layout("")
	v.Template("invoices/views/delivery_fields.html.got")
	v.AddKey("seller", s)
	v.AddKey("listmonkConfigured", listmonk.ValidateTransactionalConfig(config.Get("listmonk_URL"), config.Get("listmonk_API_token")) == nil)
	return v.Render()
}
func HandleSignature(w http.ResponseWriter, r *http.Request) error {
	if err := admin(w, r); err != nil {
		return err
	}
	s, err := LoadSettings()
	if err != nil || len(s.Signature) == 0 {
		return server.NotFoundError(err)
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, err = w.Write(s.Signature)
	return err
}
func HandleIndex(w http.ResponseWriter, r *http.Request) error {
	if err := admin(w, r); err != nil {
		return err
	}
	rows, err := query.New("invoices", "id").Select("SELECT id,kind,number,status,created_at,last_error,email_done FROM invoices").Order("created_at DESC,id").Limit(200).Results()
	if err != nil {
		return server.InternalError(err)
	}
	for _, row := range rows {
		row["email_submitted"] = resource.ValidateInt(row["email_done"]) != 0
	}
	return page(w, r, "invoices/views/index.html.got", map[string]interface{}{"documents": rows})
}
func HandleDocument(w http.ResponseWriter, r *http.Request) error {
	if err := admin(w, r); err != nil {
		return err
	}
	p, err := mux.Params(r)
	if err != nil {
		return server.BadRequestError(err)
	}
	id := p.Get("id")
	var body, number, status, lastError, attempt string
	var ready, recurring, submitted bool
	err = query.Transaction(func(tx *query.Tx) error {
		return tx.QueryRow("SELECT snapshot,number,status,last_error,pdf IS NOT NULL,attempt_id,EXISTS(SELECT 1 FROM payment_attempts a WHERE a.id=invoices.attempt_id AND a.schedule IN ('monthly','yearly')),email_done FROM invoices WHERE id=?", id).Scan(&body, &number, &status, &lastError, &ready, &attempt, &recurring, &submitted)
	})
	if err != nil {
		return server.NotFoundError(err)
	}
	var s Snapshot
	if err = json.Unmarshal([]byte(body), &s); err != nil {
		return server.InternalError(err)
	}
	if r.Method == "POST" {
		if r.FormValue("action") == "resend" {
			result, err := query.Exec("UPDATE invoices SET email_done=0,status='issued',attempts=0,next_attempt_at=0,last_error='' WHERE id=? AND number<>'' AND pdf IS NOT NULL AND email_done=1 AND status IN ('issued','delivery_failed') AND lease_until<?", id, time.Now().Unix())
			if err != nil {
				return server.InternalError(err)
			}
			n, err := result.RowsAffected()
			if err != nil {
				return server.InternalError(err)
			}
			if n != 1 {
				return server.BadRequestError(errors.New("email cannot be resent while pending or being processed; reload the document"))
			}
			return server.Redirect(w, r, "/admin/invoices/"+id)
		}
		repaired := false
		// Only an unnumbered invoice may have incomplete billing/configuration repaired.
		if number == "" && s.RefundAmount == 0 && r.FormValue("billing_name") != "" {
			s.Billing = BillingFromRequest(r)
			if err = s.Billing.Validate(); err != nil {
				return server.BadRequestError(err)
			}
			seller, e := LoadSettings()
			if e != nil {
				return server.BadRequestError(e)
			}
			s.Context.Seller = seller
			s.Context.ReviewReason = ""
			s.Context.Product.Code = r.FormValue("invoice_code")
			s.Context.Product.Rate = r.FormValue("invoice_rate")
			s.Context.Product.Inclusive = r.FormValue("invoice_inclusive") == "on"
			base, e := strconv.ParseInt(r.FormValue("invoice_base"), 10, 64)
			if e != nil || base <= 0 {
				return server.BadRequestError(errors.New("enter a positive invoice base in currency minor units"))
			}
			s.Context.Base = base
			s.Context.Country = s.Billing.Country
			if err = Calculate(&s); err != nil {
				return server.BadRequestError(err)
			}

			body = JSON(s)
			repaired = true
		}
		busy := errors.New("document is being processed or has been issued; reload before retrying")
		err = query.Transaction(func(tx *query.Tx) error {
			var result sql.Result
			var e error
			if repaired {
				result, e = tx.Exec("UPDATE invoices SET snapshot=?,status='pending',attempts=0,next_attempt_at=0,last_error='' WHERE id=? AND number='' AND lease_until<?", body, id, time.Now().Unix())
			} else {
				result, e = tx.Exec("UPDATE invoices SET status=CASE WHEN pdf IS NULL THEN 'pending' ELSE 'issued' END,attempts=0,next_attempt_at=0,last_error='' WHERE id=? AND lease_until<?", id, time.Now().Unix())
			}
			if e != nil {
				return e
			}
			n, e := result.RowsAffected()
			if e != nil {
				return e
			}
			if n != 1 {
				return busy
			}
			if repaired && recurring && r.FormValue("save_profile") == "on" {
				_, e = tx.Exec("UPDATE payment_attempts SET invoice_context=?,invoice_billing=? WHERE id=?", JSON(s.Context), JSON(s.Billing), attempt)
			}
			return e
		})
		if errors.Is(err, busy) {
			return server.BadRequestError(err)
		}
		if err != nil {
			return server.InternalError(err)
		}
		return server.Redirect(w, r, "/admin/invoices/"+id)
	}
	return page(w, r, "invoices/views/document.html.got", map[string]interface{}{"recurring": recurring, "ready": ready, "submitted": submitted, "document": s, "billing": s.Billing, "id": id, "number": number, "status": status, "error": lastError})
}
func BillingFromRequest(r *http.Request) Billing {
	return Billing{Name: r.FormValue("billing_name"), Business: r.FormValue("billing_business"), Email: r.FormValue("billing_email"), Address: r.FormValue("billing_address"), City: r.FormValue("billing_city"), State: r.FormValue("billing_state"), PostalCode: r.FormValue("billing_postcode"), Country: r.FormValue("billing_country"), GSTIN: r.FormValue("billing_gstin")}
}
func HandleDownload(w http.ResponseWriter, r *http.Request) error {
	if err := admin(w, r); err != nil {
		return err
	}
	p, err := mux.Params(r)
	if err != nil {
		return server.BadRequestError(err)
	}
	return Download(w, p.Get("id"), "")
}

// Download requires either prior admin authorization or an already-authorized attempt ID.
func Download(w http.ResponseWriter, id, attempt string) error {
	var pdf []byte
	var number string
	err := query.Transaction(func(tx *query.Tx) error {
		if attempt != "" {
			return tx.QueryRow("SELECT pdf,number FROM invoices WHERE attempt_id=? AND kind='invoice' ORDER BY created_at LIMIT 1", attempt).Scan(&pdf, &number)
		}
		return tx.QueryRow("SELECT pdf,number FROM invoices WHERE id=?", id).Scan(&pdf, &number)
	})
	if err != nil || len(pdf) == 0 {
		return server.NotFoundError(err)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", `attachment; filename="`+numberToFilename(number)+`"`)
	_, err = w.Write(pdf)
	return err
}
func Ready(attempt string) (bool, bool, error) {
	var count, ready int
	err := query.Transaction(func(tx *query.Tx) error {
		return tx.QueryRow("SELECT COUNT(*),COALESCE(SUM(CASE WHEN pdf IS NOT NULL THEN 1 ELSE 0 END),0) FROM invoices WHERE attempt_id=? AND kind='invoice'", attempt).Scan(&count, &ready)
	})
	return count > 0, ready > 0, err
}

// SnapshotBilling is immutable after provider payment starts. A browser secret
// checked by the caller is required; a UUID or provider notes are insufficient.
func SnapshotBilling(attempt string, b Billing) error {
	if err := b.Validate(); err != nil {
		return err
	}
	return query.Transaction(func(tx *query.Tx) error {
		if _, err := tx.Exec("UPDATE payment_attempts SET id=id WHERE id=?", attempt); err != nil {
			return err
		}
		var ctx, stored, status, currency string
		var amount int64
		if err := tx.QueryRow("SELECT invoice_context,invoice_billing,status,currency,amount FROM payment_attempts WHERE id=?", attempt).Scan(&ctx, &stored, &status, &currency, &amount); err != nil {
			return err
		}
		if stored != "" {
			if stored == JSON(b) {
				return nil
			}
			return errors.New("billing details are already frozen; start another checkout")
		}
		if status != "pending" || ctx == "" {
			return errors.New("checkout no longer accepts billing details")
		}
		snap := Snapshot{Billing: b, Currency: currency, Gross: amount, PaidAt: time.Now().UTC().Format(time.RFC3339)}
		if err := json.Unmarshal([]byte(ctx), &snap.Context); err != nil {
			return err
		}
		if err := Calculate(&snap); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE payment_attempts SET invoice_billing=? WHERE id=?", JSON(b), attempt)
		return err
	})
}
