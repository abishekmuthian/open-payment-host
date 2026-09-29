package subscriptions

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/resource"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/google/uuid"
)

const (
	// AttemptTableName is the database table for payment attempts
	AttemptTableName = "payment_attempts"

	// Attempt statuses
	AttemptStatusPending   = "pending"
	AttemptStatusCompleted = "completed"
	AttemptStatusCancelled = "cancelled"

	// AttemptCookie holds the opaque attempt id binding a checkout to the browser that started it
	AttemptCookie = "oph_attempt"

	// CompletionCookie holds the random completion token exchanged for the download capability
	CompletionCookie = "oph_payment"

	// attemptExpiry is how long an attempt may remain unverified before it fails closed
	attemptExpiry = 24 * time.Hour

	// completionCookieExpiry is how long the download capability cookie remains valid
	completionCookieExpiry = 15 * time.Minute
)

// PaymentAttempt is an immutable record of what the server expects a payment to be.
// It is created only after loading the product and resolving its configured country
// price. Browser values may select a product and submit a tokenized payment source,
// but browser-supplied amount, currency, price id, plan id and success-page product id
// never authorize payment or fulfillment.
type PaymentAttempt struct {
	Id                          string
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
	ProductId                   int64
	Gateway                     string
	Schedule                    string
	Country                     string
	Amount                      int64
	Currency                    string
	PriceId                     string
	ProviderOrderId             string
	ProviderPaymentId           string
	ProviderSubscriptionId      string
	CustomId                    string
	RedirectURI                 string
	CompletionTokenHash         string
	CancellationTokenHash       string
	CancellationTokenCiphertext string
	BrowserTokenHash            string
	CompletionExpiresAt         int64
	CancellationUsed            bool
	Status                      string
	ExpiresAt                   time.Time
	CompletedAt                 time.Time
	CancelledAt                 time.Time

	// completionToken holds the plaintext token transiently at creation,
	// it is only stored hashed in the database.
	completionToken string
	browserToken    string
}

// attemptWithColumns fills an attempt from database columns.
func attemptWithColumns(cols map[string]interface{}) *PaymentAttempt {
	a := &PaymentAttempt{}
	a.Id = resource.ValidateString(cols["id"])
	a.CreatedAt = resource.ValidateTime(cols["created_at"])
	a.UpdatedAt = resource.ValidateTime(cols["updated_at"])
	a.ProductId = resource.ValidateInt(cols["product_id"])
	a.Gateway = resource.ValidateString(cols["gateway"])
	a.Schedule = resource.ValidateString(cols["schedule"])
	a.Country = resource.ValidateString(cols["country"])
	a.Amount = resource.ValidateInt(cols["amount"])
	a.Currency = resource.ValidateString(cols["currency"])
	a.PriceId = resource.ValidateString(cols["price_id"])
	a.ProviderOrderId = resource.ValidateString(cols["provider_order_id"])
	a.ProviderPaymentId = resource.ValidateString(cols["provider_payment_id"])
	a.ProviderSubscriptionId = resource.ValidateString(cols["provider_subscription_id"])
	a.CustomId = resource.ValidateString(cols["custom_id"])
	a.RedirectURI = resource.ValidateString(cols["redirect_uri"])
	a.CompletionTokenHash = resource.ValidateString(cols["completion_token_hash"])
	a.CancellationTokenHash = resource.ValidateString(cols["cancellation_token_hash"])
	a.CancellationTokenCiphertext = resource.ValidateString(cols["cancellation_token_ciphertext"])
	a.BrowserTokenHash = resource.ValidateString(cols["browser_token_hash"])
	a.CompletionExpiresAt = resource.ValidateInt(cols["completion_expires_at"])
	a.CancellationUsed = resource.ValidateInt(cols["cancellation_used"]) != 0
	a.Status = resource.ValidateString(cols["status"])
	a.ExpiresAt = resource.ValidateTime(cols["expires_at"])
	a.CompletedAt = resource.ValidateTime(cols["completed_at"])
	a.CancelledAt = resource.ValidateTime(cols["cancelled_at"])
	return a
}

// attemptQuery returns a new query for payment attempts.
func attemptQuery() *query.Query {
	return query.New(AttemptTableName, "id")
}

// Save rotates only the completion capability; immutable identity and state
// cannot be overwritten by a stale browser request racing a webhook.
func (a *PaymentAttempt) Save() error {
	a.CompletionExpiresAt = time.Now().Add(completionCookieExpiry).Unix()
	return query.Transaction(func(tx *query.Tx) error {
		_, err := tx.Exec("UPDATE payment_attempts SET completion_token_hash=?, completion_expires_at=? WHERE id=?", a.CompletionTokenHash, a.CompletionExpiresAt, a.Id)
		return err
	})
}

// FindAttempt fetches a single payment attempt by its opaque id.
func FindAttempt(id string) (*PaymentAttempt, error) {
	if id == "" {
		return nil, errors.New("empty attempt id")
	}
	cols, err := attemptQuery().Where("id=?", id).FirstResult()
	if err != nil {
		return nil, err
	}
	return attemptWithColumns(cols), nil
}

// FindAttemptByProviderOrder fetches an attempt by its stored provider order id.
func FindAttemptByProviderOrder(gateway string, orderId string) (*PaymentAttempt, error) {
	return findAttemptByProviderField(gateway, "provider_order_id", orderId)
}

// FindAttemptByProviderSubscription fetches an attempt by its stored provider subscription id.
func FindAttemptByProviderSubscription(gateway string, subscriptionId string) (*PaymentAttempt, error) {
	return findAttemptByProviderField(gateway, "provider_subscription_id", subscriptionId)
}

// FindAttemptByProviderPayment fetches an attempt by its stored provider payment id.
func FindAttemptByProviderPayment(gateway string, paymentId string) (*PaymentAttempt, error) {
	return findAttemptByProviderField(gateway, "provider_payment_id", paymentId)
}

func findAttemptByProviderField(gateway string, field string, value string) (*PaymentAttempt, error) {
	if value == "" {
		return nil, errors.New("empty provider id")
	}
	cols, err := attemptQuery().Where("gateway=? AND "+field+"=?", gateway, value).FirstResult()
	if err != nil {
		return nil, err
	}
	return attemptWithColumns(cols), nil
}

// Expired returns true if the attempt is too old to be verified.
func (a *PaymentAttempt) Expired() bool {
	return a.ExpiresAt.IsZero() || time.Now().UTC().After(a.ExpiresAt)
}

// CancellationTokenValid accepts only an unused capability for this subscription.
func (a *PaymentAttempt) CancellationTokenValid(token string) bool {
	return !a.CancellationUsed && a.ProviderSubscriptionId != "" && tokensMatch(token, a.CancellationTokenHash)
}

// SetProviderIds binds provider resources once; retries may repeat the same IDs.
func (a *PaymentAttempt) SetProviderIds(orderID, paymentID, subscriptionID string) error {
	err := query.Transaction(func(tx *query.Tx) error {
		for field, value := range map[string]string{"provider_order_id": orderID, "provider_payment_id": paymentID, "provider_subscription_id": subscriptionID} {
			if value == "" {
				continue
			}
			result, err := tx.Exec("UPDATE payment_attempts SET "+field+"=? WHERE id=? AND ("+field+"='' OR "+field+"=?)", value, a.Id, value)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return errors.New("provider resource is already bound")
			}
		}
		return nil
	})
	if err == nil {
		if orderID != "" {
			a.ProviderOrderId = orderID
		}
		if paymentID != "" {
			a.ProviderPaymentId = paymentID
		}
		if subscriptionID != "" {
			a.ProviderSubscriptionId = subscriptionID
		}
	}
	return err
}

// generateToken returns a random hex token of the given byte length.
func generateToken(byteLength int) (string, error) {
	b := make([]byte, byteLength)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashToken returns the hex sha256 of a token.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// tokensMatch compares a presented token with a stored hash in constant time.
func tokensMatch(token string, hash string) bool {
	if token == "" || hash == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hashToken(token)), []byte(hash)) == 1
}

// newAttempt creates a pending payment attempt for the given product and gateway
// with the server-resolved country price, frozen experimental API values and
// random completion and cancellation tokens.
func newAttempt(productId int64, gateway string, schedule string, country string, amount int64, currency string, priceId string, customId string, redirectURI string) (*PaymentAttempt, error) {
	if productId <= 0 || amount <= 0 || len(currency) != 3 || (schedule != "onetime" && schedule != "monthly" && schedule != "yearly") {
		return nil, errors.New("invalid payment configuration")
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	completionToken, err := generateToken(32)
	if err != nil {
		return nil, err
	}
	cancellationToken, err := generateToken(32)
	if err != nil {
		return nil, err
	}

	browserToken, err := generateToken(32)
	if err != nil {
		return nil, err
	}
	encrypted, err := sealCapability(cancellationToken)
	if err != nil {
		return nil, err
	}
	a := &PaymentAttempt{
		CreatedAt:                   time.Now().UTC(),
		BrowserTokenHash:            hashToken(browserToken),
		browserToken:                browserToken,
		Id:                          id.String(),
		Gateway:                     gateway,
		ProductId:                   productId,
		Schedule:                    schedule,
		Country:                     country,
		Amount:                      amount,
		Currency:                    currency,
		PriceId:                     priceId,
		CustomId:                    customId,
		RedirectURI:                 redirectURI,
		CompletionTokenHash:         hashToken(completionToken),
		CancellationTokenCiphertext: encrypted,
		CancellationTokenHash:       hashToken(cancellationToken),
		Status:                      AttemptStatusPending,
		ExpiresAt:                   time.Now().UTC().Add(attemptExpiry),
	}

	err = query.Transaction(func(tx *query.Tx) error {
		_, err := tx.Exec(`INSERT INTO payment_attempts
  (id,created_at,updated_at,product_id,gateway,schedule,country,amount,currency,price_id,provider_order_id,provider_payment_id,provider_subscription_id,custom_id,redirect_uri,completion_token_hash,cancellation_token_hash,cancellation_token_ciphertext,browser_token_hash,status,expires_at,completed_at,cancelled_at)
  VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, a.Id, query.TimeString(a.CreatedAt), query.TimeString(a.CreatedAt), a.ProductId, a.Gateway, a.Schedule, a.Country, a.Amount, a.Currency, a.PriceId, "", "", "", a.CustomId, a.RedirectURI, a.CompletionTokenHash, a.CancellationTokenHash, a.CancellationTokenCiphertext, a.BrowserTokenHash, a.Status, query.TimeString(a.ExpiresAt), "", "")
		return err
	})
	if err != nil {
		return nil, err
	}

	// Return with the plaintext completion token so the caller can set the cookie.
	a.completionToken = completionToken
	return a, nil
}

// setAttemptCookie binds the attempt to the browser which started checkout.
func setAttemptCookie(w http.ResponseWriter, r *http.Request, attempt *PaymentAttempt) {
	http.SetCookie(w, &http.Cookie{
		Name:     AttemptCookie,
		Value:    attempt.Id + "." + attempt.browserToken,
		Path:     "/subscriptions",
		MaxAge:   int(attemptExpiry.Seconds()),
		HttpOnly: true,
		Secure:   config.Production(),
		SameSite: http.SameSiteLaxMode,
	})
}

// attemptCookie returns the attempt id stored in the browser cookie.
func attemptCookie(r *http.Request) string {
	c, err := r.Cookie(AttemptCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// setCompletionCookie exchanges the random completion token for a short-lived
// Secure, HttpOnly, SameSite=Lax cookie, keeping the capability out of browser
// history and referrer headers while the download button can be retried.
func setCompletionCookie(w http.ResponseWriter, r *http.Request, completionToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CompletionCookie,
		Value:    completionToken,
		Path:     "/subscriptions",
		MaxAge:   int(completionCookieExpiry.Seconds()),
		HttpOnly: true,
		Secure:   config.Production(),
		SameSite: http.SameSiteLaxMode,
	})
}

// completionCookieValid validates the completion token cookie against the attempt.
func completionCookieValid(r *http.Request, attempt *PaymentAttempt) bool {
	c, err := r.Cookie(CompletionCookie)
	if err != nil {
		return false
	}
	return time.Now().Unix() < attempt.CompletionExpiresAt && tokensMatch(c.Value, attempt.CompletionTokenHash)
}

// bindAttemptToRequest verifies that the attempt belongs to the request by
// requiring the browser attempt cookie to match, so a replayed provider id
// from another browser cannot fulfill an unrelated attempt.
func bindAttemptToRequest(r *http.Request, attempt *PaymentAttempt) bool {
	if attempt == nil || attempt.Expired() {
		return false
	}
	parts := strings.Split(attemptCookie(r), ".")
	return len(parts) == 2 && parts[0] == attempt.Id && tokensMatch(parts[1], attempt.BrowserTokenHash)
}

// formatInt formats an int64 as a string.
func formatInt(i int64) string {
	return strconv.FormatInt(i, 10)
}

func (a *PaymentAttempt) cancellationToken() string {
	if a.ProviderSubscriptionId == "" || a.CancellationUsed {
		return ""
	}
	token, err := openCapability(a.CancellationTokenCiphertext)
	if err != nil {
		return ""
	}
	return token
}

// CancellationURL returns the signed cancellation URL for the attempt's subscription.
func (a *PaymentAttempt) CancellationURL() string {
	token := a.cancellationToken()
	if token == "" {
		return ""
	}
	return BuildRedirectURL(config.Get("root_url")+"/subscriptions/cancel", map[string]string{"subscription_id": a.ProviderSubscriptionId, "cancellation_token": token})
}

// RenderCancellationFailed renders the cancellation failure page with a
// fail-closed message when a cancellation request cannot be authorized.
func RenderCancellationFailed(w http.ResponseWriter, r *http.Request, message string) error {
	view := view.NewRenderer(w, r)
	view.AddKey("currentUser", session.CurrentUser(w, r))
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())
	view.AddKey("title", "Cancellation not allowed")
	view.AddKey("message", message)
	view.Template("subscriptions/views/verification.html.got")
	return view.Render()
}
