package subscriptions

import "time"

type PaypalCaptureOrderResult struct {
	ID            string `json:"id,omitempty"`
	Status        string `json:"status,omitempty"`
	PaymentSource struct {
		Paypal struct {
			Name struct {
				GivenName string `json:"given_name,omitempty"`
				Surname   string `json:"surname,omitempty"`
			} `json:"name,omitempty"`
			EmailAddress  string `json:"email_address,omitempty"`
			AccountID     string `json:"account_id,omitempty"`
			AccountStatus string `json:"account_status,omitempty"`
		} `json:"paypal,omitempty"`
	} `json:"payment_source,omitempty"`
	PurchaseUnits []struct {
		ReferenceID string `json:"reference_id,omitempty"`
		Shipping    struct {
			Address struct {
				AddressLine1 string `json:"address_line_1,omitempty"`
				AddressLine2 string `json:"address_line_2,omitempty"`
				AdminArea2   string `json:"admin_area_2,omitempty"`
				AdminArea1   string `json:"admin_area_1,omitempty"`
				PostalCode   string `json:"postal_code,omitempty"`
				CountryCode  string `json:"country_code,omitempty"`
			} `json:"address,omitempty"`
		} `json:"shipping,omitempty"`
		Payments struct {
			Captures []struct {
				ID     string `json:"id,omitempty"`
				Status string `json:"status,omitempty"`
				Amount struct {
					CurrencyCode string `json:"currency_code,omitempty"`
					Value        string `json:"value,omitempty"`
				} `json:"amount,omitempty"`
				SellerProtection struct {
					Status            string   `json:"status,omitempty"`
					DisputeCategories []string `json:"dispute_categories,omitempty"`
				} `json:"seller_protection,omitempty"`
				FinalCapture              bool   `json:"final_capture,omitempty"`
				DisbursementMode          string `json:"disbursement_mode,omitempty"`
				SellerReceivableBreakdown struct {
					GrossAmount struct {
						CurrencyCode string `json:"currency_code,omitempty"`
						Value        string `json:"value,omitempty"`
					} `json:"gross_amount,omitempty"`
					PaypalFee struct {
						CurrencyCode string `json:"currency_code,omitempty"`
						Value        string `json:"value,omitempty"`
					} `json:"paypal_fee,omitempty"`
					NetAmount struct {
						CurrencyCode string `json:"currency_code,omitempty"`
						Value        string `json:"value,omitempty"`
					} `json:"net_amount,omitempty"`
				} `json:"seller_receivable_breakdown,omitempty"`
				CreateTime time.Time `json:"create_time,omitempty"`
				UpdateTime time.Time `json:"update_time,omitempty"`
				Links      []struct {
					Href   string `json:"href,omitempty"`
					Rel    string `json:"rel,omitempty"`
					Method string `json:"method,omitempty"`
				} `json:"links,omitempty"`
			} `json:"captures,omitempty"`
		} `json:"payments,omitempty"`
	} `json:"purchase_units,omitempty"`
	Payer struct {
		Name struct {
			GivenName string `json:"given_name,omitempty"`
			Surname   string `json:"surname,omitempty"`
		} `json:"name,omitempty"`
		EmailAddress string `json:"email_address,omitempty"`
		PayerID      string `json:"payer_id,omitempty"`
	} `json:"payer,omitempty"`
	Links []struct {
		Href   string `json:"href,omitempty"`
		Rel    string `json:"rel,omitempty"`
		Method string `json:"method,omitempty"`
	} `json:"links,omitempty"`
}
