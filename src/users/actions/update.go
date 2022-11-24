package useractions

import (
	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/users"
)

// HandleUpdate updates the user
func HandleUpdate(id int64, email string, password string) error {

	// Find the user
	user, err := users.Find(id)
	if err != nil {
		return server.NotFoundError(err)
	}

	// Set the password hash from the password
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	userParams := make(map[string]string)
	userParams["email"] = email
	userParams["password_hash"] = hash

	err = user.Update(userParams)
	if err != nil {
		return err
	}

	// Redirect to the new user
	user, err = users.Find(id)
	if err != nil {
		return err
	}

	// Log action
	log.Info(log.V{"msg": "user update success", "user_email": user.Email, "user_id": user.ID})

	return err
}
