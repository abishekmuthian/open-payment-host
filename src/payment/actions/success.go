package paymentactions

import (
	"net/http"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/stats"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

// HandlePaymentSuccess handles the success routine of the payment
func HandlePaymentSuccess(w http.ResponseWriter, r *http.Request) error {
	stats.RegisterHit(r)

	// Authorise
	currentUser := session.CurrentUser(w, r)
	log.Info(log.V{"Payment Success, User ID: ": currentUser.UserID()})

	// Render the template
	view := view.NewRenderer(w, r)
	view.AddKey("currentUser", currentUser)
	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())

	view.Template("payment/views/payment_success.html.got")

	return view.Render()
}
