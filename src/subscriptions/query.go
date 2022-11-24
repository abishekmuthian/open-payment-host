package subscriptions

import (
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/resource"
	"time"
)

const (
	// TableName is the database table for this resource
	TableName = "subscriptions"
	// KeyName is the primary key value for this resource
	KeyName = "id"
	// Order defines the default sort order in sql for this resource
	Order = "id desc"
)

// AllowedParams returns an array of acceptable params in update
func AllowedParams() []string {
	return []string{"txn_id", "txn_type", "transaction_subject", "business", "custom", "invoice", "receipt_ID",
		"first_name", "handling_amount", "item_number", "item_name", "last_name", "mc_currency",
		"mc_fee", "mc_gross", "payer_email", "payer_id", "payer_status", "payment_date", "payment_fee",
		"payment_gross", "payment_status", "payment_type", "protection_eligibility", "quantity",
		"receiver_id", "receiver_email", "residence_country", "shipping", "tax", "address_country",
		"test_ipn", "address_status", "address_street", "notify_version", "address_city", "verify_sign",
		"address_state", "charset", "address_name", "address_country_code", "address_zip", "subscr_id", "user_id"}
}

// NewWithColumns creates a new subscription instance and fills with data from the database cols provided.
func NewWithColumns(cols map[string]interface{}) *Subscription {
	subscription := New()
	subscription.ID = resource.ValidateInt(cols["id"])
	subscription.CreatedAt = resource.ValidateTime(cols["created_at"])
	subscription.UpdatedAt = resource.ValidateTime(cols["updated_at"])
	subscription.Created = resource.ValidateTime(cols["payment_date"])
	subscription.Amount = resource.ValidateFloat(cols["payment_gross"])
	subscription.Currency = resource.ValidateString(cols["mc_currency"])
	subscription.CustomerId = resource.ValidateString(cols["payer_id"])
	subscription.CustomerEmail = resource.ValidateString(cols["payer_email"])
	subscription.SubscriptionId = resource.ValidateString(cols["subscr_id"])
	subscription.UserId = resource.ValidateInt(cols["user_id"])
	subscription.Plan = resource.ValidateString(cols["transaction_subject"])

	return subscription
}

// New creates and initialises a new subscriptions instance.
func New() *Subscription {
	subscription := &Subscription{}
	subscription.CreatedAt = time.Now()
	subscription.UpdatedAt = time.Now()
	subscription.TableName = TableName
	subscription.KeyName = KeyName
	return subscription
}

// FindFirst fetches a single user record from the database using
// a where query with the format and args provided.
func FindFirst(format string, args ...interface{}) (*Subscription, error) {
	result, err := Query().Where(format, args...).FirstResult()
	if err != nil {
		return nil, err
	}
	return NewWithColumns(result), nil
}

// Find fetches a single subscription record from the database by id.
func Find(id string) (*Subscription, error) {
	result, err := Query().Where("subscr_id=?", id).FirstResult()
	if err != nil {
		return nil, err
	}
	return NewWithColumns(result), nil
}

// Find fetches a single subscription record from the database by user id.
func FindCustomerId(userId int64) (*Subscription, error) {
	q := Query().Limit(1)
	q.Where("user_id=?", userId)
	result, err := FindAll(q)
	if result == nil || err != nil {
		return nil, err
	}
	return result[0], nil
}

// FindAll fetches all subscription records matching this query from the database.
func FindAll(q *query.Query) ([]*Subscription, error) {

	// Fetch query.Results from query
	results, err := q.Results()
	if err != nil {
		return nil, err
	}

	// Return an array of users constructed from the results
	var subscriptions []*Subscription
	for _, cols := range results {
		p := NewWithColumns(cols)
		subscriptions = append(subscriptions, p)
	}

	return subscriptions, nil
}

// Query returns a new query for subscriptions with a default order.
func Query() *query.Query {
	return query.New(TableName, KeyName).Order(Order)
}

// Where returns a new query for subscriptions with the format and arguments supplied.
func Where(format string, args ...interface{}) *query.Query {
	return Query().Where(format, args...)
}
