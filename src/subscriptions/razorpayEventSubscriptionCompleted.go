package subscriptions

type RazorpayEventSubscriptionCompleted struct {
	Entity    string   `json:"entity,omitempty"`
	AccountID string   `json:"account_id,omitempty"`
	Event     string   `json:"event,omitempty"`
	Contains  []string `json:"contains,omitempty"`
	Payload   struct {
		Subscription struct {
			Entity struct {
				ID                  string `json:"id,omitempty"`
				Entity              string `json:"entity,omitempty"`
				PlanID              string `json:"plan_id,omitempty"`
				CustomerID          string `json:"customer_id,omitempty"`
				Status              string `json:"status,omitempty"`
				Type                int    `json:"type,omitempty"`
				CurrentStart        int    `json:"current_start,omitempty"`
				CurrentEnd          int    `json:"current_end,omitempty"`
				EndedAt             int    `json:"ended_at,omitempty"`
				Quantity            int    `json:"quantity,omitempty"`
				Notes               []any  `json:"notes,omitempty"`
				ChargeAt            any    `json:"charge_at,omitempty"`
				StartAt             int    `json:"start_at,omitempty"`
				EndAt               int    `json:"end_at,omitempty"`
				AuthAttempts        int    `json:"auth_attempts,omitempty"`
				TotalCount          int    `json:"total_count,omitempty"`
				PaidCount           int    `json:"paid_count,omitempty"`
				CustomerNotify      bool   `json:"customer_notify,omitempty"`
				CreatedAt           int64  `json:"created_at,omitempty"`
				ExpireBy            int    `json:"expire_by,omitempty"`
				ShortURL            any    `json:"short_url,omitempty"`
				HasScheduledChanges bool   `json:"has_scheduled_changes,omitempty"`
				ChangeScheduledAt   any    `json:"change_scheduled_at,omitempty"`
				Source              string `json:"source,omitempty"`
				OfferID             string `json:"offer_id,omitempty"`
				RemainingCount      int    `json:"remaining_count,omitempty"`
			} `json:"entity,omitempty"`
		} `json:"subscription,omitempty"`
		Payment struct {
			Entity struct {
				ID                string `json:"id,omitempty"`
				Entity            string `json:"entity,omitempty"`
				Amount            int    `json:"amount,omitempty"`
				Currency          string `json:"currency,omitempty"`
				Status            string `json:"status,omitempty"`
				OrderID           string `json:"order_id,omitempty"`
				InvoiceID         string `json:"invoice_id,omitempty"`
				International     bool   `json:"international,omitempty"`
				Method            string `json:"method,omitempty"`
				AmountRefunded    int    `json:"amount_refunded,omitempty"`
				AmountTransferred int    `json:"amount_transferred,omitempty"`
				RefundStatus      any    `json:"refund_status,omitempty"`
				Captured          string `json:"captured,omitempty"`
				Description       string `json:"description,omitempty"`
				CardID            string `json:"card_id,omitempty"`
				Card              struct {
					ID            string `json:"id,omitempty"`
					Entity        string `json:"entity,omitempty"`
					Name          string `json:"name,omitempty"`
					Last4         string `json:"last4,omitempty"`
					Network       string `json:"network,omitempty"`
					Type          string `json:"type,omitempty"`
					Issuer        string `json:"issuer,omitempty"`
					International bool   `json:"international,omitempty"`
					Emi           bool   `json:"emi,omitempty"`
					ExpiryMonth   int    `json:"expiry_month,omitempty"`
					ExpiryYear    int    `json:"expiry_year,omitempty"`
				} `json:"card,omitempty"`
				Bank             any            `json:"bank,omitempty"`
				Wallet           any            `json:"wallet,omitempty"`
				Vpa              any            `json:"vpa,omitempty"`
				Email            string         `json:"email,omitempty"`
				Contact          string         `json:"contact,omitempty"`
				CustomerID       string         `json:"customer_id,omitempty"`
				TokenID          any            `json:"token_id,omitempty"`
				Notes            map[string]any `json:"notes,omitempty"`
				Fee              int            `json:"fee,omitempty"`
				Tax              int            `json:"tax,omitempty"`
				ErrorCode        any            `json:"error_code,omitempty"`
				ErrorDescription any            `json:"error_description,omitempty"`
				CreatedAt        int64          `json:"created_at,omitempty"`
			} `json:"entity,omitempty"`
		} `json:"payment,omitempty"`
	} `json:"payload,omitempty"`
	CreatedAt int64 `json:"created_at,omitempty"`
}
