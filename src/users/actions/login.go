package useractions

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/users"
)

// HandleLoginShow shows the page at /users/login
func HandleLoginShow(w http.ResponseWriter, r *http.Request) error {

	// Check they're not logged in already.
	if !session.CurrentUser(w, r).Anon() {
		return server.Redirect(w, r, "/?warn=already_logged_in")
	}

	// Authorize
	currentUser := session.CurrentUser(w, r)

	params, err := mux.Params(r)
	if err != nil {
		return server.NotFoundError(err)
	}

	// Show the login page, with login failure warnings.
	view := view.NewRenderer(w, r)
	view.AddKey("login", true)
	view.AddKey("currentUser", currentUser)
	view.AddKey("error", params.Get("error"))
	view.AddKey("redirectURL", params.Get("redirecturl"))
	// Set Cloudflare turnstile site key
	view.AddKey("turnstile_site_key", config.Get("turnstile_site_key"))

	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())

	return view.Render()
}

// HandleLogin responds to POST /users/login
// by setting a cookie on the request with encrypted user data.
func HandleLogin(w http.ResponseWriter, r *http.Request) error {

	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Check they're not logged in already if so redirect.
	if !session.CurrentUser(w, r).Anon() {
		return server.Redirect(w, r, "/?warn=already_logged_in")
	}

	// Get the user details from the database
	params, err := mux.Params(r)
	if err != nil {
		return server.NotFoundError(err)
	}

	// Using turnstile to verify users
	if len(params.Get("cf-turnstile-response")) > 0 {
		if string(params.Get("cf-turnstile-response")) != "" {

			type turnstileResponse struct {
				Success      bool     `json:"success"`
				Challenge_ts string   `json:"challenge_ts"`
				Hostname     string   `json:"hostname"`
				ErrorCodes   []string `json:"error-codes"`
				Action       string   `json:"login"`
				Cdata        string   `json:"cdata"`
			}

			var remoteIP string
			var siteVerify turnstileResponse

			if config.Production() {
				// Get the IP from Cloudflare
				remoteIP = r.Header.Get("CF-Connecting-IP")

			} else {
				// Extract the IP from the address
				remoteIP = r.RemoteAddr
				forward := r.Header.Get("X-Forwarded-For")
				if len(forward) > 0 {
					remoteIP = forward
				}
			}

			postBody := url.Values{}
			postBody.Set("secret", config.Get("turnstile_secret_key"))
			postBody.Set("response", string(params.Get("cf-turnstile-response")))
			postBody.Set("remoteip", remoteIP)

			resp, err := http.Post("https://challenges.cloudflare.com/turnstile/v0/siteverify", "application/x-www-form-urlencoded", strings.NewReader(postBody.Encode()))
			if err != nil {
				log.Info(log.V{"Upload, An error occurred while sending the request to the siteverify": err})
				return server.InternalError(err)
			}
			defer resp.Body.Close()

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Error(log.V{"Upload, An error occurred while reading the response from the siteverify": err})
				return server.InternalError(err)
			}

			json.Unmarshal(body, &siteVerify)

			if !siteVerify.Success {
				// Security challenge failed
				log.Error(log.V{"Upload, Security challenge failed": siteVerify.ErrorCodes[0]})
				return server.Redirect(w, r, "/users/login?error=security_challenge_failed_login#login")
			}
		} else {
			log.Error(log.V{"Upload, Security challenge unable to process": "response not received from user"})
			return server.Redirect(w, r, "/users/login?error=security_challenge_not_completed_login#login")
		}
	} else {
		// Security challenge not completed
		return server.Redirect(w, r, "/users/login?error=security_challenge_not_completed_login#login")
	}

	// Fetch the first user by EMAIL or username
	email := params.Get("email")

	// Find the user with this email
	user, err := users.FindFirst("email=?", email)

	if err != nil {
		log.Info(log.V{"msg": "login failed", "email": email, "status": http.StatusNotFound})
		return server.Redirect(w, r, "/users/login?error=not_a_valid_login")
	}

	// Check password against the stored password
	err = auth.CheckPassword(params.Get("password"), user.PasswordHash)
	if err != nil {
		log.Info(log.V{"msg": "login failed", "error": err, "email": email, "user_id": user.ID, "status": http.StatusUnauthorized})
		return server.Redirect(w, r, "/users/login?error=not_a_valid_login")
	}

	// Now save the user details in a secure cookie,
	// so that we remember the next request
	session, err := auth.Session(w, r)
	if err != nil {
		log.Info(log.V{"msg": "login failed", "email": email, "user_id": user.ID, "status": http.StatusInternalServerError})
	}

	// Success, log it and set the cookie with user id
	session.Set(auth.SessionUserKey, fmt.Sprintf("%d", user.ID))
	session.Save(w)

	// Log action
	log.Info(log.V{"msg": "login", "user_email": user.Email, "user_name": user.Name, "user_id": user.ID})

	// Check if the default password wasn't changed
	err = auth.CheckPassword(config.Get("default_admin_password"), user.PasswordHash)
	if err == nil {
		return server.Redirect(w, r, "/users/"+strconv.FormatInt(user.ID, 10)+"/password/change")
	}

	// Redirect - ideally here we'd redirect to their original request path
	redirectURL := params.Get("redirectURL")
	if redirectURL == "" {
		redirectURL = "/"
	}

	return server.Redirect(w, r, redirectURL)
}
