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
	view.AddKey("schedule", schedule)

	switch {
	case pg == "stripe":
		view.Template("products/views/stripe_price.html.got")
	case pg == "square":
		view.Template("products/views/square_price.html.got")
	case pg == "paypal" && schedule == "onetime":
		view.Template("products/views/paypal_price_onetime.html.got")
	case pg == "paypal" && (schedule == "monthly" || schedule == "yearly"):
		view.Template("products/views/paypal_price_monthly.html.got")
	case pg == "razorpay" && schedule == "onetime":
		view.Template("products/views/razorpay_price_onetime.html.got")
	case pg == "razorpay" && (schedule == "monthly" || schedule == "yearly"):
		view.Template("products/views/razorpay_price_monthly.html.got")
	default:
		return server.NotFoundError(nil, "Unknown price fields", "No price fields for gateway "+pg+" and schedule "+schedule)
	}

	view.Layout("")

	return view.Render()
}
