package storyactions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/PuerkitoBio/goquery"
	filehelper "github.com/abishekmuthian/open-payment-host/src/lib/model/file"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth/can"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

type Country struct {
	Code string
	Name string
}

// ByName implements sort.Interface for []Country based on the Name field.
type ByName []Country

func (a ByName) Len() int           { return len(a) }
func (a ByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByName) Less(i, j int) bool { return a[i].Name < a[j].Name }

// HandleUpdateShow renders the form to update a story.
func HandleUpdateShow(w http.ResponseWriter, r *http.Request) error {

	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Find the story
	story, err := products.Find(params.GetInt(products.KeyName))
	if err != nil {
		return server.NotFoundError(err)
	}

	// Authorise update story
	currentUser := session.CurrentUser(w, r)
	err = can.Update(story, currentUser)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Render the template
	view := view.NewRenderer(w, r)

	price, err := json.Marshal(story.StripePrice)

	if err == nil && price != nil {
		view.AddKey("price", string(price))
	}

	view.AddKey("story", story)
	view.AddKey("currentUser", currentUser)
	view.AddKey("meta_foot", config.Get("meta_desc"))

	if config.GetBool("stripe") && config.Get("stripe_key") != "" {
		stripePriceJSON, err := json.Marshal(story.StripePrice)

		if err == nil {
			var stripePrices map[string]string

			err := json.Unmarshal([]byte(stripePriceJSON), &stripePrices)
			if err != nil {
				log.Error(log.V{"Error unmarshalling JSON": err})
				return err
			}

			view.AddKey("stripePrices", stripePrices)

		} else {
			view.AddKey("stripePrices", "")
		}

		view.AddKey("stripe", config.GetBool("stripe"))
	}

	if config.GetBool("square") && config.Get("square_access_token") != "" {
		squarePriceJSON, err := json.Marshal(story.SquarePrice)

		if err == nil {
			var squarePrices map[string]map[string]interface{}

			err := json.Unmarshal([]byte(squarePriceJSON), &squarePrices)
			if err != nil {
				log.Error(log.V{"Error unmarshalling JSON:": err})
				return err
			}

			view.AddKey("squarePrices", squarePrices)
		} else {
			view.AddKey("squarePrices", "")
		}

		view.AddKey("square", config.GetBool("square"))
	}

	// To add the scripts for update page
	view.AddKey("loadTrixScript", true)

	if _, err := os.Stat("public" + story.FeaturedImage); errors.Is(err, os.ErrNotExist) {
		// Featured image.jpg does not exist
		log.Error(log.V{"Product Update, Featured image does not exist": err})
	} else {
		// Featured image.jpg exists
		view.AddKey("featuredImagePath", story.FeaturedImage)
	}

	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())

	// Load scripts
	view.AddKey("loadHypermedia", true)
	view.AddKey("loadSweetAlert", true)
	view.AddKey("fieldIndex", 0)

	countryMap := CreateCountryMap()

	// Convert the map to a slice of Country structs
	var countries []Country

	for code, name := range countryMap {
		countries = append(countries, Country{Code: code, Name: name})
	}

	// Sort the slice by country name
	sort.Sort(ByName(countries))

	// Set sorted country-name translation
	view.AddKey("sortedCountries", countries)

	return view.Render()
}

// CreateCountryMap creates a map of countries given the country codes
func CreateCountryMap() map[string]string {
	// Read the HTML
	htmlFile, err := os.ReadFile("src/products/views/countries.html.got")
	if err != nil {
		log.Error(log.V{"Error reading HTML file: %v": err})
	}

	// Parse the HTML with gquery
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(htmlFile))
	if err != nil {
		log.Error(log.V{"Error parsing HTML: %v": err})
	}

	// Create a map to store country codes and names
	countryMap := make(map[string]string)

	// Traverse the options elements to extract country codes and names
	doc.Find("option").Each(func(i int, s *goquery.Selection) {
		code, _ := s.Attr("value")

		name := s.Text()
		countryMap[code] = name
	})

	return countryMap
}

// HandleUpdate handles the POST of the form to update a story
func HandleUpdate(w http.ResponseWriter, r *http.Request) error {

	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	name := params.Get("name")
	id := params.GetInt("id")

	// Find the story
	story, err := products.Find(params.GetInt(products.KeyName))
	if err != nil {
		return server.NotFoundError(err)
	}

	// Check the authenticity token
	err = session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Authorise update story
	currentUser := session.CurrentUser(w, r)
	err = can.Update(story, currentUser)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

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

			outFile, err := os.Create("public/assets/images/products/" + fmt.Sprintf("%d-%s-%s", id, filehelper.SanitizeName(name), "featured_image") + fileExtension)
			if err != nil {
				log.Error(log.V{"msg": "Image creation, Creating empty file", "error": err})
			} else {
				storyParams["featured_image"] = "/assets/images/products/" + fmt.Sprintf("%d-%s-%s", id, filehelper.SanitizeName(name), "featured_image") + fileExtension
			}
			defer outFile.Close()

			outFile.Write(fileData)
		} else {
			return server.InternalError(errors.New("Improper image format only png or jpg image format is allowed."))
		}

	}

	// Store stripe price
	if config.GetBool("stripe") && config.Get("stripe_key") != "" {
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

		if err == nil && !reflect.DeepEqual(story.SquarePrice, squarePrice) {
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
						}

					}
				}
			}

		}
	}

	err = story.Update(storyParams)
	if err != nil {
		return server.InternalError(err)
	}

	//Update featured image for other than default posts
	// FIXME : Add error handling
	/* 	if id > 5 && (file.SanitizeName(name) != file.SanitizeName(story.Name)) {
		texttoimage.TextToImage(name, id)
	} */

	// Redirect to story
	return server.Redirect(w, r, story.ShowURL())
}
