package storyactions

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/abishekmuthian/open-payment-host/src/invoices"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/abishekmuthian/open-payment-host/src/subscriptions"
)

func gstParams(values map[string]string) (map[string]string, error) {
	out := map[string]string{"gst_enabled": "0", "gst_inclusive": "0", "gst_export": "0", "gst_code": strings.TrimSpace(values["gst_code"]), "gst_rate": strings.TrimSpace(values["gst_rate"])}
	for _, key := range []string{"gst_enabled", "gst_inclusive", "gst_export"} {
		if values[key] == "on" {
			out[key] = "1"
		}
	}
	p := invoices.Product{Enabled: out["gst_enabled"] == "1", Code: out["gst_code"], Rate: out["gst_rate"], Inclusive: out["gst_inclusive"] == "1", Export: out["gst_export"] == "1"}
	if err := invoices.ValidateProduct(p); err != nil {
		return nil, server.BadRequestError(err, "Check GST invoicing settings", err.Error())
	}
	if p.Enabled {
		s, err := invoices.LoadSettings()
		if errors.Is(err, sql.ErrNoRows) {
			return nil, server.BadRequestError(fmt.Errorf("seller invoicing settings have not been saved: %w", err), "Set up invoicing first", "Save your seller details in Invoicing settings (/admin/invoicing), then return to this product and enable GST invoicing. Your product has not been changed.")
		}
		if err != nil {
			return nil, server.InternalError(err, "Unable to load invoicing settings", "The seller invoicing settings could not be loaded. Check the server logs and try again.")
		}
		if err = s.Validate(); err != nil {
			return nil, server.BadRequestError(err, "Complete invoicing settings", "Update your seller details in Invoicing settings (/admin/invoicing) before enabling GST invoicing: "+err.Error())
		}
	}
	return out, nil
}

func HandleGSTTax(w http.ResponseWriter, r *http.Request) error {
	params, err := authoriseCreateToggle(w, r)
	if err != nil {
		return err
	}
	index := params.Get("fieldIndex")
	if len(index) > 6 {
		return server.BadRequestError(errors.New("invalid field index"))
	}
	value := params.Get("paypal_tax_" + index)
	if value == "" {
		value = "0"
	}
	enabled := params.Get("gst_enabled") == "on"
	if enabled && params.Get("gst_rate") != "" {
		currency := params.Get("paypal_currency_" + index)
		if currency == "" {
			currency = "INR"
		}
		value, err = subscriptions.ConfiguredGSTTax(params.Get("paypal_amount_"+index), currency, params.Get("paypal_country_"+index), invoices.Product{Enabled: true, Code: params.Get("gst_code"), Rate: params.Get("gst_rate"), Inclusive: params.Get("gst_inclusive") == "on"})
		if err != nil {
			return server.BadRequestError(err)
		}
	}
	v := view.NewRenderer(w, r)
	v.Layout("")
	v.AddKey("fieldIndex", index)
	v.AddKey("tax", value)
	v.AddKey("readonly", enabled)
	v.Template("products/views/gst_tax_input.html.got")
	return v.Render()
}

// HandleToggleGST mirrors gateway create toggles: fields exist only while enabled.
func HandleToggleGST(w http.ResponseWriter, r *http.Request) error {
	params, err := authoriseCreateToggle(w, r)
	if err != nil {
		return err
	}
	return renderGSTFields(w, r, params.Get("gst_enabled") == "on", products.New())
}

// HandleToggleGSTUpdate authorizes the owner before rendering saved GST settings.
func HandleToggleGSTUpdate(w http.ResponseWriter, r *http.Request) error {
	params, product, err := authoriseUpdateToggle(w, r)
	if err != nil {
		return err
	}
	return renderGSTFields(w, r, params.Get("gst_enabled") == "on", product)
}

func renderGSTFields(w http.ResponseWriter, r *http.Request, enabled bool, product *products.Story) error {
	// PayPal pricing fields may have loaded before the GST inputs existed.
	w.Header().Set("HX-Trigger-After-Swap", "gstPricingChanged")
	if product.GSTEnabled == 0 && product.GSTCode == "" && product.GSTRate == "" {
		product.GSTInclusive = 1
	}
	if !enabled {
		w.WriteHeader(http.StatusOK)
		return nil
	}
	settings, err := invoices.LoadSettings()
	setupRequired := errors.Is(err, sql.ErrNoRows)
	if err != nil && !setupRequired {
		return server.InternalError(err)
	}
	setupRequired = setupRequired || settings.Validate() != nil
	v := view.NewRenderer(w, r)
	v.Layout("")
	v.AddKey("story", product)
	v.AddKey("gstSetupRequired", setupRequired)
	v.Template("products/views/gst_fields.html.got")
	return v.Render()
}
