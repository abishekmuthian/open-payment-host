package subscriptions

type RazorpayEventOrderPaid struct {
	Entity    string   `json:"entity,omitempty"`
	AccountID string   `json:"account_id,omitempty"`
	Event     string   `json:"event,omitempty"`
	Contains  []string `json:"contains,omitempty"`
	Payload   struct {
		Payment struct {
			Entity struct {
				ID               string         `json:"id,omitempty"`
				Entity           string         `json:"entity,omitempty"`
				Amount           int            `json:"amount,omitempty"`
				Currency         string         `json:"currency,omitempty"`
				Status           string         `json:"status,omitempty"`
				OrderID          string         `json:"order_id,omitempty"`
				InvoiceID        any            `json:"invoice_id,omitempty"`
				International    bool           `json:"international,omitempty"`
				Method           string         `json:"method,omitempty"`
				AmountRefunded   int            `json:"amount_refunded,omitempty"`
				RefundStatus     any            `json:"refund_status,omitempty"`
				Captured         bool           `json:"captured,omitempty"`
				Description      any            `json:"description,omitempty"`
				CardID           any            `json:"card_id,omitempty"`
				Bank             string         `json:"bank,omitempty"`
				Wallet           any            `json:"wallet,omitempty"`
				Vpa              any            `json:"vpa,omitempty"`
				Email            string         `json:"email,omitempty"`
				Name             string         `json:"name,omitempty"`
				Contact          string         `json:"contact,omitempty"`
				Notes            map[string]any `json:"notes,omitempty"`
				Fee              int            `json:"fee,omitempty"`
				Tax              int            `json:"tax,omitempty"`
				ErrorCode        any            `json:"error_code,omitempty"`
				ErrorDescription any            `json:"error_description,omitempty"`
				CreatedAt        int            `json:"created_at,omitempty"`
			} `json:"entity,omitempty"`
		} `json:"payment,omitempty"`
		Order struct {
			Entity struct {
				ID         string `json:"id,omitempty"`
				Entity     string `json:"entity,omitempty"`
				Amount     int    `json:"amount,omitempty"`
				AmountPaid int    `json:"amount_paid,omitempty"`
				AmountDue  int    `json:"amount_due,omitempty"`
				Currency   string `json:"currency,omitempty"`
				Receipt    string `json:"receipt,omitempty"`
				OfferID    any    `json:"offer_id,omitempty"`
				Status     string `json:"status,omitempty"`
				Attempts   int    `json:"attempts,omitempty"`
				Notes      []any  `json:"notes,omitempty"`
				CreatedAt  int64  `json:"created_at,omitempty"`
			} `json:"entity,omitempty"`
		} `json:"order,omitempty"`
	} `json:"payload,omitempty"`
	CreatedAt int64 `json:"created_at,omitempty"`
}
