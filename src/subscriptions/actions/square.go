package actions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	s3 "github.com/abishekmuthian/open-payment-host/src/lib/s3"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/abishekmuthian/open-payment-host/src/subscriptions"
	"github.com/google/uuid"
)

// HandleSquareShow shows the web sdk payment page for the Square by responding to the GET request /subscription/square
func HandleSquareShow(w http.ResponseWriter, r *http.Request) error {
	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Get current user
	currentUser := session.CurrentUser(w, r)

	amount := params.GetInt("amount")
	currency := params.Get("currency")
	paymentType := params.Get("type")

	// Render the template
	view := view.NewRenderer(w, r)

	view.AddKey("currentUser", currentUser)

	if paymentType == "onetime" {
		view.AddKey("price", fmt.Sprintf("%d %s/One Time", amount/1000, currency))
	} else if paymentType == "subscription" {
		view.AddKey("price", fmt.Sprintf("%d %s/Monthly", amount/1000, currency))
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

// HandleSquare receives the POST request from the square web sdk at /subscriptions/square
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
	amount := params.GetInt("amount")
	currency := params.Get("currency")
	productId := params.GetInt("productId")

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
	}

	data := Payload{
		IdempotencyKey: u.String(),
		AmountMoney: AmountMoney{
			Amount:   amount,
			Currency: currency,
		},
		SourceID:          paymentToken,
		VerificationToken: verificationToken,
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

	resp, err := http.DefaultClient.Do(req)
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
		var error subscriptions.ErrorModel

		err = json.Unmarshal(b, &error)

		if err != nil {
			log.Error(log.V{"Square Payment error JSON Unmarshall": err})
		}

		log.Info(log.V{"Square Payment parsed": error})

		return server.Redirect(w, r, "/payment/failure?errorDetail="+strings.Replace(error.Errors[0].Detail, ":", "", -1))
	}

	var charge subscriptions.Charge

	err = json.Unmarshal(b, &charge)

	if err != nil {
		log.Error(log.V{"Square Payment JSON Unmarshall": err})
	}

	log.Info(log.V{"Square Payment parsed": charge})

	if charge.Payment.Status == "COMPLETED" {
		log.Info(log.V{"Square Payment Status": "COMPLETED"})

		product, err := products.Find(productId)

		if err == nil {
			if product.S3Bucket != "" && product.S3Key != "" {

				downloadUrl, err := s3.GeneratePresignedUrl(product.S3Bucket, product.S3Key)

				if err == nil {
					return server.RedirectExternal(w, r, downloadUrl)
				}
			}

			return server.Redirect(w, r, "/payment/success")

		}

	} else {
		return server.Redirect(w, r, "/payment/failure?errorDetail="+"Payment failed try again later.")
	}

	return err
}

// HandleCreateSubscription creates a subscription for the customer on POST request to /subscriptions/subscribe
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
	amount := params.GetInt("amount")
	currency := params.Get("currency")
	productId := params.GetInt("productId")
	addressLine1 := params.Get("addressLine1")
	addressLine2 := params.Get("addressLine2")
	givenName := params.Get("givenName")
	email := params.Get("email")
	country := params.Get("country")
	city := params.Get("city")
	state := params.Get("state")
	postalCode := params.Get("postalcode")

	customerId, err := CreateCustomer(paymentToken, verificationToken, amount, currency, productId, addressLine1, addressLine2, givenName, email, country, city, state, postalCode)

	if err != nil {
		return server.Redirect(w, r, "/payment/failure?errorDetail="+strings.Replace(err.Error(), ":", "", -1))
	}

	log.Info(log.V{"Customer ID is: ": customerId})

	cardId, err := CreateCard(paymentToken, verificationToken, amount, currency, productId, addressLine1, addressLine2, givenName, email, country, city, state, postalCode, customerId)

	if err != nil {
		return server.Redirect(w, r, "/payment/failure?errorDetail="+strings.Replace(err.Error(), ":", "", -1))
	}

	log.Info(log.V{"Card ID is: ": cardId})

	product, err := products.Find(productId)

	if err != nil {
		return server.InternalError(err)
	}

	// Get the country from IP
	clientCountry := r.Header.Get("CF-IPCountry")
	log.Info(log.V{"Subscription, Client Country": clientCountry})
	if !config.Production() {
		// There will be no CF request header in the development/test
		clientCountry = config.Get("subscription_client_country")
	}

	catalogId := product.SquareSubscriptionPlanId[clientCountry]

	log.Info(log.V{"Catalog ID is: ": catalogId})

	if catalogId == "" {

		if len(product.SquareSubscriptionPlanId) > 0 {
			// Iterate to find any available catalogId
			for _, catalogId = range product.SquareSubscriptionPlanId {
				log.Info(log.V{"Catalog ID is: ": catalogId})
			}
		}

		if catalogId == "" {
			return server.Redirect(w, r, "/payment/failure?errorDetail=No subscription plan available for your region.")
		}
	}

	subscriptionId, err := CreateSubscription(config.Get("square_location_id"), catalogId, customerId, cardId)

	if err != nil {
		return server.Redirect(w, r, "/payment/failure?errorDetail="+strings.Replace(err.Error(), ":", "", -1))
	} else {
		log.Info(log.V{"Subscription Id is: ": subscriptionId})

		if product.S3Bucket != "" && product.S3Key != "" {

			downloadUrl, err := s3.GeneratePresignedUrl(product.S3Bucket, product.S3Key)

			if err == nil {
				return server.RedirectExternal(w, r, downloadUrl)
			}
		}

		return server.Redirect(w, r, "/payment/success")
	}

	return err
}

// CreateCustomer creates a customer
func CreateCustomer(paymentToken string, verificationToken string, amount int64, currency string, productId int64,
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
		ReferenceID: fmt.Sprintf("Product Id: %d", productId),
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

	resp, err := http.DefaultClient.Do(req)
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
		var error subscriptions.ErrorModel

		err = json.Unmarshal(b, &error)

		if err != nil {
			log.Error(log.V{"Square Payment error JSON Unmarshall": err})
		}

		log.Info(log.V{"Square Payment parsed": error})

		return "", errors.New(error.Errors[0].Detail)
	}

	var customer subscriptions.CustomerModel

	err = json.Unmarshal(b, &customer)

	if err != nil {
		log.Error(log.V{"Square Payment JSON Unmarshall": err})
	}

	log.Info(log.V{"Square Payment parsed": customer})

	return customer.Customer.ID, err
}

// CreateCard creates a card with the customer
func CreateCard(paymentToken string, verificationToken string, amount int64, currency string, productId int64,
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
	u, _ := uuid.NewRandom()
	var data Payload

	if !config.Production() {
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
				ReferenceID:    fmt.Sprintf("Product Id: %d", productId),
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
				ReferenceID:    fmt.Sprintf("Product Id: %d", productId),
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

	resp, err := http.DefaultClient.Do(req)
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
		var error subscriptions.ErrorModel

		err = json.Unmarshal(b, &error)

		if err != nil {
			log.Error(log.V{"Square Payment error JSON Unmarshall": err})
		}

		log.Info(log.V{"Square Payment parsed": error})

		return "", errors.New(error.Errors[0].Detail)
	}

	var card subscriptions.CardModel

	err = json.Unmarshal(b, &card)

	if err != nil {
		log.Error(log.V{"Square Payment JSON Unmarshall": err})
	}

	log.Info(log.V{"Square Payment parsed": card})

	return card.Card.ID, err
}

// CreateSubscription creates a subscription for the user
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

	resp, err := http.DefaultClient.Do(req)
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
		var error subscriptions.ErrorModel

		err = json.Unmarshal(b, &error)

		if err != nil {
			log.Error(log.V{"Square Payment error JSON Unmarshall": err})
		}

		log.Info(log.V{"Square Payment parsed": error})

		return "", errors.New(error.Errors[0].Detail)
	}

	var subscription subscriptions.SubscriptionModel

	err = json.Unmarshal(b, &subscription)

	if err != nil {
		log.Error(log.V{"Square Payment JSON Unmarshall": err})
	}

	log.Info(log.V{"Square Payment parsed": subscription})

	if subscription.Subscription.Status != "ACTIVE" {
		return "", errors.New("Creating subscription failed,")
	}

	return subscription.Subscription.ID, err
}
