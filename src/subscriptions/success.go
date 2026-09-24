package subscriptions

import (
	"errors"
	"net/http"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/razorpay/razorpay-go/utils"
)

// renderPaymentSuccessWithDownload renders the payment success page, optionally
// triggering a file download via a presigned URL. When downloadUrl is non-empty,
// the template auto-triggers the download after rendering so the user sees the
// "Payment successful" page rather than the billing details page they came from.
func renderPaymentSuccessWithDownload(w http.ResponseWriter, r *http.Request, downloadUrl string) error {
	currentUser := session.CurrentUser(w, r)
	view := view.NewRenderer(w, r)
	view.AddKey("currentUser", currentUser)
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())
	if downloadUrl != "" {
		view.AddKey("downloadUrl", downloadUrl)
	}
	view.Template("subscriptions/views/payment_success.html.got")
	return view.Render()
}

// HandlePaymentSuccess renders the clean success URL after a verified payment.
// The download capability is held by the short-lived completion token cookie,
// not by the URL, so the presigned URL never reaches browser history or
// referrer headers while the download button can still be retried.
// It also processes the Razorpay and PayPal success callbacks from checkout
// JavaScript, which are verified server-side against the payment attempt and
// the provider API before any fulfillment happens.
func HandlePaymentSuccess(w http.ResponseWriter, r *http.Request) error {
	paymentResponseHeaders(w)
	if r.URL.Query().Has("product_id") {
		return server.BadRequestError(errors.New("product_id is not accepted on success"))
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Gateway success callbacks carry provider identifiers and signatures.
	// A product_id on a success URL is ignored and rejected; the product is
	// always resolved from the server-side payment attempt.
	if providerCallback := isProviderCallback(params); providerCallback {
		return handleProviderCallback(w, r, params)
	}

	// Clean success URL: /subscriptions/success?attempt_id=<opaque id>
	return handleCleanSuccessURL(w, r, params)
}

// isProviderCallback returns true when the request carries provider checkout
// identifiers or signatures which must be verified before fulfillment.
func isProviderCallback(params *mux.RequestParams) bool {
	for _, key := range []string{
		"razorpay_payment_id", "razorpay_order_id", "razorpay_subscription_id", "razorpay_signature",
		"paypal_orderid", "paypal_subscriptionid",
	} {
		if v := params.Get(key); v != "" && v != "null" {
			return true
		}
	}
	return false
}

// handleProviderCallback verifies Razorpay and PayPal checkout callbacks against
// the payment attempt and the provider, then redirects to the clean success URL
// with the completion token exchanged for a cookie.
func handleProviderCallback(w http.ResponseWriter, r *http.Request, params *mux.RequestParams) error {
	razorpayOrderId := params.Get("razorpay_order_id")
	razorpaySubscriptionId := params.Get("razorpay_subscription_id")
	razorpayPaymentId := params.Get("razorpay_payment_id")
	razorpaySignature := params.Get("razorpay_signature")

	paypalOrderId := params.Get("paypal_orderid")
	paypalSubscriptionId := params.Get("paypal_subscriptionid")

	switch {
	case razorpayOrderId != "" && razorpayOrderId != "null" && razorpayPaymentId != "" && razorpayPaymentId != "null":
		return handleRazorpayOrderSuccess(w, r, razorpayOrderId, razorpayPaymentId, razorpaySignature)

	case razorpaySubscriptionId != "" && razorpaySubscriptionId != "null" && razorpayPaymentId != "" && razorpayPaymentId != "null":
		return handleRazorpaySubscriptionSuccess(w, r, razorpaySubscriptionId, razorpayPaymentId, razorpaySignature)

	case paypalOrderId != "" && paypalOrderId != "null":
		return handlePaypalOrderSuccess(w, r, paypalOrderId)

	case paypalSubscriptionId != "" && paypalSubscriptionId != "null":
		return handlePaypalSubscriptionSuccess(w, r, paypalSubscriptionId)
	}

	// Malformed callback, fail closed
	return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
}

// handleRazorpayOrderSuccess verifies a Razorpay one-time payment callback.
// The signature is verified using the order id stored in the payment attempt
// rather than trusting the id returned by checkout, then the payment is fetched
// from the Razorpay API to confirm captured status, amount and currency.
func handleRazorpayOrderSuccess(w http.ResponseWriter, r *http.Request, orderId string, paymentId string, signature string) error {
	attempt, err := FindAttemptByProviderOrder("razorpay", orderId)
	if err != nil {
		log.Error(log.V{"Razorpay success, no payment attempt for order": orderId, "error": err})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	// The attempt must belong to the browser which started this checkout
	if !bindAttemptToRequest(r, attempt) {
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	// Verify the callback signature using the order id stored on the server
	razorpayParams := map[string]interface{}{
		"razorpay_order_id":   attempt.ProviderOrderId,
		"razorpay_payment_id": paymentId,
	}
	if !utils.VerifyPaymentSignature(razorpayParams, signature, config.Get("razorpay_key_secret")) {
		log.Error(log.V{"Razorpay success, order verification failed": orderId})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	// Fetch the payment from the Razorpay API and require captured status,
	// matching amount, currency and order.
	facts, err := fetchRazorpayPaymentFacts(attempt.ProviderOrderId, paymentId)
	if err != nil {
		log.Error(log.V{"Razorpay success, payment verification failed": err})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	_, err = verifyAndFulfill(attempt, facts)
	if err != nil {
		log.Error(log.V{"Razorpay success, fulfillment rejected": err})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	return completeAndRedirect(w, r, attempt, facts)
}

// handleRazorpaySubscriptionSuccess verifies a Razorpay subscription callback
// against the payment attempt, fetching the subscription and payment from the
// Razorpay API to confirm the initial charge.
func handleRazorpaySubscriptionSuccess(w http.ResponseWriter, r *http.Request, subscriptionId string, paymentId string, signature string) error {
	attempt, err := FindAttemptByProviderSubscription("razorpay", subscriptionId)
	if err != nil {
		log.Error(log.V{"Razorpay success, no payment attempt for subscription": subscriptionId, "error": err})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	if !bindAttemptToRequest(r, attempt) {
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	// Verify the callback signature using the subscription id stored on the server
	razorpayParams := map[string]interface{}{
		"razorpay_subscription_id": attempt.ProviderSubscriptionId,
		"razorpay_payment_id":      paymentId,
	}
	if !utils.VerifySubscriptionSignature(razorpayParams, signature, config.Get("razorpay_key_secret")) {
		log.Error(log.V{"Razorpay success, subscription verification failed": subscriptionId})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	// Fetch the payment and subscription from the Razorpay API. The initial
	// charge must be captured before the subscription is reported active.
	facts, err := fetchRazorpaySubscriptionFacts(attempt.ProviderSubscriptionId, paymentId)
	if errors.Is(err, errPaymentPending) {
		return renderPaymentPending(w, r, attempt)
	}
	if err != nil {
		log.Error(log.V{"Razorpay success, subscription verification failed": err})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	_, err = verifyAndFulfill(attempt, facts)
	if err != nil {
		log.Error(log.V{"Razorpay success, fulfillment rejected": err})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	return completeAndRedirect(w, r, attempt, facts)
}

// handlePaypalOrderSuccess verifies a PayPal one-time order by fetching the
// order from the PayPal API and validating the completed capture and the
// purchase unit amount, currency and attempt reference against the attempt.
func handlePaypalOrderSuccess(w http.ResponseWriter, r *http.Request, orderId string) error {
	orderDetails, err := fetchPaypalOrderDetails(orderId)
	if err != nil {
		log.Error(log.V{"Paypal success, error fetching order": err})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	if len(orderDetails.PurchaseUnits) == 0 {
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	// Resolve the attempt from the server-generated reference id frozen at order creation
	attemptId := orderDetails.PurchaseUnits[0].ReferenceID
	attempt, err := FindAttempt(attemptId)
	if err != nil {
		log.Error(log.V{"Paypal success, no payment attempt for order": orderId, "error": err})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	if !bindAttemptToRequest(r, attempt) {
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	facts, err := paypalOrderFacts(orderDetails, attempt)
	if err != nil {
		log.Error(log.V{"Paypal success, order verification failed": err})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	_, err = verifyAndFulfill(attempt, facts)
	if err != nil {
		log.Error(log.V{"Paypal success, fulfillment rejected": err})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	return completeAndRedirect(w, r, attempt, facts)
}

// handlePaypalSubscriptionSuccess verifies a PayPal subscription by fetching
// the subscription and its initial transaction from the PayPal API. An ACTIVE
// status alone is not sufficient, the initial payment must be completed.
func handlePaypalSubscriptionSuccess(w http.ResponseWriter, r *http.Request, subscriptionId string) error {
	attempt, err := FindAttemptByProviderSubscription("paypal", subscriptionId)
	if err != nil {
		log.Error(log.V{"Paypal success, no payment attempt for subscription": subscriptionId, "error": err})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	if !bindAttemptToRequest(r, attempt) {
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	facts, err := fetchPaypalSubscriptionFacts(attempt)
	if errors.Is(err, errPaymentPending) {
		return renderPaymentPending(w, r, attempt)
	}
	if err != nil {
		log.Error(log.V{"Paypal success, subscription verification failed": err})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	_, err = verifyAndFulfill(attempt, facts)
	if err != nil {
		log.Error(log.V{"Paypal success, fulfillment rejected": err})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+verification+failed")
	}

	return completeAndRedirect(w, r, attempt, facts)
}

// completeAndRedirect exchanges the completion token for a cookie and redirects
// to the clean success URL, or redirects to the stored validated redirect URI
// with the frozen experimental API parameters.
func completeAndRedirect(w http.ResponseWriter, r *http.Request, attempt *PaymentAttempt, facts ProviderFacts) error {
	if attempt.Status != "completed" {
		return server.BadRequestError(errors.New("payment is not confirmed"))
	}
	if err := issueCompletionCookie(w, r, attempt); err != nil {
		return server.InternalError(err)
	}
	return server.Redirect(w, r, "/subscriptions/success?attempt_id="+attempt.Id)
}
func issueCompletionCookie(w http.ResponseWriter, r *http.Request, a *PaymentAttempt) error {
	token, err := generateToken(32)
	if err != nil {
		return err
	}
	a.CompletionTokenHash = hashToken(token)
	if err := a.Save(); err != nil {
		return err
	}
	setCompletionCookie(w, r, token)
	return nil
}
func paymentResponseHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}
func handleCleanSuccessURL(w http.ResponseWriter, r *http.Request, params *mux.RequestParams) error {
	a, err := FindAttempt(params.Get("attempt_id"))
	if err != nil {
		return server.NotAuthorizedError(errors.New("payment could not be verified"))
	}
	if a.Status == "pending" && bindAttemptToRequest(r, a) {
		return renderPaymentPending(w, r, a)
	}
	if !completionCookieValid(r, a) || a.Status != "completed" {
		return server.NotAuthorizedError(errors.New("payment could not be verified"))
	}
	p, err := products.Find(a.ProductId)
	if err != nil {
		return server.InternalError(err)
	}
	download, err := generateDownloadURL(p)
	if download != "" && err == nil {
		return renderPaymentSuccessWithDownload(w, r, download)
	}
	if a.RedirectURI != "" && (a.Gateway == "paypal" || a.Gateway == "razorpay") {
		params := map[string]string{"custom_id": a.CustomId}
		if a.Schedule == "onetime" {
			params["order_id"] = a.ProviderOrderId
		} else {
			params["subscription_id"] = a.ProviderSubscriptionId
		}
		return server.RedirectExternal(w, r, BuildRedirectURL(a.RedirectURI, params))
	}
	if err != nil {
		return server.InternalError(err)
	}
	return renderPaymentSuccessWithDownload(w, r, "")
}

// renderPaymentPending shows the verification page for subscription payments
// awaiting provider confirmation.
func renderPaymentPending(w http.ResponseWriter, r *http.Request, attempt *PaymentAttempt) error {
	currentUser := session.CurrentUser(w, r)
	view := view.NewRenderer(w, r)
	view.AddKey("currentUser", currentUser)
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())
	view.AddKey("loadHypermedia", true)
	view.AddKey("attemptId", attempt.Id)
	view.AddKey("title", "Payment verification in progress")
	view.AddKey("message", "Your payment is being verified by the payment gateway. This page will update when the payment is confirmed.")
	view.Template("subscriptions/views/verification.html.got")
	return view.Render()
}
