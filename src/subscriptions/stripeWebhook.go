package subscriptions

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/abishekmuthian/open-payment-host/src/lib/mailchimp"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/customer"
	"github.com/stripe/stripe-go/v72/webhook"
)

// HandleWebhook receives the webhook POST request from the payment gateways
func HandleWebhook(w http.ResponseWriter, r *http.Request) error {

	//subscription := New()

	// Set your secret key. Remember to switch to your live secret key in production.
	// See your keys here: https://dashboard.stripe.com/account/apikeys
	stripe.Key = config.Get("stripe_secret")

	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return nil
	}

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Error(log.V{"ioutil.ReadAll: %v": err})
		return err
	}

	// Check if the event is from Stripe
	webhookEvent, err := webhook.ConstructEvent(b, r.Header.Get("Stripe-Signature"), config.Get("stripe_webhook_secret"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Error(log.V{"webhook.ConstructEvent: ": err})
		return err
	}

	var event Event

	err = json.Unmarshal(b, &event)
	if err != nil {
		log.Error(log.V{"Webhook JSON Unmarshall": err})
	}

	log.Info(log.V{"Webhook event parsed": event})

	switch webhookEvent.Type {
	case "checkout.session.completed":
		// Payment is successful and the subscription is created.
		// You should provision the subscription.
		log.Info(log.V{"Stripe": "Checkout session completed"})

		var subscription *Subscription

		switch event.Data.Object.Mode {
		case "payment":
			subscription, err = FindPayment(event.Data.Object.PaymentIntent)
			if err != nil {
				log.Info(log.V{"Webhook, error finding subscription using Payment Intent": err})
			}
		case "subscription":
			subscription, err = FindSubscription(event.Data.Object.Subscription)
			if err != nil {
				log.Error(log.V{"Webhook, error finding subscription using Subscription Id": err})
			}
		}

		if subscription == nil {
			subscription := New()
			customer, err := customer.Get(event.Data.Object.Customer, nil)
			if err == nil {
				event.Data.Object.BillingDetails.Name = customer.Name
				err := recordSubscriptionPaymentTransaction(event, subscription)
				if err != nil {
					log.Error(log.V{"Webhook, error recording subscription transaction": err})
				} else {

					productID, err := strconv.ParseInt(event.Data.Object.MetaData.ProductID, 10, 64)

					if err == nil {
						// TODO: First value must be set manually, Default is not 0
						AddSubscribers(productID)

						story, err := products.Find(productID)

						if err == nil {
							// If mailchimp list id and mailchimp token is available add to the mailchimp list
							if story.MailchimpAudienceID != "" && config.Get("mailchimp_token") != "" {
								// Add to the mailchimp list
								audience := mailchimp.Audience{
									MergeFields: mailchimp.Merge{FirstName: event.Data.Object.BillingDetails.Name},
									Email:       event.Data.Object.CustomerDetails.Email,
									Status:      "subscribed",
								}
								go mailchimp.AddToAudience(audience, story.MailchimpAudienceID, mailchimp.GetMD5Hash(event.Data.Object.CustomerDetails.Email), config.Get("mailchimp_token"))
							}
						} else {
							log.Error(log.V{"Webhook, Error finding product in the webhook for adding to mailchimp": err})
						}
					} else {
						log.Error(log.V{"Webhook, error converting string product_Id to int64": err})

					}
				}
			}
		} else {
			log.Info(log.V{"Webhook subscription already present in the DB": subscription.ID})
		}

	case "payment_method.attached":
		// Payment method attached trying to get address
		log.Info(log.V{"Stripe": "Payment method attached"})
		params := &stripe.CustomerParams{
			Name: stripe.String(event.Data.Object.BillingDetails.Name),
			Address: &stripe.AddressParams{
				City:       stripe.String(event.Data.Object.BillingDetails.Address.City),
				Country:    stripe.String(event.Data.Object.BillingDetails.Address.Country),
				Line1:      stripe.String(event.Data.Object.BillingDetails.Address.Line1),
				Line2:      stripe.String(event.Data.Object.BillingDetails.Address.Line2),
				PostalCode: stripe.String(event.Data.Object.BillingDetails.Address.PostalCode),
				State:      stripe.String(event.Data.Object.BillingDetails.Address.State),
			},
			// Custom Fields for the Customer
			// Use this with custom flow when using stripe elements
			/*			InvoiceSettings: &stripe.CustomerInvoiceSettingsParams{
						Stripe		CustomFields: []*stripe.CustomerInvoiceCustomFieldParams{
									{
										Name:  stripe.String("HSN"),
										Value: stripe.String("9983"),
									},

								},
								Footer: stripe.String("SUPPLY MEANT FOR EXPORT UNDER BOND OR LETTER OF UNDERTAKING WITHOUT PAYMENT OF INTEGRATED TAX"),
							},*/
		}

		c, err := customer.Update(
			event.Data.Object.Customer,
			params,
		)

		if err == nil {
			log.Info(log.V{"Stripe, Updated Customer": c})
		} else {
			log.Error(log.V{"Stripe, Error updating customer": err})
		}

	case "invoice.payment_failed":
		// The payment failed or the customer does not have a valid payment method.
		// The subscription becomes past_due. Notify your customer and send them to the
		// customer portal to update their payment information.
		log.Info(log.V{"Stripe": "Invoice failed"})
	case "customer.subscription.deleted":
		// Subscription cancelled
		log.Info(log.V{"Stripe": "Subscription cancelled"})
		subscriptionId := event.Data.Object.ID
		subscription, err := Find(subscriptionId)
		if err != nil {
			log.Error(log.V{"Webhook, Error finding subscription": err})
		}

		if subscription == nil {
			log.Error(log.V{"Webhook, customer.subscription.deleted": "Subscription not found"})
		} else {

			story, err := products.Find(subscription.ProductId)

			if err == nil {
				// TODO: Implement subscription cancellation update for Stripe

				// If mailchimp list id and mailchimp token is available add to the mailchimp list
				if story.MailchimpAudienceID != "" && config.Get("mailchimp_token") != "" {
					// Add to the mailchimp list
					audience := mailchimp.Audience{
						MergeFields: mailchimp.Merge{FirstName: event.Data.Object.BillingDetails.Name},
						Email:       event.Data.Object.CustomerDetails.Email,
						Status:      "unsubscribed",
					}
					go mailchimp.UpdateToAudience(audience, story.MailchimpAudienceID, mailchimp.GetMD5Hash(event.Data.Object.CustomerDetails.Email), config.Get("mailchimp_token"))
				}
			} else {
				log.Error(log.V{"Webhook, Error finding product in the webhook for adding to mailchimp": err})
			}
		}
	default:
		// unhandled event type
		log.Error(log.V{"Stripe": "Webhook default case"})
	}

	return err
}

// recordSubscriptionPaymentTransaction adds the transaction to database
func recordSubscriptionPaymentTransaction(event Event, subscription *Subscription) error {
	// Params not validated using ValidateParams as user did not create these?
	transactionParams := make(map[string]string)
	transactionParams["pg"] = "stripe"
	transactionParams["txn_id"] = event.Data.Object.PaymentIntent
	transactionParams["payment_date"] = query.TimeString(event.Created.Time.UTC())
	transactionParams["receipt_id"] = event.Data.Object.ID
	transactionParams["mc_gross"] = strconv.FormatFloat(event.Data.Object.AmountSubTotal, 'E', -1, 64)
	transactionParams["payment_gross"] = strconv.FormatFloat(event.Data.Object.AmountTotal, 'E', -1, 64)
	transactionParams["mc_currency"] = event.Data.Object.Currency
	transactionParams["payer_id"] = event.Data.Object.Customer
	transactionParams["payer_email"] = event.Data.Object.CustomerDetails.Email
	transactionParams["txn_type"] = event.Data.Object.Mode
	transactionParams["payment_status"] = event.Data.Object.PaymentStatus
	transactionParams["subscr_id"] = event.Data.Object.Subscription
	transactionParams["tax"] = strconv.FormatFloat(event.Data.Object.TotalDetails.AmountTax, 'E', -1, 64)
	transactionParams["user_id"] = event.Data.Object.MetaData.UserID
	transactionParams["transaction_subject"] = event.Data.Object.MetaData.Plan
	transactionParams["item_name"] = event.Data.Object.MetaData.Plan
	transactionParams["item_number"] = event.Data.Object.MetaData.ProductID
	transactionParams["first_name"] = event.Data.Object.BillingDetails.Name

	if strings.Contains(event.Data.Object.ID, "cs_test") {
		transactionParams["test_pdt"] = strconv.FormatInt(1, 10)
	}

	dbId, err := subscription.Create(transactionParams)

	if err == nil {
		log.Info(log.V{"Webhook transaction added to db, ID: ": dbId})
	}

	return err
}
