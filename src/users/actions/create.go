package useractions

import (
	"fmt"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/status"
	"github.com/abishekmuthian/open-payment-host/src/users"
)

// HandleCreateAdminUser creates the user
func HandleCreateAdminUser(email string, password string) error {

	user := users.New()

	// Set the password hash from the password
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	userParams := make(map[string]string)
	userParams["email"] = email
	userParams["password_hash"] = hash
	userParams["status"] = fmt.Sprintf("%d", status.Published)
	userParams["role"] = fmt.Sprintf("%d", users.Admin)

	id, err := user.Create(userParams)
	if err != nil {
		return err
	}

	// Redirect to the new user
	user, err = users.Find(id)
	if err != nil {
		return err
	}

	// Log action
	log.Info(log.V{"msg": "user creation success", "user_email": user.Email, "user_id": user.ID})

	return err
}
