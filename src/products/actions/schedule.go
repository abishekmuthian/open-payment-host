package storyactions

import (
	"net/http"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

// HandleSchedule updates the pricing field based on the selected schedule
// Responds to get /create/schedule
func HandleSchedule(w http.ResponseWriter, r *http.Request) error {
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

	// Render the template
	view := view.NewRenderer(w, r)

	view.AddKey("paypalSchedule", params.Get("paypal_schedule"))

	view.Template("products/views/schedule.html.got")
	view.Layout("")

	return view.Render()
}
