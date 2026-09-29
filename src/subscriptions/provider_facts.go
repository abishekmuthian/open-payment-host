package subscriptions

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var errPaymentPending = errors.New("payment confirmation pending")

type razorpayPayment struct {
	ID             string `json:"id"`
	OrderID        string `json:"order_id"`
	InvoiceID      string `json:"invoice_id"`
	Amount         int64  `json:"amount"`
	Currency       string `json:"currency"`
	Status         string `json:"status"`
	Captured       bool   `json:"captured"`
	Email          string `json:"email"`
	AmountRefunded int64  `json:"amount_refunded"`
}

func fetchRazorpayPaymentFacts(orderID, paymentID string) (ProviderFacts, error) {
	var p razorpayPayment
	if err := providerJSON("razorpay", http.MethodGet, "/payments/"+url.PathEscape(paymentID), nil, &p); err != nil {
		return ProviderFacts{}, err
	}
	if p.ID != paymentID || p.OrderID != orderID || p.Status != "captured" || !p.Captured || p.AmountRefunded != 0 {
		return ProviderFacts{}, errors.New("invalid captured payment")
	}
	var o struct {
		ID         string `json:"id"`
		Receipt    string `json:"receipt"`
		Status     string `json:"status"`
		Amount     int64  `json:"amount"`
		AmountPaid int64  `json:"amount_paid"`
		Currency   string `json:"currency"`
	}
	if err := providerJSON("razorpay", http.MethodGet, "/orders/"+url.PathEscape(orderID), nil, &o); err != nil {
		return ProviderFacts{}, err
	}
	a, err := FindAttemptByProviderOrder("razorpay", orderID)
	if err != nil {
		return ProviderFacts{}, err
	}
	if o.ID != orderID || o.Receipt != a.Id || o.Status != "paid" || o.Amount != p.Amount || o.AmountPaid != p.Amount || o.Currency != p.Currency {
		return ProviderFacts{}, errors.New("order identity or amount mismatch")
	}
	return ProviderFacts{Gateway: "razorpay", OrderId: o.ID, PaymentId: p.ID, Amount: p.Amount, Currency: p.Currency, Paid: true, Email: p.Email}, nil
}
func fetchRazorpaySubscriptionFacts(subscriptionID, paymentID string) (ProviderFacts, error) {
	var sub struct {
		ID     string            `json:"id"`
		PlanID string            `json:"plan_id"`
		Status string            `json:"status"`
		Notes  map[string]string `json:"notes"`
	}
	if err := providerJSON("razorpay", http.MethodGet, "/subscriptions/"+url.PathEscape(subscriptionID), nil, &sub); err != nil {
		return ProviderFacts{}, err
	}
	a, err := FindAttemptByProviderSubscription("razorpay", subscriptionID)
	if err != nil {
		return ProviderFacts{}, err
	}
	if sub.ID != subscriptionID || sub.PlanID != a.PriceId || sub.Notes["attempt_id"] != a.Id {
		return ProviderFacts{}, errors.New("subscription identity mismatch")
	}
	if sub.Status != "active" && sub.Status != "completed" {
		return ProviderFacts{}, errPaymentPending
	}
	var p razorpayPayment
	if err := providerJSON("razorpay", http.MethodGet, "/payments/"+url.PathEscape(paymentID), nil, &p); err != nil {
		return ProviderFacts{}, err
	}
	if p.ID != paymentID || p.Status != "captured" || !p.Captured || p.AmountRefunded != 0 {
		return ProviderFacts{}, errPaymentPending
	}
	// A captured payment for another subscription must never satisfy this one.
	if p.InvoiceID == "" {
		return ProviderFacts{}, errors.New("missing subscription invoice")
	}
	var invoice struct {
		ID             string `json:"id"`
		SubscriptionID string `json:"subscription_id"`
		PaymentID      string `json:"payment_id"`
		OrderID        string `json:"order_id"`
		Status         string `json:"status"`
	}
	if err := providerJSON("razorpay", http.MethodGet, "/invoices/"+url.PathEscape(p.InvoiceID), nil, &invoice); err != nil {
		return ProviderFacts{}, err
	}
	if invoice.ID != p.InvoiceID || invoice.SubscriptionID != subscriptionID || invoice.PaymentID != paymentID || invoice.OrderID != p.OrderID || invoice.Status != "paid" {
		return ProviderFacts{}, errors.New("subscription payment invoice mismatch")
	}
	return ProviderFacts{Gateway: "razorpay", SubscriptionId: subscriptionID, PaymentId: p.ID, PriceId: sub.PlanID, Amount: p.Amount, Currency: p.Currency, Paid: true, Email: p.Email}, nil
}
func fetchPaypalOrderDetails(id string) (*PayPalOrderDetailsResult, error) {
	var order PayPalOrderDetailsResult
	err := providerJSON("paypal", http.MethodGet, "/v2/checkout/orders/"+url.PathEscape(id), nil, &order)
	return &order, err
}
func paypalOrderFacts(o *PayPalOrderDetailsResult, a *PaymentAttempt) (ProviderFacts, error) {
	if o == nil || a == nil || a.Gateway != "paypal" || a.Schedule != "onetime" || o.ID != a.ProviderOrderId || o.Status != "COMPLETED" || len(o.PurchaseUnits) != 1 {
		return ProviderFacts{}, errors.New("invalid completed order")
	}
	u := o.PurchaseUnits[0]
	if u.ReferenceID != a.Id || len(u.Items) != 1 || u.Items[0].Sku != strconv.FormatInt(a.ProductId, 10) || u.Items[0].Quantity != "1" || len(u.Payments.Captures) != 1 {
		return ProviderFacts{}, errors.New("order reference, SKU or quantity mismatch")
	}
	c := u.Payments.Captures[0]
	if c.Status != "COMPLETED" || c.ID == "" || !c.FinalCapture {
		return ProviderFacts{}, errPaymentPending
	}
	amount, err := majorValueToMinor(c.Amount.Value, c.Amount.CurrencyCode)
	if err != nil {
		return ProviderFacts{}, err
	}
	total, err := majorValueToMinor(u.Amount.Value, u.Amount.CurrencyCode)
	if err != nil {
		return ProviderFacts{}, err
	}
	if total != amount || !currencyEqual(u.Amount.CurrencyCode, c.Amount.CurrencyCode) {
		return ProviderFacts{}, errors.New("capture total mismatch")
	}
	return ProviderFacts{Gateway: "paypal", OrderId: o.ID, PaymentId: c.ID, Amount: amount, Currency: c.Amount.CurrencyCode, Paid: true, Email: o.Payer.EmailAddress, Name: o.Payer.Name.GivenName}, nil
}
func fetchPaypalSubscriptionFacts(a *PaymentAttempt) (ProviderFacts, error) {
	return fetchPaypalSubscriptionTransactionFacts(a, "")
}
func fetchPaypalSubscriptionTransactionFacts(a *PaymentAttempt, transactionID string) (ProviderFacts, error) {
	var sub struct {
		ID         string `json:"id"`
		PlanID     string `json:"plan_id"`
		CustomID   string `json:"custom_id"`
		Status     string `json:"status"`
		Subscriber struct {
			Email string `json:"email_address"`
		} `json:"subscriber"`
	}
	if err := providerJSON("paypal", http.MethodGet, "/v1/billing/subscriptions/"+url.PathEscape(a.ProviderSubscriptionId), nil, &sub); err != nil {
		return ProviderFacts{}, err
	}
	if sub.ID != a.ProviderSubscriptionId || sub.PlanID != a.PriceId || sub.CustomID != a.Id {
		return ProviderFacts{}, errors.New("subscription identity mismatch")
	}
	if sub.Status != "ACTIVE" {
		return ProviderFacts{}, errPaymentPending
	}
	start := a.CreatedAt.UTC()
	// Renewal lookups stay within PayPal's transaction date-window limit.
	// The exact transaction ID is still mandatory for renewal fulfillment.
	if transactionID != "" && start.Before(time.Now().UTC().AddDate(0, 0, -30)) {
		start = time.Now().UTC().AddDate(0, 0, -30)
	}
	q := url.Values{"start_time": {start.Format(time.RFC3339)}, "end_time": {time.Now().UTC().Format(time.RFC3339)}}
	var payments PaypalSubscriptionTransaction
	if err := providerJSON("paypal", http.MethodGet, "/v1/billing/subscriptions/"+url.PathEscape(sub.ID)+"/transactions?"+q.Encode(), nil, &payments); err != nil {
		return ProviderFacts{}, err
	}
	// For initial fulfillment use the earliest completed transaction created
	// after this server attempt. Renewal webhooks select their exact transaction.
	index := -1
	for i, t := range payments.Transactions {
		if t.ID == "" || t.Status != "COMPLETED" || t.Time.Before(a.CreatedAt.Add(-time.Second)) {
			continue
		}
		if transactionID != "" && t.ID != transactionID {
			continue
		}
		if index < 0 || t.Time.Before(payments.Transactions[index].Time) {
			index = i
		}
	}
	if index < 0 {
		return ProviderFacts{}, errPaymentPending
	}
	t := payments.Transactions[index]
	amount, err := majorValueToMinor(t.AmountWithBreakdown.GrossAmount.Value, t.AmountWithBreakdown.GrossAmount.CurrencyCode)
	if err != nil {
		return ProviderFacts{}, err
	}
	return ProviderFacts{Gateway: "paypal", SubscriptionId: sub.ID, PaymentId: t.ID, PriceId: sub.PlanID, Amount: amount, Currency: t.AmountWithBreakdown.GrossAmount.CurrencyCode, Paid: true, Email: sub.Subscriber.Email}, nil
}
