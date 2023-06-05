package subscriptions

type CardModel struct {
	Card struct {
		ID             string `json:"id"`
		BillingAddress struct {
			AddressLine1                 string `json:"address_line_1"`
			AddressLine2                 string `json:"address_line_2"`
			Locality                     string `json:"locality"`
			AdministrativeDistrictLevel1 string `json:"administrative_district_level_1"`
			PostalCode                   string `json:"postal_code"`
			Country                      string `json:"country"`
		} `json:"billing_address"`
		Fingerprint    string `json:"fingerprint"`
		Bin            string `json:"bin"`
		CardBrand      string `json:"card_brand"`
		CardType       string `json:"card_type"`
		CardholderName string `json:"cardholder_name"`
		CustomerID     string `json:"customer_id"`
		Enabled        bool   `json:"enabled"`
		ExpMonth       int    `json:"exp_month"`
		ExpYear        int    `json:"exp_year"`
		Last4          string `json:"last_4"`
		MerchantID     string `json:"merchant_id"`
		PrepaidType    string `json:"prepaid_type"`
		ReferenceID    string `json:"reference_id"`
		Version        int    `json:"version"`
	} `json:"card"`
}
