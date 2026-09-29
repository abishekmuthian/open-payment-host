package storyactions

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/razorpay/razorpay-go"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/status"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/abishekmuthian/open-payment-host/src/subscriptions"

	"github.com/kennygrant/sanitize"

	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/price"
)

// HandleShow displays a single story.
func HandleShow(w http.ResponseWriter, r *http.Request) error {

	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	redirectUri := params.Get("redirect_uri")
	customId := params.Get("custom_id")

	// Find the story
	story, err := products.Find(params.GetInt(products.KeyName))
	if err != nil {
		return server.NotFoundError(err)
	}

	// Get current user
	currentUser := session.CurrentUser(w, r)

	// Authorise access - for now all products are visible, later might control on draft/published
	if story.Status == status.Suspended && !currentUser.Admin() { // status.None previously for not using this feature
		return server.NotFoundError(nil, "product not found", "This product might have been removed for policy violations or by the user.")
	}
	if story.Status == status.Draft && !currentUser.Admin() { // status.None previously for not using this feature
		return server.NotFoundError(nil, "product not found", "This product might be under moderation, please check back later.")
	}

	/*else{ //There could be use for this in future
		err = can.Show(story, currentUser)
		if err != nil {
			return server.NotAuthorizedError(err)
		}
	}*/

	// Find the comments for this story, excluding those under 0
	/* 	q := comments.Where("story_id=?", story.ID).Where("points > 0").Order(comments.Order)
	   	comments, err := comments.FindAll(q)
	   	if err != nil {
	   		return server.InternalError(err)
	   	} */

	meta := truncateString(sanitize.HTML(story.Summary), 150)
	if meta == "" {
		meta = config.Get("meta_desc")
	}

	metaTitle := strings.TrimSpace(RemoveHashTag(story.Name))

	if metaTitle == "" {
		metaTitle = config.Get("meta_title")
	}

	metaImage := config.Get("root_url") + "/assets/images/products/" + story.FileName() + ".png"

	// Render the template
	view := view.NewRenderer(w, r)
	view.CacheKey(story.CacheKey())
	view.AddKey("meta_image", metaImage)
	view.AddKey("story", story)
	view.AddKey("meta_published_time", story.CreatedAt.Format("2006-01-02T15:04:05-0700"))
	view.AddKey("meta_modified_time", story.UpdatedAt.Format("2006-01-02T15:04:05-0700"))
	view.AddKey("meta_title", metaTitle)
	view.AddKey("meta_desc", meta)
	view.AddKey("meta_foot", config.Get("meta_desc"))
	view.AddKey("meta_keywords", fmt.Sprintf("%s%s", MetaHashTag(story.GetHashTag()), config.Get("meta_keywords")))
	// view.AddKey("comments", comments)
	view.AddKey("currentUser", currentUser)

	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())

	// Set subscribe button if price is set for Payment Gateways
	if len(story.SquarePrice) != 0 || len(story.StripePrice) != 0 || len(story.PaypalPrice) != 0 || len(story.RazorpayPrice) != 0 {

		// Get the country from IP
		clientCountry := r.Header.Get("CF-IPCountry")
		if !config.Production() {
			// There will be no CF request header in the development/test
			clientCountry = config.Get("subscription_client_country")
		}

		log.Info(log.V{"Subscription, Client Country": clientCountry})

		// Find which gateway has a price for the clientCountry, falling back to DF
		pg, priceCountry := selectGateway(story, clientCountry)
		if pg == "" {
			// No payment gateway configured for this country
			log.Error(log.V{"Show, No payment gateway configured for country": clientCountry})
			view.Template("products/views/show.html.got")
			return view.Render()
		}
		log.Info(log.V{"msg": "Show, Using price", "gateway": pg, "country": priceCountry})
		clientCountry = priceCountry

		scheduleLabel := "Monthly"
		if story.Schedule == "yearly" {
			scheduleLabel = "Year"
		}

		switch pg {
		case "stripe":
			// Code for Stripe
			priceId := story.StripePrice[clientCountry]

			if priceId == "" {
				return errors.New("Invalid price details for client country: " + clientCountry)
			}

			log.Info(log.V{"Price ID: ": priceId})

			priceClient := price.Client{B: stripe.GetBackend(stripe.APIBackend), Key: config.Get("stripe_secret")}
			p, err := priceClient.Get(priceId, nil)

			if err == nil {

				log.Info(log.V{"Currency:": p.Currency})

				view.AddKey("priceId", priceId)
				display, err := subscriptions.FormatConfiguredPrice(p.UnitAmount, string(p.Currency), true)
				if err != nil {
					return err
				}

				if p.Type == "recurring" && p.Recurring != nil {
					view.AddKey("price", display+"/"+string(p.Recurring.Interval))
				} else if p.Type == "one_time" {
					view.AddKey("price", display+"/"+"One Time")
				}
			}
			view.AddKey("stripe", config.GetBool("stripe"))
		case "square":
			// Code for Square, amounts are stored in minor units
			amount := story.SquarePrice[clientCountry]["amount"]
			currency := story.SquarePrice[clientCountry]["currency"]
			currencyCode, _ := currency.(string)

			if amount == nil || currencyCode == "" {
				return errors.New("Invalid price details for client country: " + clientCountry)
			}
			display, err := subscriptions.FormatConfiguredPrice(amount, currencyCode, true)
			if err != nil {
				return err
			}
			if story.Schedule == "onetime" {
				view.AddKey("price", display+"/"+"One Time")
				view.AddKey("type", "onetime")
			} else if story.Schedule == "monthly" || story.Schedule == "yearly" {
				view.AddKey("price", display+"/"+scheduleLabel)
				view.AddKey("type", "subscription")
			}

			view.AddKey("amount", amount)
			view.AddKey("currency", currency)
			view.AddKey("square", config.GetBool("square"))
		case "paypal":
			// Code for PayPal, amounts are stored in major units
			paypalPrice := story.PaypalPrice[clientCountry]
			amount := paypalPrice["amount"]
			currency := paypalPrice["currency"]
			currencyCode, _ := currency.(string)
			hasAmount := amount != nil && currencyCode != ""

			// One-time payments need an amount; subscriptions need at least a plan
			if !hasAmount && (story.Schedule == "onetime" || paypalPrice["plan_id"] == nil) {
				return errors.New("Invalid price details for client country: " + clientCountry)
			}
			display := ""
			if hasAmount {
				var err error
				display, err = subscriptions.FormatConfiguredPrice(amount, currencyCode, false)
				if err != nil {
					return err
				}
			}

			if story.Schedule == "onetime" {
				view.AddKey("price", display+"/"+"One Time")
				view.AddKey("type", "onetime")
				view.AddKey("paypal_payment_link", paymentEntryURL("paypal", story.ID, customId, redirectUri))
			} else if story.Schedule == "monthly" || story.Schedule == "yearly" {
				if display != "" {
					view.AddKey("price", display+"/"+scheduleLabel)
				} else {
					view.AddKey("price", scheduleLabel)
				}
				view.AddKey("type", "subscription")
				view.AddKey("paypal_payment_link", paymentEntryURL("paypal", story.ID, customId, redirectUri))
			}

			view.AddKey("amount", amount)
			view.AddKey("currency", currency)
			view.AddKey("paypal", config.GetBool("paypal"))
		case "razorpay":
			// Code for Razorpay, one-time amounts are stored in major units
			razorpayPrice := story.RazorpayPrice[clientCountry]
			amount := razorpayPrice["amount"]
			currency := razorpayPrice["currency"]
			currencyCode, _ := currency.(string)
			planId, _ := razorpayPrice["plan_id"].(string)

			if story.Schedule == "onetime" {
				if amount == nil || currencyCode == "" {
					return errors.New("Invalid price details for client country: " + clientCountry)
				}
				display, err := subscriptions.FormatConfiguredPrice(amount, currencyCode, false)
				if err != nil {
					return err
				}
				view.AddKey("price", display+"/"+"One Time")
				view.AddKey("type", "onetime")
				view.AddKey("razorpay_payment_link", paymentEntryURL("razorpay", story.ID, customId, redirectUri))
				view.AddKey("amount", amount)
				view.AddKey("currency", currency)
			} else if story.Schedule == "monthly" || story.Schedule == "yearly" {
				if planId == "" {
					return errors.New("Invalid price details for client country: " + clientCountry)
				}
				razorpayClient := razorpay.NewClient(config.Get("razorpay_key_id"), config.Get("razorpay_key_secret"))
				plan, err := razorpayClient.Plan.Fetch(planId, nil, nil)
				if err != nil {
					return server.InternalError(err)
				}
				item, ok := plan["item"].(map[string]interface{})
				if !ok {
					return errors.New("invalid plan item")
				}
				itemCurrency, ok := item["currency"].(string)
				if !ok {
					return errors.New("invalid plan currency")
				}
				label := "Monthly"
				if story.Schedule == "yearly" {
					label = "Yearly"
				}
				display, err := subscriptions.FormatConfiguredPrice(item["amount"], itemCurrency, true)
				if err != nil {
					return err
				}
				view.AddKey("price", display+"/"+label)
				view.AddKey("type", "subscription")
				view.AddKey("razorpay_payment_link", paymentEntryURL("razorpay", story.ID, customId, redirectUri))
			}
			view.AddKey("razorpay", config.GetBool("razorpay"))

		default:
			log.Error(log.V{"Show, Invalid payment gateway selected": pg, "country": clientCountry})
			return errors.New("invalid payment gateway: " + pg + " for country: " + clientCountry)
		}

		view.AddKey("showSubscribe", true)

	} else {
		view.AddKey("showSubscribe", false)
	}

	return view.Render()
}

// MetaHashTag removes #from hashtag and returns a single string formatted for meta Keywords
func MetaHashTag(hashtags []string) string {
	var metahashtag = ""
	for _, s := range hashtags {
		metahashtag = metahashtag + strings.Replace(s, "#", "", -1) + ","
	}
	return metahashtag
}

// truncateString shortens name to at most limit characters, ending in "..."
// only when it was actually shortened.
func truncateString(name string, limit int) string {
	if len([]rune(name)) <= limit {
		return name
	}
	if limit <= 3 {
		return string([]rune(name)[:limit])
	}
	return string([]rune(name)[:limit-3]) + "..."
}

func paymentEntryURL(gateway string, id int64, customID, redirect string) string {
	return "/subscriptions/" + gateway + "?" + url.Values{"product_id": {strconv.FormatInt(id, 10)}, "custom_id": {customID}, "redirect_uri": {redirect}}.Encode()
}
