package actions

import (
	"fmt"
	"net/http"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

func HandlePaypalShow(w http.ResponseWriter, r *http.Request) error {
	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Get current user
	currentUser := session.CurrentUser(w, r)

	amount := params.GetInt("amount")
	currency := params.Get("currency")
	paymentType := params.Get("type")

	// Render the template
	view := view.NewRenderer(w, r)

	view.AddKey("currentUser", currentUser)

	if paymentType == "onetime" {
		view.AddKey("price", fmt.Sprintf("%d %s/One Time", amount/1000, currency))
	} else if paymentType == "subscription" {
		view.AddKey("price", fmt.Sprintf("%d %s/Monthly", amount/1000, currency))
	}

	// Load the Paypal script
	view.AddKey("loadPaypalScript", true)

	// Add paypal client id
	view.AddKey("clientId", config.Get("paypal_client_id"))

	// Add paypal plan id
	view.AddKey("meta_plan_id", config.Get("paypal_plan_id"))

	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())
	return view.Render()
}
