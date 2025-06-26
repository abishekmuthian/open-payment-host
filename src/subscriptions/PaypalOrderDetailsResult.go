package subscriptions

import "time"

type PayPalOrderDetailsResult struct {
	ID            string `json:"id,omitempty"`
	Intent        string `json:"intent,omitempty"`
	Status        string `json:"status,omitempty"`
	PaymentSource struct {
		Paypal struct {
			EmailAddress  string `json:"email_address,omitempty"`
			AccountID     string `json:"account_id,omitempty"`
			AccountStatus string `json:"account_status,omitempty"`
			Name          struct {
				GivenName string `json:"given_name,omitempty"`
				Surname   string `json:"surname,omitempty"`
			} `json:"name,omitempty"`
			Address struct {
				CountryCode string `json:"country_code,omitempty"`
			} `json:"address,omitempty"`
		} `json:"paypal,omitempty"`
	} `json:"payment_source,omitempty"`
	PurchaseUnits []struct {
		ReferenceID string `json:"reference_id,omitempty"`
		Amount      struct {
			CurrencyCode string `json:"currency_code,omitempty"`
			Value        string `json:"value,omitempty"`
			Breakdown    struct {
			} `json:"breakdown,omitempty"`
		} `json:"amount,omitempty"`
		Payee struct {
			EmailAddress string `json:"email_address,omitempty"`
			MerchantID   string `json:"merchant_id,omitempty"`
		} `json:"payee,omitempty"`
		SoftDescriptor string `json:"soft_descriptor,omitempty"`
		Shipping       struct {
			Name struct {
				FullName string `json:"full_name,omitempty"`
			} `json:"name,omitempty"`
			Address struct {
				AddressLine1 string `json:"address_line_1,omitempty"`
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
				FinalCapture     bool `json:"final_capture,omitempty"`
				SellerProtection struct {
					Status            string   `json:"status,omitempty"`
					DisputeCategories []string `json:"dispute_categories,omitempty"`
				} `json:"seller_protection,omitempty"`
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
				Links []struct {
					Href   string `json:"href,omitempty"`
					Rel    string `json:"rel,omitempty"`
					Method string `json:"method,omitempty"`
				} `json:"links,omitempty"`
				CreateTime time.Time `json:"create_time,omitempty"`
				UpdateTime time.Time `json:"update_time,omitempty"`
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
		Address      struct {
			CountryCode string `json:"country_code,omitempty"`
		} `json:"address,omitempty"`
	} `json:"payer,omitempty"`
	CreateTime time.Time `json:"create_time,omitempty"`
	UpdateTime time.Time `json:"update_time,omitempty"`
	Links      []struct {
		Href   string `json:"href,omitempty"`
		Rel    string `json:"rel,omitempty"`
		Method string `json:"method,omitempty"`
	} `json:"links,omitempty"`
}
