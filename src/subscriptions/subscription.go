// package subscriptions refers to subscriptions resource
package subscriptions

import (
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/resource"
	"github.com/abishekmuthian/open-payment-host/src/lib/status"
)

// Subscription handles saving and retrieving users from the database
type Subscription struct {
	// resource.Base defines behaviour and fields shared between all resources
	resource.Base

	// status.ResourceStatus defines a status field and associated behaviour
	status.ResourceStatus

	Id             string
	Created        time.Time
	Amount         float64
	Currency       string
	CustomerId     string
	CustomerEmail  string
	SubscriptionId string
	UserId         int64
	Plan           string
	ProductId      int64
	PaymentStaus   string
	PaymentGateway string
	FirstName      string
}
