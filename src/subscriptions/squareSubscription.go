package subscriptions

import "time"

type SubscriptionModel struct {
	Subscription struct {
		ID         string    `json:"id"`
		LocationID string    `json:"location_id"`
		PlanID     string    `json:"plan_id"`
		CustomerID string    `json:"customer_id"`
		StartDate  string    `json:"start_date"`
		Status     string    `json:"status"`
		Version    int64     `json:"version"`
		CreatedAt  time.Time `json:"created_at"`
		CardID     string    `json:"card_id"`
		Timezone   string    `json:"timezone"`
	} `json:"subscription"`
}
