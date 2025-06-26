package subscriptions

import "time"

type PaypalWebhookEvent struct {
	ID              string    `json:"id,omitempty"`
	CreateTime      time.Time `json:"create_time,omitempty"`
	ResourceType    string    `json:"resource_type,omitempty"`
	EventType       string    `json:"event_type,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	Status          string    `json:"status,omitempty"`
	EventVersion    string    `json:"event_version,omitempty"`
	ResourceVersion string    `json:"resource_version,omitempty"`
}
