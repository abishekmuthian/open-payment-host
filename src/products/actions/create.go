package storyactions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/subscriptions"
	"github.com/google/uuid"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth/can"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/products"

	filehelper "github.com/abishekmuthian/open-payment-host/src/lib/model/file"
)

// HandleCreateShow serves the create form via GET for products.
func HandleCreateShow(w http.ResponseWriter, r *http.Request) error {

	story := products.New()

	// Authorise
	currentUser := session.CurrentUser(w, r)
	err := can.Create(story, currentUser)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Render the template
	view := view.NewRenderer(w, r)
	view.AddKey("story", story)

	view.AddKey("currentUser", currentUser)
	view.AddKey("meta_foot", config.Get("meta_desc"))
	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())

	if config.GetBool("stripe") && config.Get("stripe_key") != "" {
		view.AddKey("stripe", config.GetBool("stripe"))
	}

	if config.GetBool("square") && config.Get("square_access_token") != "" && config.Get("square_app_id") != "" {
		view.AddKey("square", config.GetBool("square"))
	}

	if config.GetBool("paypal") && config.Get("paypal_client_id") != "" && config.Get("paypal_client_secret") != "" {
		view.AddKey("paypal", config.GetBool("paypal"))
	}

	if config.GetBool("razorpay") && config.Get("razorpay_key_id") != "" && config.Get("razorpay_key_secret") != "" {
		view.AddKey("razorpay", config.GetBool("razorpay"))
	}

	// To add the scripts for add product page
	view.AddKey("loadTrixScript", true)
	view.AddKey("loadHypermedia", true)
	view.AddKey("loadSweetAlert", true)

	return view.Render()
}

// HandleCreate handles the POST of the create form for products
func HandleCreate(w http.ResponseWriter, r *http.Request) error {

	story := products.New()

	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Get user details
	currentUser := session.CurrentUser(w, r)

	// Authorise
	err = can.Create(story, currentUser)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	name := params.Get("name")

	// Check if product has more than 2 hashtags
	if CountHashTag(name) > 2 {
		return server.NotAuthorizedError(nil, "Hashtag too many or format error", "Your product has too many hashtags, title should be your product in 50 characters followed by 2 hashtags. You can click back safely to edit what you had typed.")
	}

	// Clean params according to role
	accepted := products.AllowedParams()
	if currentUser.Admin() {
		accepted = products.AllowedParamsAdmin()
	}
	storyParams := story.ValidateParams(params.Map(), accepted)

	// Set a few params to known good values
	storyParams["points"] = "1"
	storyParams["user_id"] = fmt.Sprintf("%d", currentUser.ID)
	storyParams["user_name"] = currentUser.Name

	ID, err := story.Create(storyParams)
	if err != nil {
		return err // Create returns a router.Error
	}

	// Log creation
	log.Info(log.V{"msg": "Created story", "story_id": ID})

	// Redirect to the new story
	story, err = products.Find(ID)
	if err != nil {
		return server.InternalError(err)
	}

	// We need to add a vote to the story here too by adding a join to the new id
	// No votes in Open Payment Host yet, POWER in query needs fixing
	/* 	err = recordStoryVote(story, currentUser, ip, +1)
	   	if err != nil {
	   		return err
	   	} */

	// Re-rank products
	// No ranking in Open Payment Host yet, POWER in query needs fixing
	/* 	err = updateproductsRank()
	   	if err != nil {
	   		return err
	   	} */

	//Create featured image for other than default posts
	// Call this when featured image is not set
	// FIXME : Add error handling
	/* 	if ID > 5 {
		texttoimage.TextToImage(name, ID)
	} */

	// Featured Image
	for _, fh := range params.Files {

		fileType := fh[0].Header.Get("Content-Type")
		fileSize := fh[0].Size

		log.Info(log.V{"Product Submission": "File Upload", "fileType": fileType})
		log.Info(log.V{"Product Submission": "File Upload", "fileSize (kB)": fileSize / 1000})

		if fileType == "image/png" || fileType == "image/jpeg" {
			file, err := fh[0].Open()
			defer file.Close()

			if err != nil {
				log.Error(log.V{"Create Product, Error storing featured image": err})
			}

			fileData, err := ioutil.ReadAll(file)
			if err != nil {
				log.Error(log.V{"Create Product, Error storing featured image": err})
			}

			var fileExtension string

			if fileType == "image/png" {
				fileExtension = ".png"
			}

			if fileType == "image/jpeg" {
				fileExtension = ".jpg"
			}

			outFile, err := os.Create("public/assets/images/products/" + fmt.Sprintf("%d-%s-%s", ID, filehelper.SanitizeName(name), "featured_image") + fileExtension)
			if err != nil {
				log.Error(log.V{"msg": "Image creation, Creating empty file", "error": err})
			} else {
				storyParams["featured_image"] = "/assets/images/products/" + fmt.Sprintf("%d-%s-%s", ID, filehelper.SanitizeName(name), "featured_image") + fileExtension
				err = story.Update(storyParams)
				if err != nil {
					return server.InternalError(err)
				}
			}
			defer outFile.Close()

			outFile.Write(fileData)

		} else {
			// TODO wrong image format inform user
			return server.InternalError(errors.New("improper image format only png or jpg image format is allowed"))
		}

	}
	// Store stripe price
	if config.GetBool("stripe") && config.Get("stripe_key") != "" && config.Get("stripe_secret") != "" {
		result := make(map[string]string)

		countryRegex := regexp.MustCompile(`^stripe_country_(\d+)$`)

		// Iterate over all query parameters
		r.ParseForm()
		for key, value := range params.Values {
			if len(value) > 0 {
				switch {
				case countryRegex.MatchString(key):
					index := countryRegex.FindStringSubmatch(key)[1]
					planIDKey := fmt.Sprintf("stripe_plan_id_%s", index)
					if planID, exists := r.Form[planIDKey]; exists && len(planID) > 0 {
						result[value[0]] = planID[0]
					}
				}
			}
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			log.Error(log.V{"Error marshalling JSON": err})
			return err
		}

		storyParams["stripe_price"] = string(jsonResult)
		story.Update(storyParams)
	}

	// Store razorpay price
	if config.GetBool("razorpay") && config.Get("razorpay_key_id") != "" && config.Get("razorpay_key_secret") != "" {
		result := make(map[string]map[string]interface{})

		countryRegex := regexp.MustCompile(`^razorpay_country_(\d+)$`)

		// Iterate over all query parameters
		r.ParseForm()
		for key, value := range params.Values {
			if len(value) > 0 {
				switch {
				case countryRegex.MatchString(key):
					index := countryRegex.FindStringSubmatch(key)[1]
					// Initialize a new map for the amount and currency

					amountCurrencyMap := make(map[string]interface{})

					amountKey := fmt.Sprintf("razorpay_amount_%s", index)
					if amountStr, exists := r.Form[amountKey]; exists && len(amountStr) > 0 {
						var amount float64
						if amount, err = strconv.ParseFloat(amountStr[0], 64); err == nil {
							amountCurrencyMap["amount"] = amount
						} else {
							// Handle the error, e.g., log it or return an HTTP error
							log.Error(log.V{"Failed to parse amount": err})
						}
					}

					currencyKey := fmt.Sprintf("razorpay_currency_%s", index)
					if currency, exists := r.Form[currencyKey]; exists && len(currency) > 0 {
						amountCurrencyMap["currency"] = currency[0]
					}

					planIDKey := fmt.Sprintf("razorpay_plan_id_%s", index)
					if planID, exists := r.Form[planIDKey]; exists && len(planID) > 0 {
						amountCurrencyMap["plan_id"] = planID[0]
					}

					result[value[0]] = amountCurrencyMap

				}
			}
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			log.Error(log.V{"Error marshalling JSON": err})
			return err
		}

		storyParams["razorpay_price"] = string(jsonResult)
		story.Update(storyParams)
	}

	// Store paypal price
	if config.GetBool("paypal") && config.Get("paypal_client_id") != "" && config.Get("paypal_client_secret") != "" {
		result := make(map[string]map[string]interface{})

		countryRegex := regexp.MustCompile(`^paypal_country_(\d+)$`)

		// Iterate over all query parameters
		r.ParseForm()
		for key, value := range params.Values {
			if len(value) > 0 {
				switch {
				case countryRegex.MatchString(key):
					index := countryRegex.FindStringSubmatch(key)[1]
					// Initialize a new map for the amount and currency

					amountCurrencyMap := make(map[string]interface{})

					amountKey := fmt.Sprintf("paypal_amount_%s", index)
					if amountStr, exists := r.Form[amountKey]; exists && len(amountStr) > 0 {
						var amount float64
						if amount, err = strconv.ParseFloat(amountStr[0], 64); err == nil {
							amountCurrencyMap["amount"] = amount
						} else {
							// Handle the error, e.g., log it or return an HTTP error
							log.Error(log.V{"Failed to parse amount": err})
						}
					}

					taxKey := fmt.Sprintf("paypal_tax_%s", index)
					if taxStr, exists := r.Form[taxKey]; exists && len(taxStr) > 0 {
						var tax float64
						if tax, err = strconv.ParseFloat(taxStr[0], 64); err == nil {
							amountCurrencyMap["tax"] = tax
						} else {
							// Handle the error, e.g., log it or return an HTTP error
							log.Error(log.V{"Failed to parse tax": err})
						}
					}

					currencyKey := fmt.Sprintf("paypal_currency_%s", index)
					if currency, exists := r.Form[currencyKey]; exists && len(currency) > 0 {
						amountCurrencyMap["currency"] = currency[0]
					}

					planIDKey := fmt.Sprintf("paypal_plan_id_%s", index)
					if planID, exists := r.Form[planIDKey]; exists && len(planID) > 0 {
						amountCurrencyMap["plan_id"] = planID[0]
					}

					result[value[0]] = amountCurrencyMap

				}
			}
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			log.Error(log.V{"Error marshalling JSON": err})
			return err
		}

		storyParams["paypal_price"] = string(jsonResult)
		story.Update(storyParams)
	}

	// Create subscription plan for Square
	if config.GetBool("square") && config.Get("square_access_token") != "" && config.Get("square_app_id") != "" {

		result := make(map[string]map[string]interface{})

		countryRegex := regexp.MustCompile(`^square_country_(\d+)$`)

		// Iterate over all query parameters
		r.ParseForm()
		for key, value := range params.Values {
			if len(value) > 0 {
				switch {
				case countryRegex.MatchString(key):
					index := countryRegex.FindStringSubmatch(key)[1]
					// Initialize a new map for the amount and currency

					amountCurrencyMap := make(map[string]interface{})

					amountKey := fmt.Sprintf("square_amount_%s", index)
					if amountStr, exists := r.Form[amountKey]; exists && len(amountStr) > 0 {
						var amount float64
						if amount, err = strconv.ParseFloat(amountStr[0], 64); err == nil {
							amountCurrencyMap["amount"] = amount
						} else {
							// Handle the error, e.g., log it or return an HTTP error
							log.Error(log.V{"Failed to parse amount": err})
						}
					}

					currencyKey := fmt.Sprintf("square_currency_%s", index)
					if currency, exists := r.Form[currencyKey]; exists && len(currency) > 0 {
						amountCurrencyMap["currency"] = currency[0]
					}

					result[value[0]] = amountCurrencyMap

				}
			}
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			log.Error(log.V{"Error marshalling JSON": err})
			return err
		}

		storyParams["square_price"] = string(jsonResult)
		story.Update(storyParams)

		var squarePrice map[string]map[string]interface{}

		err = json.Unmarshal([]byte(storyParams["square_price"]), &squarePrice)

		// Creating subscription plan for Square
		if err == nil {
			if len(squarePrice) != 0 {
				for clientCountry, data := range squarePrice {
					amount := data["amount"]
					currency := data["currency"]
					catalogId, error := CreateSubscriptionPlan(story.ID, int64(amount.(float64)), currency.(string))

					if err != nil {
						log.Error(log.V{"Error creating subscription plan ": error})
						continue
					}
					log.Info(log.V{"CountryCode is ": clientCountry, "Catalog ID is ": catalogId})

					if catalogId != "" && clientCountry != "" {
						catalogMap := make(map[string]string)

						catalogMap[clientCountry] = catalogId

						catalogMapJson, err := json.Marshal(catalogMap)

						if err == nil {
							storyParams["square_subscription_plan_Id"] = string(catalogMapJson)

							// Update the db with catalog id
							err = story.Update(storyParams)
							if err != nil {
								return server.InternalError(err)
							}
						}

					}
				}
			}

		}
	}

	return server.Redirect(w, r, story.IndexURL())
}

// RemoveHashTag removes hashtag from the string and returns the string
func RemoveHashTag(name string) string {
	var myregex = `#\w*`
	var re = regexp.MustCompile(myregex)

	var fulltext = re.ReplaceAllString(name, "")
	return fulltext
}

// CountHashTag returns the number of hashtag in the string
func CountHashTag(name string) int {
	var myregex = `#\w*`
	var re = regexp.MustCompile(myregex)

	var fulltext = re.FindAllString(name, -1)
	return len(fulltext)
}

// CreateSubscriptionPlan creates a subscription plan for square
func CreateSubscriptionPlan(productId int64, amount int64, currency string) (string, error) {

	type RecurringPriceMoney struct {
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	}
	type Phases struct {
		Cadence             string              `json:"cadence"`
		RecurringPriceMoney RecurringPriceMoney `json:"recurring_price_money"`
	}
	type SubscriptionPlanData struct {
		Name   string   `json:"name"`
		Phases []Phases `json:"phases"`
	}
	type Object struct {
		ID                   string               `json:"id"`
		Type                 string               `json:"type"`
		SubscriptionPlanData SubscriptionPlanData `json:"subscription_plan_data"`
	}

	type Payload struct {
		IdempotencyKey string `json:"idempotency_key"`
		Object         Object `json:"object"`
	}

	// Generate a new Version 4 UUID
	u, _ := uuid.NewRandom()

	product, err := products.Find(productId)

	if err != nil {
		// Handle error
	}

	data := Payload{
		IdempotencyKey: u.String(),
		Object: Object{
			ID:   fmt.Sprintf("#product%d", productId),
			Type: "SUBSCRIPTION_PLAN",
			SubscriptionPlanData: SubscriptionPlanData{
				Name: fmt.Sprintf("Subscription for %s", product.Name),
				Phases: []Phases{
					Phases{
						Cadence: "MONTHLY",
						RecurringPriceMoney: RecurringPriceMoney{
							Amount:   amount,
							Currency: currency,
						},
					},
				},
			},
		},
	}
	payloadBytes, err := json.Marshal(data)
	if err != nil {
		// handle err
	}
	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest("POST", config.Get("square_domain")+"/catalog/object", body)
	if err != nil {
		// handle err
	}
	req.Header.Set("Square-Version", "2023-04-19")
	req.Header.Set("Authorization", "Bearer "+config.Get("square_access_token"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// handle err
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

	var catalog subscriptions.CatalogModel

	err = json.Unmarshal(b, &catalog)

	if err != nil {
		log.Error(log.V{"Square Payment JSON Unmarshall": err})
	}

	log.Info(log.V{"Square Payment parsed": catalog})

	return catalog.CatalogObject.ID, err
}
