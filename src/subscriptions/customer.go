package subscriptions

import "time"

type CustomerModel struct {
	Customer struct {
		ID           string    `json:"id"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		GivenName    string    `json:"given_name"`
		FamilyName   string    `json:"family_name"`
		EmailAddress string    `json:"email_address"`
		Address      struct {
			AddressLine1                 string `json:"address_line_1"`
			AddressLine2                 string `json:"address_line_2"`
			Locality                     string `json:"locality"`
			AdministrativeDistrictLevel1 string `json:"administrative_district_level_1"`
			PostalCode                   string `json:"postal_code"`
			Country                      string `json:"country"`
		} `json:"address"`
		PhoneNumber string `json:"phone_number"`
		ReferenceID string `json:"reference_id"`
		Note        string `json:"note"`
		Preferences struct {
			EmailUnsubscribed bool `json:"email_unsubscribed"`
		} `json:"preferences"`
		CreationSource string `json:"creation_source"`
		Version        int    `json:"version"`
	} `json:"customer"`
}
