package app

import (
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux/middleware/gzip"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux/middleware/secure"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"

	// Resource Actions
	appactions "github.com/abishekmuthian/open-payment-host/src/app/actions"
	paymentactions "github.com/abishekmuthian/open-payment-host/src/payment/actions"
	storyactions "github.com/abishekmuthian/open-payment-host/src/products/actions"
	subscriberactions "github.com/abishekmuthian/open-payment-host/src/subscriptions/actions"
	useractions "github.com/abishekmuthian/open-payment-host/src/users/actions"
)

// SetupRoutes creates a new router and adds the routes for this app to it.
func SetupRoutes() *mux.Mux {

	router := mux.New()
	mux.SetDefault(router)

	// Add the home page route
	router.Get("/", appactions.HandleHome)

	// Add a route to handle static files
	router.Get("/favicon.ico", fileHandler)
	router.Get("/files/{path:.*}", fileHandler)
	router.Get("/assets/{path:.*}", fileHandler)
	router.Get("/assets/icons/{path:.*}", fileHandler)

	// Add story routes
	router.Get("/index{format:(.xml)?}", storyactions.HandleIndex)
	router.Get("/products/create", storyactions.HandleCreateShow)
	router.Post("/products/create", storyactions.HandleCreate)

	// Add suggestion route
	router.Post("/product/editor/suggestion", storyactions.HandleGetSuggestion)

	// Add file upload route
	router.Post("/product/editor/upload", storyactions.HandleFileAttachment)

	// No voting for the products yet
	// router.Get("/products/upvoted{format:(.xml)?}", storyactions.HandleListUpvoted)
	router.Get("/products/{id:[0-9]+}/update", storyactions.HandleUpdateShow)
	router.Post("/products/{id:[0-9]+}/update", storyactions.HandleUpdate)
	router.Post("/products/{id:[0-9]+}/destroy", storyactions.HandleDestroy)
	router.Get("/products/{id:[0-9]+}/subscription", storyactions.HandleSubscriptionShow)
	router.Post("/products/{id:[0-9]+}/subscription/subscribe", storyactions.HandleSubscription)
	router.Post("/products/{id:[0-9]+}/subscription/unsubscribe", storyactions.HandleUnSubscription)
	// For show insights link the product page
	//router.Post("/products/{id:[0-9]+}/insights", storyactions.HandleInsights)
	router.Get("/products/{id:[0-9]+}", storyactions.HandleShow)
	router.Get("/products{format:(.xml)?}", storyactions.HandleIndex)
	router.Get("/sitemap.xml", storyactions.HandleSiteMap)

	// Add subscription routes for razorpay
	/*	router.Get("/subscriptions/payment/{plan:.*}", payment.HandlePaymentShow)
		router.Post("/subscriptions/payment/{id:[0-9]+}/success", payment.HandlePayment)
		router.Post("/subscriptions/payment/razorpay/verification", subscriberactions.HandleRazorpayPaymentVerification)*/

	// Add subscription routes for PayPal
	/*	router.Get("/subscriptions/verification", subscriberactions.HandleVerificationShow)
		router.Post("/subscriptions/verification", subscriberactions.HandleVerification)*/

	// Add subscription routes for Square, Stripe
	router.Post("/subscriptions/create-checkout-session", subscriberactions.HandleCreateCheckoutSession)
	router.Get("/subscriptions/billing", subscriberactions.HandleBillingShow)
	router.Post("/subscriptions/billing", subscriberactions.HandleBilling)
	router.Get("/subscriptions/square", subscriberactions.HandleSquareShow)
	router.Post("/subscriptions/square", subscriberactions.HandleSquare)
	router.Post("/subscriptions/subscribe", subscriberactions.HandleCreateSubscription)
	router.Get("/payment/success", paymentactions.HandlePaymentSuccess)
	router.Get("/payment/cancel", paymentactions.HandlePaymentCancel)
	router.Post("/payment/webhook", paymentactions.HandleWebhook)
	router.Post("/payment/square_webhook", paymentactions.HandleSquareWebhook)
	router.Get("/payment/failure", paymentactions.HandlePaymentFailure)
	// Billing not yet active
	// router.Post("/subscriptions/manage-billing", subscriberactions.HandleCustomerPortal)

	// Add user routes
	router.Get("/users/{id:[0-9]+}/password/change", useractions.HandlePasswordChangeShow)
	router.Post("/users/{id:[0-9]+}/password/change", useractions.HandlePasswordChange)
	router.Get("/users/login", useractions.HandleLoginShow)
	router.Post("/users/login", useractions.HandleLogin)
	router.Post("/users/logout", useractions.HandleLogout)

	// Set the default file handler
	router.FileHandler = fileHandler
	router.ErrorHandler = errHandler

	// Add middleware
	router.AddMiddleware(log.Middleware)
	router.AddMiddleware(session.Middleware)
	router.AddMiddleware(gzip.Middleware)
	router.AddMiddleware(secure.Middleware)

	return router
}
