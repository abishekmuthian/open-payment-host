package actions

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
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
)

// HandleBillingShow shows the billing details page for GET request /subscriptions/billing
func HandleBillingShow(w http.ResponseWriter, r *http.Request) error {

	// Check if required tokens are present
	if config.Get("square_access_token") == "" || config.Get("square_app_id") == "" || config.Get("square_location_id") == "" {
		return server.InternalError(errors.New("Please set the Square token, key, and ids in the config."))
	}

	// Fetch the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Get current user
	currentUser := session.CurrentUser(w, r)

	amount := params.Get("amount")
	currency := params.Get("currency")
	paymentType := params.Get("type")
	productId := params.Get("productId")

	// Render the template
	view := view.NewRenderer(w, r)

	view.AddKey("currentUser", currentUser)

	if paymentType == "onetime" {
		view.AddKey("price", amount+" "+currency)
	} else if paymentType == "subscription" {
		view.AddKey("price", amount+" "+currency+"/"+"monthly")
	}

	view.AddKey("amount", amount)
	view.AddKey("currency", currency)
	view.AddKey("type", paymentType)
	view.AddKey("productId", productId)

	// Set Cloudflare turnstile site key
	view.AddKey("turnstile_site_key", config.Get("turnstile_site_key"))
	view.AddKey("billing", true)
	view.AddKey("error", params.Get("error"))

	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())

	return view.Render()
}

// HandleBilling processes the billing details and completes the payment transaction
func HandleBilling(w http.ResponseWriter, r *http.Request) error {

	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Get the size of elements in params for security
	paramMap := params.Map()
	for _, element := range paramMap {
		if len(element) > 1000 {
			return server.Redirect(w, r, "/payment/failure?errorDetail=Check your billing details.")
		}
	}

	log.Info(log.V{"params from billing": params})

	name := params.Get("name")
	email := params.Get("email")
	country := params.Get("country")
	addressLine1 := params.Get("addressline1")
	addressLine2 := params.Get("addressline2")
	locality := params.Get("locality")
	state := params.Get("state")
	postalcode := params.Get("postalcode")
	amount := params.Get("amount")
	currency := params.Get("currency")
	paymentType := params.Get("type")
	productId := params.Get("productId")

	var intent string

	if paymentType == "onetime" {
		intent = "CHARGE"
	} else if paymentType == "subscription" {
		intent = "STORE"
	}

	// Using turnstile to verify users
	if len(params.Get("cf-turnstile-response")) > 0 {
		if string(params.Get("cf-turnstile-response")) != "" {

			type turnstileResponse struct {
				Success      bool     `json:"success"`
				Challenge_ts string   `json:"challenge_ts"`
				Hostname     string   `json:"hostname"`
				ErrorCodes   []string `json:"error-codes"`
				Action       string   `json:"login"`
				Cdata        string   `json:"cdata"`
			}

			var remoteIP string
			var siteVerify turnstileResponse

			if config.Production() {
				// Get the IP from Cloudflare
				remoteIP = r.Header.Get("CF-Connecting-IP")

			} else {
				// Extract the IP from the address
				remoteIP = r.RemoteAddr
				forward := r.Header.Get("X-Forwarded-For")
				if len(forward) > 0 {
					remoteIP = forward
				}
			}

			postBody := url.Values{}
			postBody.Set("secret", config.Get("turnstile_secret_key"))
			postBody.Set("response", string(params.Get("cf-turnstile-response")))
			postBody.Set("remoteip", remoteIP)

			resp, err := http.Post("https://challenges.cloudflare.com/turnstile/v0/siteverify", "application/x-www-form-urlencoded", strings.NewReader(postBody.Encode()))
			if err != nil {
				log.Info(log.V{"Upload, An error occurred while sending the request to the siteverify": err})
				return server.InternalError(err)
			}
			defer resp.Body.Close()

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Error(log.V{"Upload, An error occurred while reading the response from the siteverify": err})
				return server.InternalError(err)
			}

			json.Unmarshal(body, &siteVerify)

			if !siteVerify.Success {
				// Security challenge failed
				log.Error(log.V{"Upload, Security challenge failed": siteVerify.ErrorCodes[0]})
				return server.Redirect(w, r, "/subscriptions/billing?error=security_challenge_failed_login"+fmt.Sprintf("&amount=%s&currency=%s&type=%s&productId=%s", amount, currency, paymentType, productId))
			}
		} else {
			log.Error(log.V{"Upload, Security challenge unable to process": "response not received from user"})
			return server.Redirect(w, r, "/subscriptions/billing?error=security_challenge_not_completed_login"+fmt.Sprintf("&amount=%s&currency=%s&type=%s&productId=%s", amount, currency, paymentType, productId))
		}
	} else {
		// Security challenge not completed
		return server.Redirect(w, r, "/subscriptions/billing?error=security_challenge_not_completed_login"+fmt.Sprintf("&amount=%s&currency=%s&type=%s&productId=%s", amount, currency, paymentType, productId))
	}

	return server.Redirect(w, r, fmt.Sprintf("/subscriptions/square?amount=%s&currency=%s&type=%s&addressLine1=%s&addressLine2=%s&givenName=%s&email=%s&country=%s&city=%s&state=%s&postalcode=%s&intent=%s&productId=%s", amount, currency, paymentType, addressLine1, addressLine2, name, email, country, locality, state, postalcode, intent, productId))
}
