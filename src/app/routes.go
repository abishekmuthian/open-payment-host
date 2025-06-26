package app

import (
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux/middleware/gzip"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux/middleware/secure"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/subscriptions"

	// Resource Actions
	appactions "github.com/abishekmuthian/open-payment-host/src/app/actions"
	storyactions "github.com/abishekmuthian/open-payment-host/src/products/actions"
	subscriptionactions "github.com/abishekmuthian/open-payment-host/src/subscriptions/actions"
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
	router.Get("/products/create/price/{fieldIndex:[0-9]+}/{pg:[a-zA-Z]+}/{schedule:[a-zA-Z ]+}", storyactions.HandlePrice)
	router.Get("/products/create/schedule", storyactions.HandleSchedule)

	// Add suggestion route
	router.Post("/product/editor/suggestion", storyactions.HandleGetSuggestion)

	// Add file upload route
	router.Post("/product/editor/upload", storyactions.HandleFileAttachment)

	// No voting for the products yet
	// router.Get("/products/upvoted{format:(.xml)?}", storyactions.HandleListUpvoted)
	router.Get("/products/{id:[0-9]+}/update", storyactions.HandleUpdateShow)
	router.Post("/products/{id:[0-9]+}/update", storyactions.HandleUpdate)
	router.Post("/products/{id:[0-9]+}/destroy", storyactions.HandleDestroy)
	router.Get("/products/{id:[0-9]+}/subscription", subscriptions.HandleSubscriptionShow)
	router.Post("/products/{id:[0-9]+}/subscription/subscribe", subscriptions.HandleSubscription)
	router.Post("/products/{id:[0-9]+}/subscription/unsubscribe", subscriptions.HandleUnSubscription)
	// For show insights link the product page
	//router.Post("/products/{id:[0-9]+}/insights", storyactions.HandleInsights)
	router.Get("/products/{id:[0-9]+}", storyactions.HandleShow)
	router.Get("/products{format:(.xml)?}", storyactions.HandleIndex)
	router.Get("/sitemap.xml", storyactions.HandleSiteMap)

	// Add subscription routes for Square, Stripe, Paypal
	router.Post("/subscriptions/create-checkout-session", subscriptions.HandleCreateCheckoutSession)
	router.Get("/subscriptions/billing", subscriptions.HandleBillingShow)
	router.Post("/subscriptions/billing", subscriptions.HandleBilling)
	router.Get("/subscriptions/square", subscriptions.HandleSquareShow)
	router.Post("/subscriptions/square", subscriptions.HandleSquare)
	router.Get("/subscriptions/paypal", subscriptions.HandlePaypalShow)
	router.Post("/subscriptions/paypal/orders", subscriptions.HandlePaypalCreateOrder)
	router.Post("/subscriptions/paypal/orders/{id:[a-zA-Z0-9]+}/capture", subscriptions.HandlePaypalCaptureOrder)
	router.Get("/subscriptions/razorpay", subscriptions.HandleRazorpayShow)
	router.Post("/subscriptions/subscribe", subscriptions.HandleCreateSubscription)
	router.Get("/subscriptions/success", subscriptions.HandlePaymentSuccess)
	router.Get("/subscriptions/cancel", subscriptionactions.HandlePaymentCancel)
	router.Post("/subscriptions/stripe-webhook", subscriptions.HandleWebhook)
	router.Post("/subscriptions/square-webhook", subscriptions.HandleSquareWebhook)
	router.Post("/subscriptions/paypal-webhook", subscriptions.HandlePaypalWebhook)
	router.Post("/subscriptions/razorpay-webhook", subscriptions.HandleRazorpayWebhook)
	router.Get("/subscriptions/failure", subscriptions.HandlePaymentFailure)
	// Billing not yet active
	// router.Post("/subscriptions/manage-billing", subscriptions.HandleCustomerPortal)

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
