package storyactions

import (
	"errors"
	"fmt"
	"net/http"
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

		// Find which price has the clientCountry

		var pg string
		if story.StripePrice[clientCountry] != "" {
			pg = "stripe"
			log.Info(log.V{"msg": "Show, Using Stripe Price as client country was found"})
		} else if story.SquarePrice[clientCountry]["amount"] != nil {
			pg = "square"
			log.Info(log.V{"msg": "Show, Using Square Price as client country was found"})
		} else if story.PaypalPrice[clientCountry]["amount"] != nil || story.PaypalPrice[clientCountry]["plan_id"] != nil {
			pg = "paypal"
			log.Info(log.V{"msg": "Show, Using PayPal Price as client country was found"})
		} else if story.RazorpayPrice[clientCountry]["amount"] != nil || story.RazorpayPrice[clientCountry]["plan_id"] != nil {
			pg = "razorpay"
			log.Info(log.V{"msg": "Show, Using Razorpay Price as client country was found"})
		} else if story.StripePrice["DF"] != "" {
			pg = "stripe"
			clientCountry = "DF"
			log.Info(log.V{"msg": "Show, Using Stripe Price as default country was found"})
		} else if story.SquarePrice["DF"]["amount"] != nil {
			pg = "square"
			clientCountry = "DF"
			log.Info(log.V{"msg": "Show, Using Square Price as default country was found"})

		} else if story.PaypalPrice["DF"]["amount"] != nil || story.PaypalPrice["DF"]["plan_id"] != nil {
			pg = "paypal"
			clientCountry = "DF"
			log.Info(log.V{"msg": "Show, Using PayPal Price as default country was found"})

		} else if story.RazorpayPrice["DF"]["amount"] != nil || story.RazorpayPrice["DF"]["plan_id"] != nil {
			pg = "razorpay"
			clientCountry = "DF"
			log.Info(log.V{"msg": "Show, Using Razorpay Price as default country was found"})
		}

		switch pg {
		case "stripe":
			// Code for Stripe
			priceId := story.StripePrice[clientCountry]

			if priceId == "" {
				return errors.New("Invalid price details for client country: " + clientCountry)
			}

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
			view.AddKey("stripe", config.GetBool("stripe"))
		case "square":
			// Code for Square
			amount := story.SquarePrice[clientCountry]["amount"]
			currency := story.SquarePrice[clientCountry]["currency"]

			if amount != nil && currency != nil {
				if story.Schedule == "onetime" {
					view.AddKey("price", strconv.FormatFloat(amount.(float64)/1000, 'g', 5, 64)+" "+currency.(string)+"/"+"One Time")
					view.AddKey("type", "onetime")
				} else if story.Schedule == "monthly" || story.Schedule == "yearly" {
					scheduleLabel := "Monthly"
					if story.Schedule == "yearly" {
						scheduleLabel = "Yearly"
					}
					view.AddKey("price", strconv.FormatFloat(amount.(float64)/1000, 'g', 5, 64)+" "+currency.(string)+"/"+scheduleLabel)
					view.AddKey("type", "subscription")
				}
			} else {
				return errors.New("Invalid price details for client country: " + clientCountry)

			}

			view.AddKey("amount", amount)
			view.AddKey("currency", currency)
			view.AddKey("square", config.GetBool("square"))
		case "paypal":
			// Code for PayPal
			amount := story.PaypalPrice[clientCountry]["amount"]
			currency := story.PaypalPrice[clientCountry]["currency"]
			planId := story.PaypalPrice[clientCountry]["plan_id"]

			if amount != nil && currency != nil {
				if story.Schedule == "onetime" {
					view.AddKey("price", strconv.FormatFloat(amount.(float64), 'g', 5, 64)+" "+currency.(string)+"/"+"One Time")
					view.AddKey("type", "onetime")
					view.AddKey("paypal_payment_link", "/subscriptions/paypal?"+fmt.Sprintf("type=%s&product_id=%d", "onetime", story.ID))
				} else if story.Schedule == "monthly" || story.Schedule == "yearly" {
					scheduleLabel := "Monthly"
					if story.Schedule == "yearly" {
						scheduleLabel = "Yearly"
					}
					view.AddKey("price", strconv.FormatFloat(amount.(float64), 'g', 5, 64)+" "+currency.(string)+"/"+scheduleLabel)
					view.AddKey("type", "subscription")
					view.AddKey("paypal_payment_link", "/subscriptions/paypal?"+fmt.Sprintf("type=%s&product_id=%d&plan_id=%s&redirect_uri=%s&custom_id=%s", "subscription", story.ID, planId.(string), redirectUri, customId))
				}
			} else {
				return errors.New("Invalid price details for client country: " + clientCountry)
			}

			view.AddKey("amount", amount)
			view.AddKey("currency", currency)
			view.AddKey("paypal", config.GetBool("paypal"))
		case "razorpay":
			// Code for Razorpay
			amount := story.RazorpayPrice[clientCountry]["amount"]
			currency := story.RazorpayPrice[clientCountry]["currency"]
			planId := story.RazorpayPrice[clientCountry]["plan_id"]
			if (amount != nil && currency != nil) || planId != nil {
				if story.Schedule == "onetime" {
					view.AddKey("price", strconv.FormatFloat(amount.(float64), 'g', 5, 64)+" "+currency.(string)+"/"+"One Time")
					view.AddKey("type", "onetime")
					view.AddKey("razorpay_payment_link", "/subscriptions/razorpay?"+fmt.Sprintf("type=%s&product_id=%d", "onetime", story.ID))
					view.AddKey("amount", amount)
					view.AddKey("currency", currency)
				} else if story.Schedule == "monthly" || story.Schedule == "yearly" {
					// Create a subscription using the plan id

					razorpayClient := razorpay.NewClient(config.Get("razorpay_key_id"), config.Get("razorpay_key_secret"))

					// Set total_count based on schedule: 120 for monthly (10 years), 30 for yearly (30 years)
					// Razorpay UPI payment method requires expire_at to be max 30 years
					totalCount := 120
					if story.Schedule == "yearly" {
						totalCount = 30
					}

					data := map[string]interface{}{
						"plan_id":     planId,
						"total_count": totalCount,
					}

					subscription, err := razorpayClient.Subscription.Create(data, nil)

					if err != nil {
						log.Error(log.V{"Show product, Error creating Razorpay subscription": err})
						return server.InternalError(err)
					}

					subscriptionId := subscription["id"].(string)

					if subscriptionId != "" {
						view.AddKey("type", "subscription")
						view.AddKey("razorpay_payment_link", "/subscriptions/razorpay?"+fmt.Sprintf("type=%s&product_id=%d&subscription_id=%s&redirect_uri=%s&custom_id=%s", "subscription", story.ID, subscriptionId, redirectUri, customId))
						razorpaySubscription, err := razorpayClient.Subscription.Fetch(subscriptionId, nil, nil)

						if err != nil {
							return errors.New("Error fetching Razorpay subscription: " + err.Error())
						}

						razorpayPlanId := razorpaySubscription["plan_id"]

						razorpayPlan, err := razorpayClient.Plan.Fetch(razorpayPlanId.(string), nil, nil)

						if err == nil {
							razorpayItem := razorpayPlan["item"].(map[string]interface{})

							razorpayAmount := razorpayItem["amount"]

							razorpayCurrency := razorpayItem["currency"]

							scheduleLabel := "Monthly"
							if story.Schedule == "yearly" {
								scheduleLabel = "Yearly"
							}
							view.AddKey("price", strconv.FormatFloat(razorpayAmount.(float64)/100, 'g', 5, 64)+" "+razorpayCurrency.(string)+"/"+scheduleLabel)
						} else {
							log.Error(log.V{"Product show, Error fetching razorpay amount": err})
						}

					}
				}
				view.AddKey("razorpay", config.GetBool("razorpay"))
			} else {
				return errors.New("Invalid price details for client country: " + clientCountry)
			}

		default:
			return errors.New("invalid payment gateway")
		}

		// Check which payment gateway has the price for this country and use it
		/* 		if story.StripePrice != nil && (story.StripePrice[clientCountry] != "" || story.StripePrice["DF"] != "") {

		   		} else if story.SquarePrice != nil && (story.SquarePrice[clientCountry]["amount"] != nil || story.SquarePrice["DF"]["amount"] != nil) {

		   		} else if story.PaypalPrice != nil && ((story.PaypalPrice[clientCountry]["amount"] != nil || story.PaypalPrice[clientCountry]["plan_id"] != nil) || (story.PaypalPrice["DF"]["amount"] != nil || story.PaypalPrice["DF"]["plan_id"] != nil)) {

		   		} else if story.RazorpayPrice != nil && ((story.RazorpayPrice[clientCountry]["amount"] != nil || story.RazorpayPrice[clientCountry]["plan_id"] != nil) || (story.RazorpayPrice["DF"]["amount"] != nil || story.RazorpayPrice["DF"]["plan_id"] != nil)) {
		   			if story.Schedule == "onetime" {
		   				amount := story.RazorpayPrice[clientCountry]["amount"]
		   				currency := story.RazorpayPrice[clientCountry]["currency"]
		   				if amount != nil && currency != nil {

		   					view.AddKey("price", strconv.FormatFloat(amount.(float64), 'g', 5, 64)+" "+currency.(string)+"/"+"One Time")
		   					view.AddKey("type", "onetime")
		   					view.AddKey("razorpay_payment_link", "/subscriptions/razorpay?"+fmt.Sprintf("type=%s&product_id=%d", "onetime", story.ID))

		   				} else {
		   					if len(story.RazorpayPrice) > 0 {
		   						clientCountry := "DF"
		   						amount := story.RazorpayPrice[clientCountry]["amount"]
		   						currency := story.RazorpayPrice[clientCountry]["currency"]
		   						if amount == nil || currency == nil {
		   							return errors.New("Invalid price details for client country: " + clientCountry)
		   						}
		   						view.AddKey("price", strconv.FormatFloat(amount.(float64), 'g', 5, 64)+" "+currency.(string)+"/"+"One Time")
		   						view.AddKey("type", "onetime")
		   						view.AddKey("razorpay_payment_link", "/subscriptions/razorpay?"+fmt.Sprintf("type=%s&product_id=%d", "onetime", story.ID))
		   					}
		   				}
		   				view.AddKey("amount", amount)
		   				view.AddKey("currency", currency)
		   			} else if story.Schedule == "monthly" {
		   				planId := story.RazorpayPrice[clientCountry]["plan_id"]
		   				// Create a subscription using the plan id

		   				razorpayClient := razorpay.NewClient(config.Get("razorpay_key_id"), config.Get("razorpay_key_secret"))

		   				data := map[string]interface{}{
		   					"plan_id":     planId,
		   					"total_count": 120,
		   				}

		   				subscription, err := razorpayClient.Subscription.Create(data, nil)

		   				if err != nil {
		   					log.Error(log.V{"Show product, Error creating Razorpay subscription": err})
		   					return server.InternalError(err)
		   				}

		   				subscriptionId := subscription["id"].(string)

		   				if subscriptionId != "" {
		   					// view.AddKey("price", strconv.FormatFloat(amount.(float64), 'g', 5, 64)+" "+currency.(string)+"/"+"Monthly")
		   					view.AddKey("type", "subscription")
		   					view.AddKey("razorpay_payment_link", "/subscriptions/razorpay?"+fmt.Sprintf("type=%s&product_id=%d&subscription_id=%s", "subscription", story.ID, subscriptionId))
		   				} else {
		   					if len(story.RazorpayPrice) > 0 {
		   						clientCountry := "DF"
		   						planId := story.RazorpayPrice[clientCountry]["plan_id"]
		   						// Create a subscription using the plan id

		   						razorpayClient := razorpay.NewClient(config.Get("razorpay_key_id"), config.Get("razorpay_key_secret"))

		   						data := map[string]interface{}{
		   							"plan_id":     planId,
		   							"total_count": 1,
		   						}

		   						subscription, err := razorpayClient.Subscription.Create(data, nil)

		   						if err != nil {
		   							log.Error(log.V{"Show product, Error creating Razorpay subscription": err})
		   							return server.InternalError(err)
		   						}

		   						subscriptionId := subscription["id"].(string)
		   						if subscriptionId == "" {
		   							return errors.New("Invalid subscription ID for client country: " + clientCountry)
		   						}

		   						view.AddKey("type", "subscription")
		   						view.AddKey("razorpay_payment_link", "/subscriptions/razorpay?"+fmt.Sprintf("type=%s&product_id=%d&subscription_id=%s", "subscription", story.ID, subscriptionId))
		   					}
		   				}

		   				razorpaySubscription, err := razorpayClient.Subscription.Fetch(subscriptionId, nil, nil)

		   				if err != nil {
		   					return errors.New("Error fetching Razorpay subscription: " + err.Error())
		   				}

		   				razorpayPlanId := razorpaySubscription["plan_id"]

		   				razorpayPlan, err := razorpayClient.Plan.Fetch(razorpayPlanId.(string), nil, nil)

		   				if err == nil {
		   					razorpayItem := razorpayPlan["item"].(map[string]interface{})

		   					razorpayAmount := razorpayItem["amount"]

		   					razorpayCurrency := razorpayItem["currency"]

		   					view.AddKey("price", strconv.FormatFloat(razorpayAmount.(float64)/100, 'g', 5, 64)+" "+razorpayCurrency.(string)+"/"+"Monthly")
		   				} else {
		   					log.Error(log.V{"Product show, Error fetching razorpay amount": err})
		   				}
		   			}
		   			view.AddKey("razorpay", config.GetBool("razorpay"))

		   		} */

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
