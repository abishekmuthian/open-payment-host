package subscriptions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/abishekmuthian/open-payment-host/src/lib/listmonk"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
)

func addSubscriberToListmonk(listID int64, email string, name string) {
	baseURL := config.Get("listmonk_URL")
	apiToken := config.Get("listmonk_API_token")
	preconfirmSubscriptions := listmonkPreconfirmSubscriptions()
	if listID <= 0 || baseURL == "" || apiToken == "" || email == "" {
		return
	}

	go func() {
		upsertSubscriberToListmonk(baseURL, apiToken, listID, email, name, preconfirmSubscriptions)
	}()
}

func addSquareSubscriberToListmonk(listID int64, email string, customerID string) {
	baseURL := config.Get("listmonk_URL")
	apiToken := config.Get("listmonk_API_token")
	preconfirmSubscriptions := listmonkPreconfirmSubscriptions()
	if listID <= 0 || baseURL == "" || apiToken == "" {
		return
	}

	if email != "" {
		addSubscriberToListmonk(listID, email, "")
		return
	}
	if customerID == "" {
		return
	}

	go func() {
		customer, err := fetchSquareCustomer(customerID)
		if err != nil {
			log.Error(log.V{"Square, Error retrieving customer for Listmonk": err, "customer_id": customerID})
			return
		}
		if customer.Customer.EmailAddress == "" {
			log.Error(log.V{"Square, Customer email is empty for Listmonk": customerID})
			return
		}
		upsertSubscriberToListmonk(baseURL, apiToken, listID, customer.Customer.EmailAddress, customer.Customer.GivenName, preconfirmSubscriptions)
	}()
}

func upsertSubscriberToListmonk(baseURL string, apiToken string, listID int64, email string, name string, preconfirmSubscriptions bool) {
	_, err := listmonk.UpsertSubscriber(baseURL, apiToken, email, name, int(listID), preconfirmSubscriptions)
	if err != nil {
		log.Error(log.V{"Listmonk, Error adding subscriber to list": err, "list_id": listID, "email": email})
		return
	}
	log.Info(log.V{"msg": "Listmonk, Subscriber added to list", "list_id": listID, "email": email})
}

func listmonkPreconfirmSubscriptions() bool {
	value := strings.ToLower(strings.TrimSpace(config.Get("listmonk_preconfirm_subscriptions")))
	if value == "" {
		return true
	}
	return value == "true" || value == "yes" || value == "1" || value == "on"
}

func fetchSquareCustomer(customerID string) (*CustomerModel, error) {
	req, err := http.NewRequest(http.MethodGet, config.Get("square_domain")+"/customers/"+url.PathEscape(customerID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Square-Version", "2023-05-17")
	req.Header.Set("Authorization", "Bearer "+config.Get("square_access_token"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Square Customers API returned %s", resp.Status)
	}

	var customer CustomerModel
	if err := json.NewDecoder(resp.Body).Decode(&customer); err != nil {
		return nil, err
	}
	return &customer, nil
}
