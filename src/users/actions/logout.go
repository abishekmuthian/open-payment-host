package useractions

import (
	"net/http"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
)

// HandleLogout clears the current user's session /users/logout
func HandleLogout(w http.ResponseWriter, r *http.Request) error {

	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Clear the current session cookie
	auth.ClearSession(w)

	// Redirect to home
	return server.Redirect(w, r, "/")
}
