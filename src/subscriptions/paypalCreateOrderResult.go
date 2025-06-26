package subscriptions

type PaypalCreateOrderResult struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	PaymentSource struct {
		Paypal struct {
		} `json:"paypal"`
	} `json:"payment_source"`
	Links []struct {
		Href   string `json:"href"`
		Rel    string `json:"rel"`
		Method string `json:"method"`
	} `json:"links"`
}
