package subscriptions

import "time"

type PaypalEventCheckout struct {
	ID           string    `json:"id,omitempty"`
	CreateTime   time.Time `json:"create_time,omitempty"`
	ResourceType string    `json:"resource_type,omitempty"`
	EventType    string    `json:"event_type,omitempty"`
	Summary      string    `json:"summary,omitempty"`
	Resource     struct {
		UpdateTime    time.Time `json:"update_time,omitempty"`
		CreateTime    time.Time `json:"create_time,omitempty"`
		PurchaseUnits []struct {
			ReferenceID string `json:"reference_id,omitempty"`
			Amount      struct {
				CurrencyCode string `json:"currency_code,omitempty"`
				Value        string `json:"value,omitempty"`
				Breakdown    struct {
					ItemTotal struct {
						CurrencyCode string `json:"currency_code,omitempty"`
						Value        string `json:"value,omitempty"`
					} `json:"item_total,omitempty"`
					Shipping struct {
						CurrencyCode string `json:"currency_code,omitempty"`
						Value        string `json:"value,omitempty"`
					} `json:"shipping,omitempty"`
					Handling struct {
						CurrencyCode string `json:"currency_code,omitempty"`
						Value        string `json:"value,omitempty"`
					} `json:"handling,omitempty"`
					TaxTotal struct {
						CurrencyCode string `json:"currency_code,omitempty"`
						Value        string `json:"value,omitempty"`
					} `json:"tax_total,omitempty"`
					Insurance struct {
						CurrencyCode string `json:"currency_code,omitempty"`
						Value        string `json:"value,omitempty"`
					} `json:"insurance,omitempty"`
					ShippingDiscount struct {
						CurrencyCode string `json:"currency_code,omitempty"`
						Value        string `json:"value,omitempty"`
					} `json:"shipping_discount,omitempty"`
					Discount struct {
						CurrencyCode string `json:"currency_code,omitempty"`
						Value        string `json:"value,omitempty"`
					} `json:"discount,omitempty"`
				} `json:"breakdown,omitempty"`
			} `json:"amount,omitempty"`
			Items []struct {
				Name        string `json:"name,omitempty"`
				Description string `json:"description,omitempty"`
				UnitAmount  struct {
					CurrencyCode string `json:"currency_code,omitempty"`
					Value        string `json:"value,omitempty"`
				} `json:"unit_amount,omitempty"`
				Quantity string `json:"quantity,omitempty"`
				Category string `json:"category,omitempty"`
				Sku      string `json:"sku,omitempty"`
				ImageURL string `json:"image_url,omitempty"`
				URL      string `json:"url,omitempty"`
				Upc      struct {
					Type string `json:"type,omitempty"`
					Code string `json:"code,omitempty"`
				} `json:"upc,omitempty"`
			} `json:"items,omitempty"`
			Payee struct {
				EmailAddress string `json:"email_address,omitempty"`
				MerchantID   string `json:"merchant_id,omitempty"`
			} `json:"payee,omitempty"`
			CustomID string `json:"custom_id,omitempty"`
			Shipping struct {
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
					CustomID string `json:"custom_id,omitempty"`
					Links    []struct {
						Href   string `json:"href,omitempty"`
						Rel    string `json:"rel,omitempty"`
						Method string `json:"method,omitempty"`
					} `json:"links,omitempty"`
					CreateTime time.Time `json:"create_time,omitempty"`
					UpdateTime time.Time `json:"update_time,omitempty"`
				} `json:"captures,omitempty"`
			} `json:"payments,omitempty"`
		} `json:"purchase_units,omitempty"`
		Links []struct {
			Href   string `json:"href,omitempty"`
			Rel    string `json:"rel,omitempty"`
			Method string `json:"method,omitempty"`
		} `json:"links,omitempty"`
		ID            string `json:"id,omitempty"`
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
		Intent string `json:"intent,omitempty"`
		Payer  struct {
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
		Status string `json:"status,omitempty"`
	} `json:"resource,omitempty"`
	Status        string `json:"status,omitempty"`
	Transmissions []struct {
		WebhookURL      string `json:"webhook_url,omitempty"`
		HTTPStatus      int    `json:"http_status,omitempty"`
		ReasonPhrase    string `json:"reason_phrase,omitempty"`
		ResponseHeaders struct {
			TransferEncoding        string `json:"Transfer-Encoding,omitempty"`
			ReferrerPolicy          string `json:"Referrer-Policy,omitempty"`
			StrictTransportSecurity string `json:"Strict-Transport-Security,omitempty"`
			XContentTypeOptions     string `json:"X-Content-Type-Options,omitempty"`
			XXSSProtection          string `json:"X-Xss-Protection,omitempty"`
			ContentSecurityPolicy   string `json:"Content-Security-Policy,omitempty"`
			Vary                    string `json:"Vary,omitempty"`
			NgrokAgentIps           string `json:"Ngrok-Agent-Ips,omitempty"`
			Date                    string `json:"Date,omitempty"`
		} `json:"response_headers,omitempty"`
		TransmissionID string    `json:"transmission_id,omitempty"`
		Status         string    `json:"status,omitempty"`
		Timestamp      time.Time `json:"timestamp,omitempty"`
	} `json:"transmissions,omitempty"`
	Links []struct {
		Href    string `json:"href,omitempty"`
		Rel     string `json:"rel,omitempty"`
		Method  string `json:"method,omitempty"`
		EncType string `json:"encType,omitempty"`
	} `json:"links,omitempty"`
	EventVersion    string `json:"event_version,omitempty"`
	ResourceVersion string `json:"resource_version,omitempty"`
}
