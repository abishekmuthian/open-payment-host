package storyactions

import (
	"net/http"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

// HandlePrice updates the pricing field
// Responds to get /create/price
func HandlePrice(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	log.Info(log.V{"Params: ": params})

	fieldIndex := params.GetInt("fieldIndex")
	pg := params.Get("pg")
	schedule := params.Get("schedule")

	// Render the template
	view := view.NewRenderer(w, r)

	// view.AddKey("paypalSchedule", params.Get("paypal_schedule"))

	view.AddKey("fieldIndex", fieldIndex+1)

	if pg == "stripe" {
		view.Template("products/views/stripe_price.html.got")
	}

	if pg == "square" {
		view.Template("products/views/square_price.html.got")
	}

	if pg == "paypal" {
		if schedule == "onetime" {
			view.Template("products/views/paypal_price_onetime.html.got")
		} else if schedule == "monthly" {
			view.Template("products/views/paypal_price_monthly.html.got")
		}
	}

	if pg == "razorpay" {
		if schedule == "onetime" {
			view.Template("products/views/razorpay_price_onetime.html.got")
		} else if schedule == "monthly" {
			view.Template("products/views/razorpay_price_monthly.html.got")
		}
	}

	view.Layout("")

	return view.Render()
}
