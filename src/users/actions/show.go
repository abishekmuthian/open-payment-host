package useractions

import (
	"net/http"

	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/stats"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/abishekmuthian/open-payment-host/src/users"
)

// HandleShow displays a single user.
func HandleShow(w http.ResponseWriter, r *http.Request) error {
	stats.RegisterHit(r)

	// No authorisation on user show

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

	// Find logged in user (if any)
	currentUser := session.CurrentUser(w, r)

	var q *query.Query

	// Get the user comments
	/* 	if currentUser.ID == user.ID || currentUser.Admin() {
	   		q = comments.Where("user_id=?", user.ID).Limit(10).Order("created_at desc")
	   	} else {
	   		q = comments.Where("user_id=? AND points > 0", user.ID).Limit(10).Order("created_at desc")
	   	}
	   	userComments, err := comments.FindAll(q)
	   	if err != nil {
	   		return server.InternalError(err)
	   	} */

	// Get the user products
	if currentUser.ID == user.ID || currentUser.Admin() {
		q = products.Where("user_id=?", user.ID).Limit(50).Order("created_at desc")
	} else {
		q = products.Where("user_id=? AND status IS NULL OR status NOT IN (50,1)", user.ID).Limit(50).Order("created_at desc")
	}
	userproducts, err := products.FindAll(q)
	if err != nil {
		return server.InternalError(err)
	}

	// Render the template
	view := view.NewRenderer(w, r)
	view.CacheKey(user.CacheKey())
	view.AddKey("profile", 1)
	view.AddKey("user", user)
	view.AddKey("products", userproducts)
	// view.AddKey("comments", userComments)
	view.AddKey("currentUser", currentUser)
	view.AddKey("meta_url", config.Get("meta_url")+user.ShowURL())
	view.AddKey("meta_title", user.Name+" open-payment-host profile")
	view.AddKey("meta_desc", user.Summary)
	view.AddKey("meta_image", config.Get("meta_image"))
	view.AddKey("meta_keywords", config.Get("meta_keywords"))
	view.AddKey("meta_foot", config.Get("meta_desc"))

	if user.Subscription {
		if user.Plan == config.Get("subscription_plan_name") {
			view.AddKey("flair", config.Get("subscription_plan_name_subscriber_flair"))
		}
	}

	return view.Render()
}

// HandleShowName redirects a GET request of /u/username to the user show page
func HandleShowName(w http.ResponseWriter, r *http.Request) error {

	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Find the user by name
	q := users.Where("name ILIKE ?", params.Get("name")+"%")
	results, err := users.FindAll(q)
	if err != nil {
		return server.NotFoundError(err, "Error finding user")
	}

	// If valid query but no results
	if len(results) == 0 {
		return server.NotFoundError(err, "User not found")
	}

	// Redirect to user show page
	return server.Redirect(w, r, results[0].ShowURL())
}
