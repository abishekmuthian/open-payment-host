package payment

import (
	"github.com/abishekmuthian/open-payment-host/src/lib/auth/can"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/stats"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/users"
	//razorpay "github.com/razorpay/razorpay-go"
	"net/http"
)

// HandlePaymentShow shows the subscriptions page by responding to the GET request
func HandlePaymentShow(w http.ResponseWriter, r *http.Request) error {
	stats.RegisterHit(r)

	// Get the params
	/*	params, err := mux.Params(r)

		if err != nil {
			return server.InternalError(err)
		}*/

	// Handle Razorpay
	//var planID, subscriptionID string

	// Render the template
	view := view.NewRenderer(w, r)

	// Handle Razorpay
	/*	if params.Get("plan") == "ideator"{
			planID = config.Get("razorpay_plan_ideator")
		}
	*/
	//Create Razorpay subscription
	/*	client := razorpay.NewClient("rzp_test_tlWikPDhUlVTGS", "dHqWELGi3u2SH9LwagivIhkT")
		data := map[string]interface{}{
			"plan_id":         planID,
			"total_count":        "1200",
			"quantity":         "1",
			"customer_notify": 1,
		}
		body, err := client.Subscription.Create(data, nil)

		if err == nil{
			if body["status"] =="created"{
				subscriptionID = body["id"].(string)
			}
		}

		log.Info(log.V{"Subscription":"Razorpay","Body":body})*/

	// Handle PayPal
	//view.AddKey("paypalButton", config.Get("paypal_button"))

	// Handle Razorpay
	//view.AddKey("subscriptionID",subscriptionID)

	view.Template("subscriptions/views/payment.html.got")

	return view.Render()
}

// HandlePayment enables the subscription features after payment
func HandlePayment(w http.ResponseWriter, r *http.Request) error {
	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Find the user
	user, err := users.Find(params.GetInt(users.KeyName))
	if err != nil {
		return server.NotFoundError(err)
	}

	// Check the authenticity token
	err = session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Authorise update user
	currentUser := session.CurrentUser(w, r)

	//Authorize update for subscription
	err = can.Update(user, currentUser)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Validate the params, removing any we don't accept according to the role
	accepted := users.AllowedParams()
	if currentUser.Admin() {
		accepted = users.AllowedParamsAdmin()
	}
	userParams := user.ValidateParams(params.Map(), accepted)

	// Set subscription to true
	userParams["subscription"] = "true"

	err = user.Update(userParams)
	if err != nil {
		return server.InternalError(err)
	}

	// Redirect to user
	return server.Redirect(w, r, "/subscriptions/payment")
}
