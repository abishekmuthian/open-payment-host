package subscriptions

import "time"

type PaypalSubscriptionTransaction struct {
	Transactions []struct {
		ID         string `json:"id,omitempty"`
		Status     string `json:"status,omitempty"`
		PayerEmail string `json:"payer_email,omitempty"`
		PayerName  struct {
			GivenName string `json:"given_name,omitempty"`
			Surname   string `json:"surname,omitempty"`
		} `json:"payer_name,omitempty"`
		AmountWithBreakdown struct {
			GrossAmount struct {
				CurrencyCode string `json:"currency_code,omitempty"`
				Value        string `json:"value,omitempty"`
			} `json:"gross_amount,omitempty"`
			TaxAmount struct {
				CurrencyCode string `json:"currency_code"`
				Value        string `json:"value"`
			} `json:"tax_amount"`
			FeeAmount struct {
				CurrencyCode string `json:"currency_code,omitempty"`
				Value        string `json:"value,omitempty"`
			} `json:"fee_amount,omitempty"`
			NetAmount struct {
				CurrencyCode string `json:"currency_code,omitempty"`
				Value        string `json:"value,omitempty"`
			} `json:"net_amount,omitempty"`
		} `json:"amount_with_breakdown,omitempty"`
		Time time.Time `json:"time,omitempty"`
	} `json:"transactions,omitempty"`
	Links []struct {
		Href   string `json:"href,omitempty"`
		Rel    string `json:"rel,omitempty"`
		Method string `json:"method,omitempty"`
	} `json:"links,omitempty"`
}
