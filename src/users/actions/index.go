package useractions

import (
	"net/http"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth/can"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/users"
)

// HandleIndex displays a list of users.
func HandleIndex(w http.ResponseWriter, r *http.Request) error {

	// Authorise list user
	err := can.List(users.New(), session.CurrentUser(w, r))
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Build a query
	q := users.Query()

	// Order by required order, or default to id asc
	switch params.Get("order") {

	case "1":
		q.Order("created desc")

	case "2":
		q.Order("updated desc")

	default:
		q.Order("points desc")
	}

	// Filter if requested
	filter := params.Get("filter")
	if len(filter) > 0 {
		q.Where("name ILIKE ?", filter)
	}

	// Fetch the users
	results, err := users.FindAll(q)
	if err != nil {
		return server.InternalError(err)
	}

	// Render the template
	view := view.NewRenderer(w, r)
	view.AddKey("filter", filter)
	view.AddKey("users", results)
	return view.Render()
}
