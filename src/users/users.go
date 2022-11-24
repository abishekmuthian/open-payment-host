// Package users represents the user resource
package users

import (
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/resource"
	"github.com/abishekmuthian/open-payment-host/src/lib/status"
)

// User handles saving and retrieving users from the database
type User struct {
	// resource.Base defines behaviour and fields shared between all resources
	resource.Base

	// status.ResourceStatus defines a status field and associated behaviour
	status.ResourceStatus

	Email        string
	Name         string
	Points       int64
	Role         int64
	Summary      string
	Text         string
	Title        string
	Notification bool
	Subscription bool
	Plan         string

	PasswordHash    string
	PasswordResetAt time.Time
}
