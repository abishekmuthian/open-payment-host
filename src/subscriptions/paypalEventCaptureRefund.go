package subscriptions

import "time"

type PaypalEventCaptureRefund struct {
	ID           string    `json:"id,omitempty"`
	CreateTime   time.Time `json:"create_time,omitempty"`
	ResourceType string    `json:"resource_type,omitempty"`
	EventType    string    `json:"event_type,omitempty"`
	Summary      string    `json:"summary,omitempty"`
	Resource     struct {
		SellerPayableBreakdown struct {
			TotalRefundedAmount struct {
				Value        string `json:"value,omitempty"`
				CurrencyCode string `json:"currency_code,omitempty"`
			} `json:"total_refunded_amount,omitempty"`
			PaypalFee struct {
				Value        string `json:"value,omitempty"`
				CurrencyCode string `json:"currency_code,omitempty"`
			} `json:"paypal_fee,omitempty"`
			GrossAmount struct {
				Value        string `json:"value,omitempty"`
				CurrencyCode string `json:"currency_code,omitempty"`
			} `json:"gross_amount,omitempty"`
			NetAmount struct {
				Value        string `json:"value,omitempty"`
				CurrencyCode string `json:"currency_code,omitempty"`
			} `json:"net_amount,omitempty"`
		} `json:"seller_payable_breakdown,omitempty"`
		Amount struct {
			Value        string `json:"value,omitempty"`
			CurrencyCode string `json:"currency_code,omitempty"`
		} `json:"amount,omitempty"`
		UpdateTime string `json:"update_time,omitempty"`
		CreateTime string `json:"create_time,omitempty"`
		CustomID   string `json:"custom_id,omitempty"`
		Links      []struct {
			Method string `json:"method,omitempty"`
			Rel    string `json:"rel,omitempty"`
			Href   string `json:"href,omitempty"`
		} `json:"links,omitempty"`
		ID    string `json:"id,omitempty"`
		Payer struct {
			EmailAddress string `json:"email_address,omitempty"`
			MerchantID   string `json:"merchant_id,omitempty"`
		} `json:"payer,omitempty"`
		Status string `json:"status,omitempty"`
	} `json:"resource,omitempty"`
	Status        string `json:"status,omitempty"`
	Transmissions []struct {
		WebhookURL     string `json:"webhook_url,omitempty"`
		TransmissionID string `json:"transmission_id,omitempty"`
		Status         string `json:"status,omitempty"`
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
