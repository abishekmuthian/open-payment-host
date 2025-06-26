package useractions

import (
	"strconv"
	"strings"
	"time"

	m "github.com/abishekmuthian/open-payment-host/src/lib/mandrill"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/abishekmuthian/open-payment-host/src/subscriptions"
	"github.com/abishekmuthian/open-payment-host/src/users"
)

func EmailMonthlyInsights() {
	// Find the users who are subscribed
	q := users.Subscribed()
	subscribers, err := users.FindAll(q)
	if err == nil {

		for _, subscriber := range subscribers {
			if subscriber.Notification {
				var bodyContent strings.Builder

				currentMonth := time.Now().UTC().AddDate(0, 0, -30)

				subscriptions, err := subscriptions.ListSubscriptions(subscriber.ID)
				if err == nil {
					log.Info(log.V{"Notification email, Subscriptions": subscriptions})

					q := products.GetTrendingStory()
					products, err := products.FindAll(q)
					if err == nil {
						// Write the trending need gap from the past 30 days
						bodyContent.WriteString(
							"<h3>" +
								"Trending need gap of " + currentMonth.Format("January") +
								"</h3>" +
								"<p>" +
								"<a style=\"color: #D32F2F;\" href=\"" + products[0].CompleteURL() + "\"" + ">" + products[0].Name + "</a>" + "<br/>" +
								"<br/>" +
								"30 days page views: " + strconv.FormatInt(products[0].ThirtyDaysPageViews, 10) +
								"<br/>" +
								"30 days top 3 countries: " + products[0].ThirtyDaysTop3Countries +
								"</p>" +
								"<br/><br/>")
					}

					bodyContent.WriteString(
						"<h3>" +
							"Insights of your subscribed need gaps for " + currentMonth.Format("January") +
							"</h3>")

					for _, product := range subscriptions {

						bodyContent.WriteString(
							"<p>" +
								"<a style=\"color: #D32F2F;\" href=\"" + product.CompleteURL() + "\"" + ">" + product.Name + "</a>" + "<br/>" +
								"<br/>" +
								"30 days page views: " + strconv.FormatInt(product.ThirtyDaysPageViews, 10) +
								"<br/>" +
								"30 days top 3 countries: " + product.ThirtyDaysTop3Countries +
								"</p>" +
								"<br/>")
					}

				} else {
					log.Error(log.V{"Notification email, Error getting subscriptions": err})
				}

				client := m.ClientWithKey(config.Get("mandrill_key"))
				fromEmail := config.Get("notification_email")
				fromName := "open-payment-host"
				message := &m.Message{}
				message.FromEmail = fromEmail
				message.FromName = fromName
				message.Subject = "Your monthly insights for " + currentMonth.Format("January") + " from open-payment-host is here"

				message.AddRecipient(subscriber.Email, subscriber.Name, "to")
				// Global vars
				message.GlobalMergeVars = m.MapToVars(map[string]interface{}{"UNAME": subscriber.Name, "PASTMONTH": currentMonth.Format("January"), "MONTHLYINSIGHTS": bodyContent.String()})
				templateContent := map[string]string{}

				response, err := client.MessagesSendTemplate(message, config.Get("mailchimp_monthly_insights_template"), templateContent)
				if err != nil {
					log.Error(log.V{"msg": "Notification email, Error sending monthly insights to subscribers", "error": err})
				} else {
					log.Info(log.V{"msg": "Notification email, Monthly insights emailed to the subscribers successfully", "response": response})
				}
			} else {
				log.Info(log.V{"Notification email, ": "Subscriber doesn't have the notification enabled"})
			}

		}

	} else {
		log.Info(log.V{"Email, Error finding subscribers": err})
	}
}
