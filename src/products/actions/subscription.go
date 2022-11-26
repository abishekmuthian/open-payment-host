package storyactions

import (
	"net/http"
	"strconv"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth/can"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/stats"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/abishekmuthian/open-payment-host/src/subscriptions"
)

// HandleSubscriptionShow shows the subscriptions page by responding to the GET request
func HandleSubscriptionShow(w http.ResponseWriter, r *http.Request) error {
	stats.RegisterHit(r)

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Find the story
	story, err := products.Find(params.GetInt(products.KeyName))
	if err != nil {
		return server.NotFoundError(err)
	}

	// Get current user
	currentUser := session.CurrentUser(w, r)

	// User should be logged in to subscribe
	if currentUser.Anon() {
		return server.Redirect(w, r, "/users/login?redirecturl="+story.ShowURL()+"/subscription")
	}

	// Render the template
	view := view.NewRenderer(w, r)
	view.CacheKey(story.CacheKey())
	view.AddKey("story", story)
	view.AddKey("currentUser", currentUser)

	clientCountry := r.Header.Get("CF-IPCountry")
	log.Info(log.V{"Subscription, Client Country": clientCountry})
	if !config.Production() {
		// There will be no CF request header in the development/test
		clientCountry = config.Get("subscription_client_country")
	}

	if clientCountry == "IN" {
		view.AddKey("priceID", config.Get("stripe_price_id_ideator_IN"))
		view.AddKey("price", config.Get("stripe_price_IN"))
	} else {
		view.AddKey("priceID", config.Get("stripe_price_id_ideator_US"))
		view.AddKey("price", config.Get("stripe_price_US"))
	}

	return view.Render()
}

// HandleSubscription adds subscribers to the product by responding to the POST request
func HandleSubscription(w http.ResponseWriter, r *http.Request) error {
	stats.RegisterHit(r)

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Find the story
	story, err := products.Find(params.GetInt(products.KeyName))
	if err != nil {
		return server.NotFoundError(err)
	}

	// Check the authenticity token
	err = session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Authorise
	currentUser := session.CurrentUser(w, r)
	subscription := subscriptions.New()

	err = can.Create(subscription, currentUser)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Check if the user has email ID in their profile
	if currentUser.Email == "" {
		return server.Redirect(w, r, "/users/"+strconv.FormatInt(currentUser.ID, 10)+"/update?warning=Please save your email ID to use the subscription feature."+"&redirecturl="+story.ShowURL())
	}

	//products.AddSubscribers(currentUser.ID,story.ID)
	err = AddSubscribers(story.ID)

	return server.Redirect(w, r, story.ShowURL())
}

// HandleUnSubscription removes subscribers to the product by responding to the POST request
func HandleUnSubscription(w http.ResponseWriter, r *http.Request) error {
	stats.RegisterHit(r)

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Find the story
	story, err := products.Find(params.GetInt(products.KeyName))
	if err != nil {
		return server.NotFoundError(err)
	}

	// Check the authenticity token
	err = session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Authorise
	currentUser := session.CurrentUser(w, r)
	subscription := subscriptions.New()

	err = can.Create(subscription, currentUser)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	err = RemoveSubscribers(story.ID)

	return server.Redirect(w, r, story.ShowURL())
}

// AddSubscribers adds subscribers to the product
func AddSubscribers(productId int64) error {
	_, err := query.Exec("UPDATE products SET subscribers= subscribers + 1 WHERE id=$1", productId)
	return err
}

// RemoveSubscribers removes subscribers to the product
func RemoveSubscribers(productId int64) error {
	_, err := query.Exec("UPDATE products SET subscribers= subscribers - 1 WHERE id=$1", productId)
	return err
}

// Subscribed returns true or false depending upon when the user has subscribed to the product
// FIX ME: Sqlite3 doesn't have Any
/* func Subscribed(productId int64, userId int64) (bool, error) {
	q := products.Query()

	q.Where("? = ANY(subscribers)", userId).Where("id=?", productId)
	subscription, err := products.FindAll(q)

	if subscription != nil {
		return true, nil
	}
	return false, err
} */

// ListSubscriptions returns the list of subscriptions for the user
func ListSubscriptions(userId int64) ([]*products.Story, error) {
	q := products.Query()
	q.Where("? = ANY(subscribers)", userId)

	// Fetch the products
	results, err := products.FindAll(q)
	if err != nil {
		log.Error(log.V{"message": "products: error getting subscribed products", "error": err})
		return nil, err
	}

	return results, nil

}
