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
	"sort"
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

	if config.GetBool("square") && config.Get("square_access_token") != "" && config.Get("square_app_id") != "" {
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

	if config.GetBool("paypal") && config.Get("paypal_client_id") != "" && config.Get("paypal_client_secret") != "" {
		paypalPriceJSON, err := json.Marshal(story.PaypalPrice)

		if err == nil {
			var paypalPrices map[string]map[string]interface{}

			err := json.Unmarshal([]byte(paypalPriceJSON), &paypalPrices)
			if err != nil {
				log.Error(log.V{"Error unmarshalling JSON:": err})
				return err
			}

			view.AddKey("paypalPrices", paypalPrices)
		} else {
			view.AddKey("paypalPrices", "")
		}
		view.AddKey("paypal", config.GetBool("paypal"))
	}
	if config.GetBool("razorpay") && config.Get("razorpay_key_id") != "" && config.Get("razorpay_key_secret") != "" {
		razorpayPriceJSON, err := json.Marshal(story.RazorpayPrice)

		if err == nil {
			var razorpayPrices map[string]map[string]interface{}

			err := json.Unmarshal([]byte(razorpayPriceJSON), &razorpayPrices)
			if err != nil {
				log.Error(log.V{"Error unmarshalling JSON:": err})
				return err
			}

			view.AddKey("razorpayPrices", razorpayPrices)
		} else {
			view.AddKey("razorpayPrices", "")
		}
		view.AddKey("razorpay", config.GetBool("razorpay"))
	}
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

	// To add the scripts for update page
	view.AddKey("loadTrixScript", true)
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
	if listID, ok := storyParams["listmonk_list_id"]; ok && listID == "" {
		storyParams["listmonk_list_id"] = "0"
	}

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

	// Parse the per-country gateway prices, skipping invalid rows
	prices := buildPriceMaps(params.Values, params.Get("schedule"))

	// Store stripe price
	if config.GetBool("stripe") && config.Get("stripe_key") != "" {
		jsonResult, err := json.Marshal(prices.Stripe)
		if err != nil {
			log.Error(log.V{"Error marshalling JSON": err})
			return err
		}

		storyParams["stripe_price"] = string(jsonResult)
		story.Update(storyParams)

	}

	if config.GetBool("square") && config.Get("square_access_token") != "" && config.Get("square_app_id") != "" {
		jsonResult, err := json.Marshal(prices.Square)
		if err != nil {
			log.Error(log.V{"Error marshalling JSON": err})
			return err
		}

		storyParams["square_price"] = string(jsonResult)
		story.Update(storyParams)

		// Recreate the Square plans only when prices or the schedule changed
		if !reflect.DeepEqual(story.SquarePrice, prices.Square) || story.Schedule != storyParams["schedule"] {
			catalogMap := createSquarePlans(story.ID, prices.Square, storyParams["schedule"])
			if len(catalogMap) != 0 {
				catalogMapJson, err := json.Marshal(catalogMap)
				if err == nil {
					storyParams["square_subscription_plan_Id"] = string(catalogMapJson)
				}
			}
		}
	}

	if config.GetBool("paypal") && config.Get("paypal_client_id") != "" && config.Get("paypal_client_secret") != "" {
		jsonResult, err := json.Marshal(prices.Paypal)
		if err != nil {
			log.Error(log.V{"Error marshalling JSON": err})
			return err
		}

		storyParams["paypal_price"] = string(jsonResult)
		story.Update(storyParams)
	}

	if config.GetBool("razorpay") && config.Get("razorpay_key_id") != "" && config.Get("razorpay_key_secret") != "" {
		jsonResult, err := json.Marshal(prices.Razorpay)
		if err != nil {
			log.Error(log.V{"Error marshalling JSON": err})
			return err
		}

		storyParams["razorpay_price"] = string(jsonResult)
		story.Update(storyParams)
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
