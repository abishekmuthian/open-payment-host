package paymentactions

import (
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/stats"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"net/http"
)

// HandlePaymentFailure handles the success routine of the payment
func HandlePaymentCancel(w http.ResponseWriter, r *http.Request) error {
	stats.RegisterHit(r)

	// Authorise
	currentUser := session.CurrentUser(w, r)
	log.Info(log.V{"Payment Cancelled, User ID: ": currentUser.UserID()})

	// Render the template
	view := view.NewRenderer(w, r)
	view.AddKey("currentUser", currentUser)

	view.Template("payment/views/payment_cancel.html.got")

	return view.Render()
}
