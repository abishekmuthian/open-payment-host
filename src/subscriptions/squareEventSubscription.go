package subscriptions

import "time"

type EventSubscriptionModel struct {
	MerchantID string    `json:"merchant_id"`
	Type       string    `json:"type"`
	EventID    string    `json:"event_id"`
	CreatedAt  time.Time `json:"created_at"`
	Data       struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		Object struct {
			Subscription struct {
				ID            string `json:"id"`
				CreatedDate   string `json:"created_date"`
				CustomerID    string `json:"customer_id"`
				LocationID    string `json:"location_id"`
				PlanID        string `json:"plan_id"`
				StartDate     string `json:"start_date"`
				Status        string `json:"status"`
				TaxPercentage string `json:"tax_percentage"`
				Timezone      string `json:"timezone"`
				Version       int64  `json:"version"`
			} `json:"subscription"`
		} `json:"object"`
	} `json:"data"`
}

type EventPaymentModel struct {
	MerchantID string    `json:"merchant_id"`
	Type       string    `json:"type"`
	EventID    string    `json:"event_id"`
	CreatedAt  time.Time `json:"created_at"`
	Data       struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		Object struct {
			Payment struct {
				ID                 string `json:"id"`
				CreatedAt          string `json:"created_at"`
				UpdatedAt          string `json:"updated_at"`
				AmountMoney        struct {
					Amount   int64  `json:"amount"`
					Currency string `json:"currency"`
				} `json:"amount_money"`
				TotalMoney         struct {
					Amount   int64  `json:"amount"`
					Currency string `json:"currency"`
				} `json:"total_money"`
				Status             string `json:"status"`
				SourceType         string `json:"source_type"`
				CustomerID         string `json:"customer_id"`
				LocationID         string `json:"location_id"`
				OrderID            string `json:"order_id"`
				ReferenceID        string `json:"reference_id"`
				ReceiptNumber      string `json:"receipt_number"`
				ReceiptURL         string `json:"receipt_url"`
				VersionToken       string `json:"version_token"`
			} `json:"payment"`
		} `json:"object"`
	} `json:"data"`
}
