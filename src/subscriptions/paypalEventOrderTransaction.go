package subscriptions

type PaypalEventOrderTransaction struct {
	TransactionDetails []struct {
		TransactionInfo struct {
			PaypalAccountID           string `json:"paypal_account_id"`
			TransactionID             string `json:"transaction_id"`
			TransactionEventCode      string `json:"transaction_event_code"`
			TransactionInitiationDate string `json:"transaction_initiation_date"`
			TransactionUpdatedDate    string `json:"transaction_updated_date"`
			TransactionAmount         struct {
				CurrencyCode string `json:"currency_code"`
				Value        string `json:"value"`
			} `json:"transaction_amount"`
			FeeAmount struct {
				CurrencyCode string `json:"currency_code"`
				Value        string `json:"value"`
			} `json:"fee_amount"`
			TransactionStatus     string `json:"transaction_status"`
			ProtectionEligibility string `json:"protection_eligibility"`
		} `json:"transaction_info"`
		PayerInfo struct {
			AccountID     string `json:"account_id"`
			EmailAddress  string `json:"email_address"`
			AddressStatus string `json:"address_status"`
			PayerStatus   string `json:"payer_status"`
			PayerName     struct {
				GivenName         string `json:"given_name"`
				Surname           string `json:"surname"`
				AlternateFullName string `json:"alternate_full_name"`
			} `json:"payer_name"`
			CountryCode string `json:"country_code"`
		} `json:"payer_info"`
		ShippingInfo struct {
			Name    string `json:"name"`
			Method  string `json:"method"`
			Address struct {
				Line1       string `json:"line1"`
				City        string `json:"city"`
				CountryCode string `json:"country_code"`
				PostalCode  string `json:"postal_code"`
			} `json:"address"`
		} `json:"shipping_info"`
		CartInfo struct {
			ItemDetails []struct {
				ItemCode      string `json:"item_code"`
				ItemName      string `json:"item_name"`
				ItemQuantity  string `json:"item_quantity"`
				ItemUnitPrice struct {
					CurrencyCode string `json:"currency_code"`
					Value        string `json:"value"`
				} `json:"item_unit_price"`
				ItemAmount struct {
					CurrencyCode string `json:"currency_code"`
					Value        string `json:"value"`
				} `json:"item_amount"`
				TaxAmounts []struct {
					TaxAmount struct {
						CurrencyCode string `json:"currency_code"`
						Value        string `json:"value"`
					} `json:"tax_amount"`
				} `json:"tax_amounts"`
				BasicShippingAmount struct {
					CurrencyCode string `json:"currency_code"`
					Value        string `json:"value"`
				} `json:"basic_shipping_amount"`
				TotalItemAmount struct {
					CurrencyCode string `json:"currency_code"`
					Value        string `json:"value"`
				} `json:"total_item_amount"`
			} `json:"item_details"`
		} `json:"cart_info"`
		StoreInfo struct {
		} `json:"store_info"`
		AuctionInfo struct {
			AuctionSite        string `json:"auction_site"`
			AuctionItemSite    string `json:"auction_item_site"`
			AuctionBuyerID     string `json:"auction_buyer_id"`
			AuctionClosingDate string `json:"auction_closing_date"`
		} `json:"auction_info"`
		IncentiveInfo struct {
		} `json:"incentive_info"`
	} `json:"transaction_details"`
	AccountNumber         string `json:"account_number"`
	LastRefreshedDatetime string `json:"last_refreshed_datetime"`
	Page                  int    `json:"page"`
	TotalItems            int    `json:"total_items"`
	TotalPages            int    `json:"total_pages"`
	Links                 []struct {
		Href   string `json:"href"`
		Rel    string `json:"rel"`
		Method string `json:"method"`
	} `json:"links"`
}
