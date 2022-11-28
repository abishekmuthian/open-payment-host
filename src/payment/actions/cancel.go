package paymentactions

import (
	"net/http"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

// HandlePaymentFailure handles the success routine of the payment
func HandlePaymentCancel(w http.ResponseWriter, r *http.Request) error {

	// Authorise
	currentUser := session.CurrentUser(w, r)
	log.Info(log.V{"Payment Cancelled, User ID: ": currentUser.UserID()})

	// Render the template
	view := view.NewRenderer(w, r)
	view.AddKey("currentUser", currentUser)
	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())

	view.Template("payment/views/payment_cancel.html.got")

	return view.Render()
}
