package subscriptions

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
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

// HandleSquareShow shows the web sdk payment page for the Square by responding to the GET request /subscriptions/square
func HandleSquareShow(w http.ResponseWriter, r *http.Request) error {
	paymentResponseHeaders(w)
	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Get current user
	currentUser := session.CurrentUser(w, r)

	productId := params.GetInt("productId")
	paymentType := params.Get("type")

	// Resolve the display price from the product configuration server-side.
	// Square amounts are minor units and are charged exactly as configured.
	product, productErr := products.Find(productId)
	if productErr != nil {
		return server.NotFoundError(productErr)
	}
	var priceLabel, verificationAmount, verificationCurrency string
	if productErr == nil {
		displayCountry := r.Header.Get("CF-IPCountry")
		if !config.Production() {
			displayCountry = config.Get("subscription_client_country")
		}
		amount := product.SquarePrice[displayCountry]
		if amount == nil {
			amount = product.SquarePrice["DF"]
		}
		if amount != nil && amount["amount"] != nil && amount["currency"] != nil {
			currency := amount["currency"].(string)
			minorAmount, err := configuredMinor(amount["amount"], currency, true)
			if err != nil {
				return server.BadRequestError(err)
			}
			verificationAmount = minorDecimal(minorAmount, currency)
			verificationCurrency = currency
			priceLabel = formatMinorUnits(minorAmount, currency)
			if paymentType == "onetime" {
				priceLabel = priceLabel + "/One Time"
			} else if paymentType == "subscription" {
				priceLabel = priceLabel + "/" + squareScheduleLabel(product.Schedule)
			}
		}
	}

	// Render the template
	view := view.NewRenderer(w, r)

	view.AddKey("currentUser", currentUser)
	view.AddKey("verificationAmount", verificationAmount)
	view.AddKey("verificationCurrency", verificationCurrency)
	view.AddKey("paymentSchedule", product.Schedule)

	if priceLabel != "" {
		view.AddKey("price", priceLabel)
	}

	view.AddKey("meta_app_id", config.Get("square_app_id"))
	view.AddKey("meta_location_id", config.Get("square_location_id"))

	// Load the Square script
	view.AddKey("loadSquareScript", true)

	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())
	return view.Render()
}

// squareScheduleLabel returns the display label for a Square subscription cadence
func squareScheduleLabel(schedule string) string {
	if schedule == "yearly" {
		return "Yearly"
	}
	return "Monthly"
}

// HandleSquare receives the POST request from the square web sdk at /subscriptions/square.
// The amount and currency are resolved from the product's configured country
// price on the server - posted amount and currency values are ignored. The
// attempt id is stored in reference_id and the returned completed Payment is
// validated for amount, currency and reference before fulfillment.
func HandleSquare(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	paymentToken := params.Get("paymentToken")
	verificationToken := params.Get("verificationToken")
	productId := params.GetInt("productId")
	email := params.Get("email")

	// Resolve the product and its configured price for the client country
	product, err := products.Find(productId)
	if err != nil {
		return server.InternalError(err)
	}

	clientCountry := r.Header.Get("CF-IPCountry")
	if !config.Production() {
		clientCountry = config.Get("subscription_client_country")
	}

	price := product.SquarePrice[clientCountry]
	if price == nil || price["amount"] == nil || price["currency"] == nil {
		clientCountry = "DF"
		price = product.SquarePrice[clientCountry]
	}

	if price == nil || price["amount"] == nil || price["currency"] == nil {
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=No+price+configured+for+your+region")
	}

	if product.Schedule != "onetime" {
		return server.BadRequestError(errors.New("product is not a one-time purchase"))
	}
	// Square amounts are already minor units and are charged exactly as configured
	amountMinor, err := configuredMinor(price["amount"], price["currency"].(string), true)
	if err != nil {
		return server.BadRequestError(err)
	}
	currency := price["currency"].(string)

	// Create the immutable payment attempt from the server-side configuration
	attempt, err := newAttempt(productId, "square", "onetime", clientCountry, amountMinor, currency, "", "", "")
	if err != nil {
		log.Error(log.V{"Square payment, error creating payment attempt": err})
		return server.InternalError(err)
	}

	// Generate a new Version 4 UUID
	u, err := uuid.NewRandom()
	if err != nil {
		return server.InternalError(err)
	}

	type AmountMoney struct {
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	}

	type Payload struct {
		IdempotencyKey    string      `json:"idempotency_key"`
		AmountMoney       AmountMoney `json:"amount_money"`
		SourceID          string      `json:"source_id"`
		VerificationToken string      `json:"verification_token"`
		ReferenceID       string      `json:"reference_id,omitempty"`
		BuyerEmailAddress string      `json:"buyer_email_address,omitempty"`
	}

	data := Payload{
		IdempotencyKey: u.String(),
		AmountMoney: AmountMoney{
			Amount:   amountMinor,
			Currency: currency,
		},
		SourceID:          paymentToken,
		VerificationToken: verificationToken,
		ReferenceID:       attempt.Id,
		BuyerEmailAddress: email,
	}
	payloadBytes, err := json.Marshal(data)
	if err != nil {
		return server.InternalError(err)
	}
	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest("POST", config.Get("square_domain")+"/payments", body)
	if err != nil {
		return server.InternalError(err)
	}
	req.Header.Set("Square-Version", "2023-04-19")
	req.Header.Set("Authorization", "Bearer "+config.Get("square_access_token"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := paymentHTTPClient.Do(req)
	if err != nil {
		return server.InternalError(err)
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Error(log.V{"square ioutil.ReadAll: %v": err})
		return err
	}

	if resp.StatusCode != 200 {
		var error ErrorModel

		err = json.Unmarshal(b, &error)

		if err != nil {
			log.Error(log.V{"Square Payment error JSON Unmarshall": err})
		}

		log.Info(log.V{"Square Payment parsed": error})

		return server.Redirect(w, r, "/subscriptions/failure?errorDetail="+strings.Replace(squareErrorDetail(error), ":", "", -1))
	}

	var charge Charge

	err = json.Unmarshal(b, &charge)

	if err != nil {
		log.Error(log.V{"Square Payment JSON Unmarshall": err})
	}

	log.Info(log.V{"Square Payment parsed": charge.Payment.ID, "status": charge.Payment.Status})

	// Validate the returned completed Payment: status, amount, currency and reference
	if charge.Payment.Status != "COMPLETED" {
		log.Error(log.V{"Square Payment not completed": charge.Payment.Status})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail="+"Payment failed try again later.")
	}

	if charge.Payment.ReferenceID != attempt.Id {
		log.Error(log.V{"Square Payment reference mismatch": charge.Payment.ReferenceID})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	if int64(charge.Payment.AmountMoney.Amount) != amountMinor || charge.Payment.AmountMoney.Currency != currency {
		log.Error(log.V{"Square Payment amount mismatch": charge.Payment.AmountMoney.Amount, "expected": amountMinor})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	if err := attempt.SetProviderIds("", charge.Payment.ID, ""); err != nil {
		return server.InternalError(err)
	}
	facts := ProviderFacts{
		Gateway:   "square",
		PaymentId: charge.Payment.ID,
		Amount:    int64(charge.Payment.AmountMoney.Amount),
		Currency:  charge.Payment.AmountMoney.Currency,
		Paid:      true,
		Email:     charge.Payment.BuyerEmailAddress,
	}

	// Bind the attempt to this browser before fulfillment
	setAttemptCookie(w, r, attempt)

	_, err = verifyAndFulfill(attempt, facts)
	if err != nil {
		log.Error(log.V{"Square payment, fulfillment rejected": err})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	// Exchange the completion token for a cookie and go to the clean success URL
	completionToken, err := generateToken(32)
	if err != nil {
		return server.InternalError(err)
	}
	attempt.CompletionTokenHash = hashToken(completionToken)
	if err := attempt.Save(); err != nil {
		log.Error(log.V{"Square payment, error saving completion token": err})
		return server.InternalError(err)
	}
	setCompletionCookie(w, r, completionToken)

	return server.Redirect(w, r, "/subscriptions/success?attempt_id="+attempt.Id)
}

// HandleCreateSubscription creates a subscription for the customer on POST request to /subscriptions/subscribe.
// The returned subscription id is stored in the attempt which stays pending
// until invoice.payment_made or a fetched completed Payment proves the first
// invoice was paid - an ACTIVE Square subscription alone does not authorize
// fulfillment.
func HandleCreateSubscription(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	paymentToken := params.Get("paymentToken")
	verificationToken := params.Get("verificationToken")
	productId := params.GetInt("productId")
	addressLine1 := params.Get("addressLine1")
	addressLine2 := params.Get("addressLine2")
	givenName := params.Get("givenName")
	email := params.Get("email")
	country := params.Get("country")
	city := params.Get("city")
	state := params.Get("state")
	postalCode := params.Get("postalcode")

	// Resolve the product and its configured price on the server
	product, err := products.Find(productId)
	if err != nil {
		return server.InternalError(err)
	}

	clientCountry := r.Header.Get("CF-IPCountry")
	if !config.Production() {
		clientCountry = config.Get("subscription_client_country")
	}

	squarePrice := product.SquarePrice[clientCountry]
	if squarePrice == nil || squarePrice["amount"] == nil {
		clientCountry = "DF"
		squarePrice = product.SquarePrice[clientCountry]
	}

	if product.Schedule != "monthly" && product.Schedule != "yearly" {
		return server.BadRequestError(errors.New("product is not a subscription"))
	}
	if squarePrice == nil || squarePrice["amount"] == nil || squarePrice["currency"] == nil {
		return server.BadRequestError(errors.New("missing Square price"))
	}
	expectedCurrency, _ := squarePrice["currency"].(string)
	expectedAmount, err := configuredMinor(squarePrice["amount"], expectedCurrency, true)
	if err != nil {
		return server.BadRequestError(err)
	}
	catalogId := product.SquareSubscriptionPlanId[clientCountry]
	if catalogId == "" {
		return server.BadRequestError(errors.New("missing matching Square plan"))
	}
	if err := validateSquarePlan(catalogId, product.Schedule, expectedAmount, expectedCurrency); err != nil {
		return server.BadRequestError(err)
	}
	attempt, err := newAttempt(productId, "square", product.Schedule, clientCountry, expectedAmount, expectedCurrency, catalogId, "", "")
	if err != nil {
		return server.InternalError(err)
	}

	// Bind the attempt to this browser so the pending status page is authorized
	setAttemptCookie(w, r, attempt)

	customerId, err := CreateCustomer(paymentToken, verificationToken, attempt.Id, addressLine1, addressLine2, givenName, email, country, city, state, postalCode)

	if err != nil {
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail="+strings.Replace(err.Error(), ":", "", -1))
	}

	log.Info(log.V{"Customer ID is: ": customerId})

	cardId, err := CreateCard(paymentToken, verificationToken, attempt.Id, addressLine1, addressLine2, givenName, email, country, city, state, postalCode, customerId)

	if err != nil {
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail="+strings.Replace(err.Error(), ":", "", -1))
	}

	log.Info(log.V{"Card ID is: ": cardId})

	subscriptionId, err := CreateSubscription(config.Get("square_location_id"), catalogId, customerId, cardId)

	if err != nil {
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail="+strings.Replace(err.Error(), ":", "", -1))
	}

	log.Info(log.V{"Subscription Id is: ": subscriptionId})

	// Store the subscription id in the attempt which remains pending until
	// the first invoice is paid
	if err := attempt.SetProviderIds("", "", subscriptionId); err != nil {
		log.Error(log.V{"Square subscription, error storing subscription id in attempt": err})
		return server.InternalError(err)
	}

	// No fulfillment here: the attempt stays pending until the Square webhook
	// proves the initial invoice was paid
	return server.Redirect(w, r, "/subscriptions/success?attempt_id="+attempt.Id)
}

// CreateCustomer creates a customer with the attempt id as reference
func CreateCustomer(paymentToken string, verificationToken string, attemptId string,
	addressLine1 string, addressLine2 string, givenName string, email string,
	country string, city string, state string, postalCode string) (string, error) {

	type Address struct {
		AddressLine1                 string `json:"address_line_1"`
		AddressLine2                 string `json:"address_line_2"`
		Locality                     string `json:"locality"`
		AdministrativeDistrictLevel1 string `json:"administrative_district_level_1"`
		PostalCode                   string `json:"postal_code"`
		Country                      string `json:"country"`
	}

	type Payload struct {
		GivenName    string  `json:"given_name"`
		EmailAddress string  `json:"email_address"`
		Address      Address `json:"address"`
		ReferenceID  string  `json:"reference_id"`
	}

	data := Payload{
		GivenName:    givenName,
		EmailAddress: email,
		Address: Address{
			AddressLine1:                 addressLine1,
			AddressLine2:                 addressLine2,
			Locality:                     city,
			AdministrativeDistrictLevel1: state,
			PostalCode:                   postalCode,
			Country:                      country,
		},
		ReferenceID: attemptId,
	}
	payloadBytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest("POST", config.Get("square_domain")+"/customers", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Square-Version", "2023-05-17")
	req.Header.Set("Authorization", "Bearer "+config.Get("square_access_token"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := paymentHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(log.V{"square ioutil.ReadAll: %v": err})
		return "", err
	}

	if resp.StatusCode != 200 {
		var error ErrorModel

		err = json.Unmarshal(b, &error)

		if err != nil {
			log.Error(log.V{"Square Payment error JSON Unmarshall": err})
		}

		log.Info(log.V{"Square Payment parsed": error})

		return "", errors.New(squareErrorDetail(error))
	}

	var customer CustomerModel

	err = json.Unmarshal(b, &customer)

	if err != nil {
		log.Error(log.V{"Square Payment JSON Unmarshall": err})
	}

	log.Info(log.V{"Square customer created": customer.Customer.ID})

	return customer.Customer.ID, err
}

// CreateCard creates a card with the customer
func CreateCard(paymentToken string, verificationToken string, attemptId string,
	addressLine1 string, addressLine2 string, givenName string, email string,
	country string, city string, state string, postalCode string, customerId string) (string, error) {

	type BillingAddress struct {
		AddressLine1                 string `json:"address_line_1"`
		AddressLine2                 string `json:"address_line_2"`
		Locality                     string `json:"locality"`
		AdministrativeDistrictLevel1 string `json:"administrative_district_level_1"`
		PostalCode                   string `json:"postal_code"`
		Country                      string `json:"country"`
	}
	type Card struct {
		BillingAddress BillingAddress `json:"billing_address"`
		CardholderName string         `json:"cardholder_name"`
		CustomerID     string         `json:"customer_id"`
		ReferenceID    string         `json:"reference_id"`
	}

	type Payload struct {
		IdempotencyKey    string `json:"idempotency_key"`
		SourceID          string `json:"source_id"`
		VerificationToken string `json:"verification_token"`
		Card              Card   `json:"card"`
	}

	// Generate a new Version 4 UUID
	u, err := uuid.NewRandom() // FIXME: Handle error

	if err != nil {
		log.Error(log.V{"Error generating UUID": err})
	}

	var data Payload

	if config.Get("square_sandbox_source_id") != "" {
		data = Payload{
			// fill struct
			IdempotencyKey: u.String(),
			SourceID:       config.Get("square_sandbox_source_id"),
			Card: Card{
				BillingAddress: BillingAddress{
					AddressLine1:                 addressLine1,
					AddressLine2:                 addressLine2,
					Locality:                     city,
					AdministrativeDistrictLevel1: state,
					PostalCode:                   postalCode,
					Country:                      country,
				},
				CardholderName: givenName,
				CustomerID:     customerId,
				ReferenceID:    attemptId,
			},
		}
	} else {
		data = Payload{
			// fill struct
			IdempotencyKey:    u.String(),
			SourceID:          paymentToken,
			VerificationToken: verificationToken,
			Card: Card{
				BillingAddress: BillingAddress{
					AddressLine1:                 addressLine1,
					AddressLine2:                 addressLine2,
					Locality:                     city,
					AdministrativeDistrictLevel1: state,
					PostalCode:                   postalCode,
					Country:                      country,
				},
				CardholderName: givenName,
				CustomerID:     customerId,
				ReferenceID:    attemptId,
			},
		}
	}

	payloadBytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest("POST", config.Get("square_domain")+"/cards", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Square-Version", "2023-04-19")
	req.Header.Set("Authorization", "Bearer "+config.Get("square_access_token"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := paymentHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(log.V{"square ioutil.ReadAll: %v": err})
		return "", err
	}

	if resp.StatusCode != 200 {
		var error ErrorModel

		err = json.Unmarshal(b, &error)

		if err != nil {
			log.Error(log.V{"Square Payment error JSON Unmarshall": err})
		}

		log.Info(log.V{"Square Payment parsed": error})

		return "", errors.New(squareErrorDetail(error))
	}

	var card CardModel

	err = json.Unmarshal(b, &card)

	if err != nil {
		log.Error(log.V{"Square Payment JSON Unmarshall": err})
	}

	log.Info(log.V{"Square card created": card.Card.ID})

	return card.Card.ID, err
}

// CreateSubscription creates a subscription for the user. Yearly products use
// the ANNUAL cadence, other recurring products use MONTHLY as before.
func CreateSubscription(locationId string, planId string, customerId string, cardId string) (string, error) {

	type Payload struct {
		IdempotencyKey string `json:"idempotency_key"`
		LocationID     string `json:"location_id"`
		PlanID         string `json:"plan_id"`
		CustomerID     string `json:"customer_id"`
		CardID         string `json:"card_id"`
	}

	// Generate a new Version 4 UUID
	u, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}

	data := Payload{
		IdempotencyKey: u.String(),
		LocationID:     locationId,
		PlanID:         planId,
		CustomerID:     customerId,
		CardID:         cardId,
	}

	payloadBytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest("POST", config.Get("square_domain")+"/subscriptions", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Square-Version", "2023-04-19")
	req.Header.Set("Authorization", "Bearer "+config.Get("square_access_token"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := paymentHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(log.V{"square ioutil.ReadAll: %v": err})
		return "", err
	}

	if resp.StatusCode != 200 {
		var error ErrorModel

		err = json.Unmarshal(b, &error)

		if err != nil {
			log.Error(log.V{"Square Payment error JSON Unmarshall": err})
		}

		log.Info(log.V{"Square Payment parsed": error})

		return "", errors.New(squareErrorDetail(error))
	}

	var subscription SubscriptionModel

	err = json.Unmarshal(b, &subscription)

	if err != nil {
		log.Error(log.V{"Square Payment JSON Unmarshall": err})
	}

	log.Info(log.V{"Square subscription created": subscription.Subscription.ID, "status": subscription.Subscription.Status})

	if subscription.Subscription.Status != "ACTIVE" {
		return "", errors.New("Creating subscription failed,")
	}

	return subscription.Subscription.ID, err
}

func squareErrorDetail(e ErrorModel) string {
	if len(e.Errors) == 0 {
		return "Square request failed"
	}
	return e.Errors[0].Detail
}
