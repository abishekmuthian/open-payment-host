package subscriptions

type RazorpayWebhookEvent struct {
	Entity    string `json:"entity,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	Event     string `json:"event,omitempty"`
	CreatedAt int    `json:"created_at,omitempty"`
}
