package subscriptions

import "time"

type CatalogModel struct {
	CatalogObject struct {
		Type                  string    `json:"type"`
		ID                    string    `json:"id"`
		UpdatedAt             time.Time `json:"updated_at"`
		CreatedAt             time.Time `json:"created_at"`
		Version               int64     `json:"version"`
		IsDeleted             bool      `json:"is_deleted"`
		PresentAtAllLocations bool      `json:"present_at_all_locations"`
		SubscriptionPlanData  struct {
			Name   string `json:"name"`
			Phases []struct {
				UID                 string `json:"uid"`
				Cadence             string `json:"cadence"`
				RecurringPriceMoney struct {
					Amount   int    `json:"amount"`
					Currency string `json:"currency"`
				} `json:"recurring_price_money"`
				Ordinal int `json:"ordinal"`
			} `json:"phases"`
		} `json:"subscription_plan_data"`
	} `json:"catalog_object"`
	IDMappings []struct {
		ClientObjectID string `json:"client_object_id"`
		ObjectID       string `json:"object_id"`
	} `json:"id_mappings"`
}
