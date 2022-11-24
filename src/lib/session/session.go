package session

import (
	"net/http"
	"strconv"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/users"
)

// CurrentUser returns the saved user (or an empty anon user)
// for the current session cookie
func CurrentUser(w http.ResponseWriter, r *http.Request) *users.User {

	// Start with an anon user by default (role 0, id 0)
	user := &users.User{}

	// Build the session from the secure cookie, or create a new one
	session, err := auth.Session(w, r)
	if err != nil {
		//log.Info(log.V{"msg": "session error", "error": err, "status": http.StatusInternalServerError})
		return user
	}

	// Fetch the current user record if we have one recorded in the session
	var id int64
	val := session.Get(auth.SessionUserKey)

	// If we have no value, we have no login
	if len(val) == 0 {
		//log.Info(log.V{"msg": "session error", "session": session, "status": http.StatusInternalServerError})
		return user
	}

	if len(val) > 0 {
		id, err = strconv.ParseInt(val, 10, 64)
		if err != nil {
			log.Info(log.V{"msg": "session error decoding", "val": val, "error": err, "status": http.StatusInternalServerError})
			return user
		}
	}

	if id > 0 {
		user, err = users.Find(id)
		if err != nil {
			log.Info(log.V{"msg": "session error user not found", "user_id": id, "error": err, "status": http.StatusNotFound})
			return user
		}
	}

	return user
}

// clearSession clears the request session cookie entirely.
// If an error is encountered in processing params, the session is cleared.
func clearSession(w http.ResponseWriter, r *http.Request) error {
	// Clear the session
	session, err := auth.SessionGet(r)
	if err != nil {
		return err
	}
	session.Clear(w)
	return nil
}

// CheckAuthenticity checks the authenticity token in params against cookie -
// The masked token is inserted into forms and POSTS by js.
// The token is inserted into the cookie by the middleware above.
// This is a shortcut for where you don't need params otherwise.
func CheckAuthenticity(w http.ResponseWriter, r *http.Request) error {

	// We should never be called on GET requests
	if r.Method == http.MethodGet {
		return nil
	}

	// Get the token from params and compare against cookie
	params, err := mux.Params(r)
	if err != nil {
		clearSession(w, r)
		return err
	}

	//	log.Info(log.V{"PARAMS": params})

	// Get the token from params (it is inserted there by js)
	// we do this to allow just one token in the head of every page
	token := params.Get(auth.SessionTokenKey)

	// Compare that param against the token stored in the session cookie.
	err = auth.CheckAuthenticityToken(token, r)
	if err != nil {
		clearSession(w, r)
		return err
	}

	return nil
}

// checkAuthenticityToken checks for token authenticity directly without params
func CheckAuthenticityToken(w http.ResponseWriter, r *http.Request, token string) error {
	// Compare that param against the token stored in the session cookie.
	err := auth.CheckAuthenticityToken(token, r)
	if err != nil {
		clearSession(w, r)
		return err
	}

	return nil
}
