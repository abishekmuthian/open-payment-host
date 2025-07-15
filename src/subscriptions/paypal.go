package subscriptions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/google/uuid"
)

func HandlePaypalShow(w http.ResponseWriter, r *http.Request) error {
	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Get current user
	currentUser := session.CurrentUser(w, r)

	productId := params.GetInt("product_id")

	product, err := products.Find(productId)
	if err != nil {
		// Handle the error appropriately
		log.Error(log.V{"Error finding product with ID": productId, "error": err})
		return server.InternalError(err)
	}

	// Get the country from IP
	clientCountry := r.Header.Get("CF-IPCountry")
	if !config.Production() {
		// There will be no CF request header in the development/test
		clientCountry = config.Get("subscription_client_country")
	}

	log.Info(log.V{"Subscription, Client Country": clientCountry})

	amount := product.PaypalPrice[clientCountry]["amount"]
	currency := product.PaypalPrice[clientCountry]["currency"]

	// If there is no amount for the client country then get the amount for default country

	if amount == nil || currency == nil {
		clientCountry = "DF"
		amount = product.PaypalPrice[clientCountry]["amount"]
		currency = product.PaypalPrice[clientCountry]["currency"]
	}

	// Render the template
	view := view.NewRenderer(w, r)

	view.AddKey("currentUser", currentUser)

	switch product.Schedule {
	case "onetime":
		view.AddKey("price", fmt.Sprintf("%d %s/One Time", amount, currency))
		// Load the Paypal script
		view.AddKey("loadPaypalOneTimeScript", true)

		view.AddKey("meta_payment_script_type", "checkout")
	case "monthly":
		planId := product.PaypalPrice[clientCountry]["plan_id"]

		// Add paypal plan id
		view.AddKey("meta_plan_id", planId) // TODO: Retrieve the plan id from the product

		view.AddKey("price", fmt.Sprintf("%d %s/Monthly", amount, currency))
		view.AddKey("meta_payment_script_type", "subscription")
		view.AddKey("loadPaypalSubscriptionScript", true)
	}

	view.AddKey("currency", currency)

	if !config.Production() {
		view.AddKey("sandbox", true)
		view.AddKey("country", clientCountry)
	}
	view.AddKey("story", product)

	view.AddKey("loadSweetAlert", true)
	view.AddKey("meta_product_id", productId)
	view.AddKey("meta_product_amount", amount)

	// Add paypal client id
	view.AddKey("clientId", config.Get("paypal_client_id")) // Use this for Paypal subscription
	// view.AddKey("clientId", "BAA_37xNWO-_TYQABs_za4T-tDHEKnjtnx0H-pmTIVu4ByQ8IKQdYLGZ-frvwVTcVK6G7z6Bzkg0Zyr-f8")

	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())
	return view.Render()
}

// HandlePaypalCreateOrder creates order and returns order id.
// It responds to /subscriptions/paypal/orders
func HandlePaypalCreateOrder(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	productId := params.GetInt("product_id")
	customId := params.Get("custom_id")

	log.Info(log.V{"Creating order for product": productId})

	// Get the country from IP
	clientCountry := r.Header.Get("CF-IPCountry")
	if !config.Production() {
		// There will be no CF request header in the development/test
		clientCountry = config.Get("subscription_client_country")
	}

	log.Info(log.V{"Subscription, Client Country": clientCountry})

	product, err := products.Find(productId)
	if err != nil {
		// Handle the error appropriately
		log.Error(log.V{"Error finding product with ID": productId, "error": err})
		return server.InternalError(err)
	}

	amount := product.PaypalPrice[clientCountry]["amount"]
	currency := product.PaypalPrice[clientCountry]["currency"]
	tax := product.PaypalPrice[clientCountry]["tax"]

	// If there is no amount for the client country then get the amount for default country

	if amount == nil || currency == nil || tax == nil {
		amount = product.PaypalPrice["DF"]["amount"]
		currency = product.PaypalPrice["DF"]["currency"]
		tax = product.PaypalPrice["DF"]["tax"]
	}

	data := PaypalCreateOrder{
		Intent: "CAPTURE",
		PurchaseUnits: []PurchaseUnits{
			{
				CustomID: customId,
				Amount: Amount{
					CurrencyCode: currency.(string),
					Value:        fmt.Sprintf("%.2f", float64(amount.(float64))+float64(tax.(float64))),
					Breakdown: Breakdown{
						ItemTotal: ItemTotal{
							CurrencyCode: currency.(string),
							Value:        fmt.Sprintf("%.2f", amount),
						},
						TaxTotal: TaxTotal{
							CurrencyCode: currency.(string),
							Value:        fmt.Sprintf("%.2f", tax),
						},
					},
				},
				Items: []Items{
					{
						Name:        product.Name,
						Description: product.Description,
						Quantity:    1,
						Sku:         fmt.Sprintf("%d", product.ID),
						UnitAmount: UnitAmount{
							CurrencyCode: currency.(string),
							Value:        fmt.Sprintf("%.2f", amount),
						},
					},
				},
			},
		},
	}

	payloadBytes, err := json.Marshal(data)
	if err != nil {
		log.Error(log.V{"Error marshalling data": err})
	}
	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest(http.MethodPost, config.Get("paypal_api_domain")+"/v2/checkout/orders", body)
	if err != nil {
		log.Error(log.V{"Error sending request to create paypal order": err})
		return server.InternalError(err)
	}

	// Generate a new Version 4 UUID
	u, err := uuid.NewRandom()

	if err != nil {
		log.Error(log.V{"Error generating UUID": err})
		return server.InternalError(err)
	}

	accessToken, err := GetPaypalAuthorizationToken()

	if err != nil {
		log.Error(log.V{"Error getting access token": err})
		return server.InternalError(err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Paypal-Request-Id", u.String())
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("prefer", "return=minimal")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error(log.V{"Error sending request for creating paypal order": err})

	}
	defer resp.Body.Close()

	var paypalCreateOrderResult PaypalCreateOrderResult

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(log.V{"Paypal error reading order create response body": err})
		return server.InternalError(err)
	}

	err = json.Unmarshal(b, &paypalCreateOrderResult)

	if err != nil {
		log.Error(log.V{"Paypal error unmarshaling order create response": err})
		return server.InternalError(err)
	}

	// return the order ID in paypalCreateOrderResult as JSON
	return json.NewEncoder(w).Encode(paypalCreateOrderResult)
}

// HandlePaypalCaptureOrder creates order and returns order id.
// It responds to /subscriptions/paypal/orders/{id}/capture
func HandlePaypalCaptureOrder(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	orderId := params.Get("id")

	// This request doesn't require payload
	data := map[string]interface{}{}

	payloadBytes, err := json.Marshal(data)
	if err != nil {
		log.Error(log.V{"Error marshalling data": err})
	}
	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest(http.MethodPost, config.Get("paypal_api_domain")+"/v2/checkout/orders/"+orderId+"/capture", body)
	if err != nil {
		// handle err
		log.Error(log.V{"Error sending paypal order capture request": err})
	}

	// Generate a new Version 4 UUID
	u, err := uuid.NewRandom()

	if err != nil {
		log.Error(log.V{"Error generating UUID": err})
		return server.InternalError(err)
	}

	accessToken, err := GetPaypalAuthorizationToken()

	if err != nil {
		log.Error(log.V{"Error getting access token": err})
		return server.InternalError(err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Paypal-Request-Id", u.String())
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("prefer", "return=minimal")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error(log.V{"Error sending request for capturing paypal order": err})

	}
	defer resp.Body.Close()

	var paypalCaptureOrderResult PaypalCaptureOrderResult

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(log.V{"Paypal error reading order capture response body": err})
		return server.InternalError(err)
	}

	err = json.Unmarshal(b, &paypalCaptureOrderResult)

	if err != nil {
		log.Error(log.V{"Paypal error unmarshaling order capture response": err})
		return server.InternalError(err)
	}

	// return the order ID in paypalCreateOrderResult as JSON
	return json.NewEncoder(w).Encode(paypalCaptureOrderResult)
}

// GetPaypalAuthorizationToken fetches the bearer access token and returns it
func GetPaypalAuthorizationToken() (string, error) {

	type Token struct {
		Scope       string `json:"scope"`
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		AppID       string `json:"app_id"`
		ExpiresIn   int    `json:"expires_in"`
		Nonce       string `json:"nonce"`
	}
	params := url.Values{}
	params.Add("grant_type", `client_credentials`)
	body := strings.NewReader(params.Encode())

	req, err := http.NewRequest(http.MethodPost, config.Get("paypal_api_domain")+"/v1/oauth2/token", body)
	if err != nil {
		// handle err
		log.Error(log.V{"Error creating request for Paypal authorization": err})
	}
	req.SetBasicAuth(config.Get("paypal_client_id"), config.Get("paypal_client_secret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error(log.V{"Error creating Paypal authorization": err})
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(log.V{"Paypal authorization error": err})
		return "", err
	}

	var token Token

	err = json.Unmarshal(b, &token)

	if err != nil {
		log.Error(log.V{"Paypal authorization unmarshalling error": err})
		return "", err
	}

	return token.AccessToken, nil
}

// IsPaypalOrderValid checks if the give paypal order id is valid and was updated with last 1 hour
func IsPayPalOrderValid(orderId string) (bool, error) {
	// This request doesn't require payload
	data := map[string]interface{}{}

	payloadBytes, err := json.Marshal(data)
	if err != nil {
		log.Error(log.V{"Error marshalling data": err})
	}
	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest(http.MethodGet, config.Get("paypal_api_domain")+"/v2/checkout/orders/"+orderId, body)
	if err != nil {
		// handle err
		log.Error(log.V{"Error sending paypal order detail request": err})
	}

	// Generate a new Version 4 UUID
	u, err := uuid.NewRandom()

	if err != nil {
		log.Error(log.V{"Error generating UUID": err})
		return false, err
	}

	accessToken, err := GetPaypalAuthorizationToken()

	if err != nil {
		log.Error(log.V{"Error getting access token": err})
		return false, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Paypal-Request-Id", u.String())
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("prefer", "return=minimal")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error(log.V{"Error sending request for paypal order detail": err})

	}
	defer resp.Body.Close()

	var payPalOrderDetailsResult PayPalOrderDetailsResult

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(log.V{"Paypal error reading order detail response body": err})
		return false, err
	}

	err = json.Unmarshal(b, &payPalOrderDetailsResult)

	if err != nil {
		log.Error(log.V{"Paypal error unmarshaling order detail response": err})
		return false, err
	}

	if payPalOrderDetailsResult.PurchaseUnits[0].Payments.Captures[0].Status == "COMPLETED" {
		// Check if UpdatedTime is within past 1 hour
		if payPalOrderDetailsResult.UpdateTime.Before(time.Now().Add(-1 * time.Hour)) {
			return false, errors.New("transaction is older than 1 hour")
		}
		return true, nil
	}

	return false, err
}

func IsPaypalSubscriptionValid(transactionId string) (bool, error) {
	accessToken, err := GetPaypalAuthorizationToken()

	if err != nil {
		log.Error(log.V{"Error getting access token": err})
		return false, err
	}
	transaction, err := GetPaypalSubscriptionTransaction(transactionId, accessToken)

	if err != nil {
		log.Error(log.V{"Error finding subscription transaction": err})
		return false, err
	}

	if len(transaction.Transactions) > 0 {
		if transaction.Transactions[0].Status == "COMPLETED" {
			// Check if UpdatedTime is within past 1 hour
			if transaction.Transactions[0].Time.Before(time.Now().Add(-1 * time.Hour)) {
				return false, errors.New("transaction is older than 1 hour")
			}
			return true, nil
		}
	} else {
		return false, errors.New("no transaction found")
	}

	return false, err
}

// GetPaypalOrderTransaction fetches the paypal transaction details given the transaction id and access token
// Requires at least 3 hours for the transaction to appear in paypal's database
func GetPaypalOrderTransaction(transactionId string, accessToken string) (PaypalEventOrderTransaction, error) {

	currentTime := time.Now().UTC()
	startDate := currentTime.Add(-48 * time.Hour).Format("2006-01-02T15:04:05+0000")
	endDate := currentTime.Format("2006-01-02T15:04:05+0000")

	// URL encode startDate and endDate
	startDateEncoded := url.QueryEscape(startDate)
	endDateEncoded := url.QueryEscape(endDate)

	// transactionFields := "transaction_info,payer_info,cart_info"
	transactionFields := "all"

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/reporting/transactions?start_date=%s&end_date=%s&transaction_id=%s&fields=%s", config.Get("paypal_api_domain"), startDateEncoded, endDateEncoded, transactionId, transactionFields), nil)
	if err != nil {
		log.Error(log.V{"Error creating request for getting transaction": err})
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error(log.V{"Error sending request for getting transaction": err})
	}
	defer resp.Body.Close()

	var transaction PaypalEventOrderTransaction

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(log.V{"Paypal transaction search error": err})
		return transaction, err
	}

	err = json.Unmarshal(b, &transaction)

	if err != nil {
		log.Error(log.V{"Paypal transaction search unmarshaling error": err})
		return transaction, err
	}

	return transaction, nil
}

// GetPaypalSubscriptionTransaction fetches the paypal transaction details given the transaction id and access token
func GetPaypalSubscriptionTransaction(transactionId string, accessToken string) (PaypalSubscriptionTransaction, error) {

	currentTime := time.Now().UTC()
	startDate := currentTime.Add(-48 * time.Hour).Format("2006-01-02T15:04:05Z")
	endDate := currentTime.Format("2006-01-02T15:04:05Z")

	// URL encode startDate and endDate
	startDateEncoded := url.QueryEscape(startDate)
	endDateEncoded := url.QueryEscape(endDate)

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/billing/subscriptions/%s/transactions?start_time=%s&end_time=%s", config.Get("paypal_api_domain"), transactionId, startDateEncoded, endDateEncoded), nil)
	if err != nil {
		log.Error(log.V{"Error creating request for getting subscription transaction": err})
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error(log.V{"Error sending request for getting subscription transaction": err})
	}
	defer resp.Body.Close()

	var transaction PaypalSubscriptionTransaction

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(log.V{"Paypal subscription transaction search error": err})
		return transaction, err
	}

	err = json.Unmarshal(b, &transaction)

	if err != nil {
		log.Error(log.V{"Paypal subscription transaction search unmarshaling error": err})
		return transaction, err
	}

	return transaction, nil
}

func CancelPaypalSubscription(subscriptionId string) error {

	accessToken, err := GetPaypalAuthorizationToken()

	if err != nil {
		log.Error(log.V{"Error getting access token": err})
		return server.InternalError(err)
	}
	type Payload struct {
		Reason string `json:"reason"`
	}

	data := Payload{
		Reason: "User requested cancellation",
	}

	payloadBytes, err := json.Marshal(data)
	if err != nil {
		log.Error(log.V{"Paypal, Error marshalling payload": err})
	}

	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/v1/billing/subscriptions/%s/cancel", config.Get("paypal_api_domain"), subscriptionId), body)
	if err != nil {
		log.Error(log.V{"Error creating request for getting subscription cancellation": err})
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error(log.V{"Error sending request for subscription cancellation": err})
	}
	defer resp.Body.Close()

	// Check  if the response code is 204 No Content
	if resp.StatusCode == http.StatusNoContent {
		return nil
	} else {
		return fmt.Errorf("failed to cancel subscription, status code: %d", resp.StatusCode)
	}
}
