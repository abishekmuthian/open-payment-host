package storyactions

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/status"
	"github.com/abishekmuthian/open-payment-host/src/products"

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

	// Set subscribe button if price is set for Stripe
	if len(story.SquarePrice) != 0 || len(story.Price) != 0 {

		// Get the country from IP
		clientCountry := r.Header.Get("CF-IPCountry")
		log.Info(log.V{"Subscription, Client Country": clientCountry})
		if !config.Production() {
			// There will be no CF request header in the development/test
			clientCountry = config.Get("subscription_client_country")
		}

		// Check if its Stripe
		if config.GetBool("stripe") {

			priceId := story.Price[clientCountry]

			log.Info(log.V{"Price ID: ": priceId})

			stripe.Key = config.Get("stripe_secret")

			p, err := price.Get(priceId, nil)

			if err == nil {

				log.Info(log.V{"Currency:": p.Currency})

				view.AddKey("priceId", priceId)

				if p.Type == "recurring" {
					view.AddKey("price", strconv.FormatInt(p.UnitAmount/100, 10)+" "+string(p.Currency)+"/"+string(p.Recurring.Interval))
				} else if p.Type == "one_time" {
					view.AddKey("price", strconv.FormatInt(p.UnitAmount/100, 10)+" "+string(p.Currency)+"/"+"One Time")
				}
			}
		} else if config.GetBool("square") {
			amount := story.SquarePrice[clientCountry]["amount"]
			currency := story.SquarePrice[clientCountry]["currency"]

			if amount != nil && currency != nil {
				if story.Schedule == "One Time" {
					view.AddKey("price", strconv.FormatFloat(amount.(float64)/1000, 'g', 5, 64)+" "+currency.(string)+"/"+"One Time")
					view.AddKey("type", "onetime")
				} else if story.Schedule == "Monthly Subscription" {
					view.AddKey("price", strconv.FormatFloat(amount.(float64)/1000, 'g', 5, 64)+" "+currency.(string)+"/"+"Monthly")
					view.AddKey("type", "subscription")
				}
			} else {
				if len(story.SquarePrice) > 0 {
					for clientCountry, _ = range story.SquarePrice {
						amount = story.SquarePrice[clientCountry]["amount"]
						currency = story.SquarePrice[clientCountry]["currency"]
					}
					if story.Schedule == "One Time" {
						view.AddKey("price", strconv.FormatFloat(amount.(float64)/1000, 'g', 5, 64)+" "+currency.(string)+"/"+"One Time")
						view.AddKey("type", "onetime")
					} else if story.Schedule == "Monthly Subscription" {
						view.AddKey("price", strconv.FormatFloat(amount.(float64)/1000, 'g', 5, 64)+" "+currency.(string)+"/"+"Monthly")
						view.AddKey("type", "subscription")
					}
				}

				if amount == "" && currency == "" {
					log.Error(log.V{"Product": "Amount and Currency not set"})
				}
			}

			view.AddKey("amount", amount)
			view.AddKey("currency", currency)

		}

		view.AddKey("showSubscribe", true)
		view.AddKey("stripe", config.GetBool("stripe"))
		view.AddKey("square", config.GetBool("square"))
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

func truncateString(name string, limit int) string {
	result := name
	chars := 0
	if len(name) > limit {
		if limit > 3 {
			limit -= 3
			for i := range name {
				if chars >= limit {
					result = name[:i]
					break
				}
				chars++
			}
		}
	}
	return result + "..."
}
