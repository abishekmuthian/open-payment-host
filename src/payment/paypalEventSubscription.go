package payment

import "time"

type PaypalEventSubscriptionModel struct {
	ID           string    `json:"id"`
	CreateTime   time.Time `json:"create_time"`
	ResourceType string    `json:"resource_type"`
	EventType    string    `json:"event_type"`
	Summary      string    `json:"summary"`
	Resource     struct {
		Quantity   string `json:"quantity"`
		Subscriber struct {
			Name struct {
				GivenName string `json:"given_name"`
				Surname   string `json:"surname"`
			} `json:"name"`
			EmailAddress    string `json:"email_address"`
			ShippingAddress struct {
				Name struct {
					FullName string `json:"full_name"`
				} `json:"name"`
				Address struct {
					AddressLine1 string `json:"address_line_1"`
					AddressLine2 string `json:"address_line_2"`
					AdminArea2   string `json:"admin_area_2"`
					AdminArea1   string `json:"admin_area_1"`
					PostalCode   string `json:"postal_code"`
					CountryCode  string `json:"country_code"`
				} `json:"address"`
			} `json:"shipping_address"`
		} `json:"subscriber"`
		CreateTime     time.Time `json:"create_time"`
		ShippingAmount struct {
			CurrencyCode string `json:"currency_code"`
			Value        string `json:"value"`
		} `json:"shipping_amount"`
		StartTime   time.Time `json:"start_time"`
		UpdateTime  time.Time `json:"update_time"`
		BillingInfo struct {
			OutstandingBalance struct {
				CurrencyCode string `json:"currency_code"`
				Value        string `json:"value"`
			} `json:"outstanding_balance"`
			CycleExecutions []struct {
				TenureType                  string `json:"tenure_type"`
				Sequence                    int    `json:"sequence"`
				CyclesCompleted             int    `json:"cycles_completed"`
				CyclesRemaining             int    `json:"cycles_remaining"`
				CurrentPricingSchemeVersion int    `json:"current_pricing_scheme_version"`
			} `json:"cycle_executions"`
			LastPayment struct {
				Amount struct {
					CurrencyCode string `json:"currency_code"`
					Value        string `json:"value"`
				} `json:"amount"`
				Time time.Time `json:"time"`
			} `json:"last_payment"`
			NextBillingTime     time.Time `json:"next_billing_time"`
			FinalPaymentTime    time.Time `json:"final_payment_time"`
			FailedPaymentsCount int       `json:"failed_payments_count"`
		} `json:"billing_info"`
		Links []struct {
			Href   string `json:"href"`
			Rel    string `json:"rel"`
			Method string `json:"method"`
		} `json:"links"`
		ID               string    `json:"id"`
		PlanID           string    `json:"plan_id"`
		AutoRenewal      bool      `json:"auto_renewal"`
		Status           string    `json:"status"`
		StatusUpdateTime time.Time `json:"status_update_time"`
	} `json:"resource"`
	Links []struct {
		Href    string `json:"href"`
		Rel     string `json:"rel"`
		Method  string `json:"method"`
		EncType string `json:"encType"`
	} `json:"links"`
	EventVersion    string `json:"event_version"`
	ResourceVersion string `json:"resource_version"`
}
