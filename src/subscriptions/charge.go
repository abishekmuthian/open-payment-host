package subscriptions

import "time"

type Charge struct {
	Payment struct {
		ID          string    `json:"id"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
		AmountMoney struct {
			Amount   int    `json:"amount"`
			Currency string `json:"currency"`
		} `json:"amount_money"`
		Status        string `json:"status"`
		DelayDuration string `json:"delay_duration"`
		SourceType    string `json:"source_type"`
		CardDetails   struct {
			Status string `json:"status"`
			Card   struct {
				CardBrand   string `json:"card_brand"`
				Last4       string `json:"last_4"`
				ExpMonth    int    `json:"exp_month"`
				ExpYear     int    `json:"exp_year"`
				Fingerprint string `json:"fingerprint"`
				CardType    string `json:"card_type"`
				PrepaidType string `json:"prepaid_type"`
				Bin         string `json:"bin"`
			} `json:"card"`
			EntryMethod          string `json:"entry_method"`
			CvvStatus            string `json:"cvv_status"`
			AvsStatus            string `json:"avs_status"`
			StatementDescription string `json:"statement_description"`
			CardPaymentTimeline  struct {
				AuthorizedAt time.Time `json:"authorized_at"`
				CapturedAt   time.Time `json:"captured_at"`
			} `json:"card_payment_timeline"`
		} `json:"card_details"`
		LocationID string `json:"location_id"`
		OrderID    string `json:"order_id"`
		TotalMoney struct {
			Amount   int    `json:"amount"`
			Currency string `json:"currency"`
		} `json:"total_money"`
		ApprovedMoney struct {
			Amount   int    `json:"amount"`
			Currency string `json:"currency"`
		} `json:"approved_money"`
		ReceiptNumber      string    `json:"receipt_number"`
		ReceiptURL         string    `json:"receipt_url"`
		DelayAction        string    `json:"delay_action"`
		DelayedUntil       time.Time `json:"delayed_until"`
		ApplicationDetails struct {
			SquareProduct string `json:"square_product"`
			ApplicationID string `json:"application_id"`
		} `json:"application_details"`
		VersionToken string `json:"version_token"`
	} `json:"payment"`
}
