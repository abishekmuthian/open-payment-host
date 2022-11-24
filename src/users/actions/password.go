package useractions

import (
	"net/http"
	"strconv"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth/can"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/users"
)

// HandlePasswordResetChangeShow responds to GET /users/password/change
func HandlePasswordChangeShow(w http.ResponseWriter, r *http.Request) error {
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

	// Authorise update user
	err = can.Update(user, session.CurrentUser(w, r))
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Render the template
	view := view.NewRenderer(w, r)
	view.Template("users/views/password_change.html.got")
	view.AddKey("user", user)
	view.AddKey("error", params.Get("error"))
	return view.Render()
}

// HandlePasswordChange responds to  gets the new password, validates it and updates it in the db
func HandlePasswordChange(w http.ResponseWriter, r *http.Request) error {
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
	err = can.Update(user, session.CurrentUser(w, r))
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Get the password
	pass := params.Get("password")
	passConfirm := params.Get("password-confirm")

	// Check if the passwords match
	if pass != passConfirm {
		log.Error(log.V{"Password": "Password doesn't match"})
		return server.Redirect(w, r, "/users/"+strconv.FormatInt(user.ID, 10)+"/password/change/?error=passwords_dont_match")
	}

	// Password must be at least 8 characters
	if len(pass) < 8 {
		log.Error(log.V{"Password": "Password characters are less than 8"})
		return server.Redirect(w, r, "/users/"+strconv.FormatInt(user.ID, 10)+"/password/change/?error=low_passwords_characters")
	}

	// Check if the pass is same as default password
	err = auth.CheckPassword(pass, user.PasswordHash)
	if err == nil {
		return server.Redirect(w, r, "/users/"+strconv.FormatInt(user.ID, 10)+"/password/change/?error=no_default_password")
	}

	// Set the password hash from the password
	hash, err := auth.HashPassword(pass)
	if err != nil {
		return server.InternalError(err)
	}

	//Set the hashed password in the params
	params.SetString("password_hash", hash)

	// Validate the params, removing any we don't accept
	userParams := user.ValidateParams(params.Map(), users.AllowedParams())

	// Update in database
	err = user.Update(userParams)
	if err != nil {
		return server.InternalError(err)
	}
	//Logout the user
	return HandleLogout(w, r)
}
