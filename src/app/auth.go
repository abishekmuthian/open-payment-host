package app

import (
	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
	"github.com/abishekmuthian/open-payment-host/src/lib/auth/can"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/abishekmuthian/open-payment-host/src/subscriptions"
	"github.com/abishekmuthian/open-payment-host/src/users"
)

// SetupAuth sets up the auth pkg and authorisation for users
func SetupAuth() {

	// Set up the auth package with our secrets from config
	auth.HMACKey = auth.HexToBytes(config.Get("hmac_key"))
	auth.SecretKey = auth.HexToBytes(config.Get("secret_key"))
	auth.SessionName = config.Get("session_name")

	// Enable https cookies on production server - everyone should be on https
	if config.Production() {
		auth.SecureCookies = true
	}

	// Set up our authorisation for user roles on resources using can pkg

	// Admins are allowed to manage all resources
	can.Authorise(users.Admin, can.ManageResource, can.Anything)

	// Readers may edit their user
	can.AuthoriseOwner(users.Reader, can.UpdateResource, users.TableName)

	// Readers may add comments and edit their own comments
	// No comments implementation yet
	/* 	can.Authorise(users.Reader, can.CreateResource, comments.TableName)
	   	can.AuthoriseOwner(users.Reader, can.UpdateResource, comments.TableName) */

	// Readers may add products and edit their own products (up to time limit)
	can.Authorise(users.Reader, can.CreateResource, products.TableName)
	can.AuthoriseOwner(users.Reader, can.UpdateResource, products.TableName)

	// Readers may add subscriptions and edit their own subscriptions
	can.Authorise(users.Reader, can.CreateResource, subscriptions.TableName)
	can.AuthoriseOwner(users.Reader, can.UpdateResource, subscriptions.TableName)

	// Anon may create users
	can.AuthoriseOwner(users.Anon, can.CreateResource, users.TableName)

}
