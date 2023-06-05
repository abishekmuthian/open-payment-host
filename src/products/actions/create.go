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
	// To add the scripts for add product page
	view.AddKey("loadTrixScript", true)
	view.AddKey("currentUser", currentUser)
	view.AddKey("meta_foot", config.Get("meta_desc"))
	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())

	if config.Get("square_access_token") != "" {
		view.AddKey("square", config.GetBool("square"))
	} else {
		view.AddKey("square", false)
	}
	view.AddKey("stripe", config.GetBool("stripe"))
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
			return server.InternalError(errors.New("Improper image format only png or jpg image format is allowed."))
		}

	}

	// Create subscription plan for Square
	if config.GetBool("square") && storyParams["square_price"] != "" {

		var squarePrice map[string]map[string]interface{}

		err = json.Unmarshal([]byte(storyParams["square_price"]), &squarePrice)

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
