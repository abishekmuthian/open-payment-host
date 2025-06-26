package subscriptions

type RazorpayCancelSubscriptionResult struct {
	ID           string `json:"id,omitempty"`
	Entity       string `json:"entity,omitempty"`
	PlanID       string `json:"plan_id,omitempty"`
	CustomerID   string `json:"customer_id,omitempty"`
	Status       string `json:"status,omitempty"`
	CurrentStart int    `json:"current_start,omitempty"`
	CurrentEnd   int    `json:"current_end,omitempty"`
	EndedAt      int    `json:"ended_at,omitempty"`
	Quantity     int    `json:"quantity,omitempty"`
	Notes        struct {
		NotesKey1 string `json:"notes_key_1,omitempty"`
		NotesKey2 string `json:"notes_key_2,omitempty"`
	} `json:"notes,omitempty"`
	ChargeAt            int    `json:"charge_at,omitempty"`
	StartAt             int    `json:"start_at,omitempty"`
	EndAt               int    `json:"end_at,omitempty"`
	AuthAttempts        int    `json:"auth_attempts,omitempty"`
	TotalCount          int    `json:"total_count,omitempty"`
	PaidCount           int    `json:"paid_count,omitempty"`
	CustomerNotify      bool   `json:"customer_notify,omitempty"`
	CreatedAt           int    `json:"created_at,omitempty"`
	ExpireBy            int    `json:"expire_by,omitempty"`
	ShortURL            string `json:"short_url,omitempty"`
	HasScheduledChanges bool   `json:"has_scheduled_changes,omitempty"`
	ChangeScheduledAt   any    `json:"change_scheduled_at,omitempty"`
	Source              string `json:"source,omitempty"`
	OfferID             string `json:"offer_id,omitempty"`
	RemainingCount      int    `json:"remaining_count,omitempty"`
}
