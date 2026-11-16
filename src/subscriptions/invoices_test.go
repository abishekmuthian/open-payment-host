package subscriptions

import (
	"encoding/json"
	"fmt"
	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/invoices"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func setupGST(t *testing.T) {
	t.Helper()
	setupPayments(t)
	s := invoices.Settings{Name: "Seller", Address: "Coimbatore", GSTIN: "33AAAAA0000A1Z9", State: "33", Email: "seller@example.test", Signatory: "Seller", Prefix: "OPH", CreditPrefix: "CN", Start: 1}
	testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", invoices.JSON(s))
	testenv.Exec(t, "UPDATE products SET gst_enabled=1,gst_inclusive=0,gst_rate='18',gst_code='998439' WHERE id=1")
}
func gstAttempt(t *testing.T, gateway, schedule string) (*PaymentAttempt, ProviderFacts) {
	t.Helper()
	testenv.Exec(t, "UPDATE products SET schedule=? WHERE id=1", schedule)
	a, err := newAttempt(1, gateway, schedule, "IN", 11800, "INR", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := products.Find(1)
	if err = setInvoiceBase(a, p, 10000, "IN"); err != nil {
		t.Fatal(err)
	}
	b := invoices.Billing{Name: "Buyer", Email: "buyer@example.test", Address: "1 Main Street", City: "Chennai", State: "TN", Country: "IN", PostalCode: "600001"}
	if err = invoices.SnapshotBilling(a.Id, b); err != nil {
		t.Fatal(err)
	}
	f := ProviderFacts{Gateway: gateway, Amount: 11800, Currency: "INR", PaymentId: "pay_" + a.Id, Paid: true, PaidAt: time.Now()}
	if schedule == "onetime" {
		f.OrderId = "order_" + a.Id
	} else {
		f.SubscriptionId = "sub_" + a.Id
	}
	if err = a.SetProviderIds(f.OrderId, "", f.SubscriptionId); err != nil {
		t.Fatal(err)
	}
	return a, f
}
func TestGSTInvoicesAllGatewaysSchedulesRenewalsAndRefunds(t *testing.T) {
	setupGST(t)
	testenv.Exec(t, "UPDATE products SET name='Digital service #newsletter #monthly' WHERE id=1")
	for _, gateway := range []string{"stripe", "square", "paypal", "razorpay"} {
		for _, schedule := range []string{"onetime", "monthly", "yearly"} {
			t.Run(gateway+"/"+schedule, func(t *testing.T) {
				a, f := gstAttempt(t, gateway, schedule)
				var context invoices.Context
				if err := json.Unmarshal([]byte(a.InvoiceContext), &context); err != nil {
					t.Fatal(err)
				}
				if context.Description != "Digital service" {
					t.Fatalf("invoice description includes product hashtags: %q", context.Description)
				}
				if _, err := verifyAndFulfill(a, f); err != nil {
					t.Fatal(err)
				}
				if _, err := verifyAndFulfill(a, f); err != nil {
					t.Fatal(err)
				}
				if n := testenv.Scalar(t, "SELECT COUNT(*) FROM invoices WHERE attempt_id=?", a.Id); n != 1 {
					t.Fatalf("initial duplicate invoices %d", n)
				}
				if schedule != "onetime" {
					f.PaymentId = "renewal_" + a.Id
					if _, err := verifyAndFulfill(a, f); err != nil {
						t.Fatal(err)
					}
					if n := testenv.Scalar(t, "SELECT COUNT(*) FROM invoices WHERE attempt_id=?", a.Id); n != 2 {
						t.Fatalf("missing renewal %d", n)
					}
				}
				if err := applyRefund(gateway, f.PaymentId, "refund_"+a.Id, 5000, "INR"); err != nil {
					t.Fatal(err)
				}
				if err := applyRefund(gateway, f.PaymentId, "refund_"+a.Id, 5000, "INR"); err != nil {
					t.Fatal(err)
				}
				if n := testenv.Scalar(t, "SELECT COUNT(*) FROM invoices WHERE kind='credit_note' AND attempt_id=?", a.Id); n != 1 {
					t.Fatalf("duplicate credit %d", n)
				}
			})
		}
	}
}
func TestGSTInvoiceRollbackAndUnauthorizedDownload(t *testing.T) {
	setupGST(t)
	a, f := gstAttempt(t, "razorpay", "onetime")
	testenv.Exec(t, "DROP TABLE invoices")
	if _, err := verifyAndFulfill(a, f); err == nil {
		t.Fatal("missing atomic failure")
	}
	if testenv.Scalar(t, "SELECT COUNT(*) FROM payment_transactions") != 0 || testenv.Scalar(t, "SELECT total_onetime_payments FROM products WHERE id=1") != 0 {
		t.Fatal("partial payment committed")
	}
	testenv.Route("GET", "/subscriptions/invoice", HandleInvoiceDownload)
	r := httptest.NewRequest(http.MethodGet, "/subscriptions/invoice?attempt_id="+a.Id, nil)
	w := testenv.Serve(t, r)
	if w.Code != 401 {
		t.Fatal("download authorized by UUID")
	}
}
func TestGSTBillingRequiresBrowserCapabilityAndFreezes(t *testing.T) {
	setupGST(t)
	testenv.Auth(t)
	a, err := newAttempt(1, "razorpay", "onetime", "IN", 11800, "INR", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	product, _ := products.Find(1)
	if err = setInvoiceBase(a, product, 10000, "IN"); err != nil {
		t.Fatal(err)
	}
	testenv.Route("POST", "/subscriptions/invoice-billing", HandleInvoiceBilling)
	values := url.Values{"attempt_id": {a.Id}, "billing_name": {"Buyer"}, "billing_email": {"buyer@example.test"}, "billing_address": {"1 Main Street"}, "billing_city": {"Chennai"}, "billing_state": {"TN"}, "billing_country": {"IN"}, "billing_postcode": {"600001"}}
	r := testenv.AuthedPOST(t, "/subscriptions/invoice-billing", values, 0)
	w := testenv.Serve(t, r)
	if w.Code != 401 {
		t.Fatal("UUID accepted without capability")
	}
	r = testenv.AuthedPOST(t, "/subscriptions/invoice-billing", values, 0)
	cw := httptest.NewRecorder()
	setAttemptCookie(cw, r, a)
	for _, cookie := range cw.Result().Cookies() {
		r.AddCookie(cookie)
	}
	w = testenv.Serve(t, r)
	if w.Code != 200 {
		t.Fatalf("billing save %d %s", w.Code, w.Body.String())
	}
	b := invoices.Billing{Name: "Other", Email: "other@example.test", Address: "2 Street", City: "Chennai", Country: "IN", State: "TN", PostalCode: "600001"}
	if err = invoices.SnapshotBilling(a.Id, b); err == nil {
		t.Fatal("frozen billing overwritten")
	}
	if testenv.Text(t, "SELECT invoice_billing FROM payment_attempts WHERE id=?", a.Id) == "" {
		t.Fatal("billing not stored")
	}
}

func TestGSTOneTimeCheckoutChargesAllGateways(t *testing.T) {
	for _, gateway := range []string{"stripe", "square", "paypal", "razorpay"} {
		for _, inclusive := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/inclusive=%t", gateway, inclusive), func(t *testing.T) {
				setupHandlers(t)
				testenv.Set("subscription_client_country", "IN")
				testenv.Set("stripe_tax_rate_IN", "txr_gst")
				settings := invoices.Settings{Name: "Seller", Address: "Coimbatore", GSTIN: "33AAAAA0000A1Z9", State: "33", Email: "seller@example.test", Signatory: "Seller", Prefix: "OPH", CreditPrefix: "CN", Start: 1}
				testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", invoices.JSON(settings))
				testenv.Exec(t, `UPDATE products SET gst_enabled=1,gst_inclusive=?,gst_rate='18',gst_code='998439',stripe_price='{"IN":"price_gst"}',square_price='{"IN":{"amount":10000,"currency":"INR"}}',paypal_price='{"IN":{"amount":100,"currency":"INR","tax":37}}',razorpay_price='{"IN":{"amount":100,"currency":"INR"}}' WHERE id=1`, inclusive)
				gross := int64(11800)
				if inclusive {
					gross = 10000
				}
				var payload map[string]interface{}
				mockPayments(t, func(r *http.Request) string {
					if strings.Contains(r.URL.Path, "oauth2") {
						return `{"access_token":"test"}`
					}
					if r.Method == http.MethodPost {
						if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
							t.Fatal(err)
						}
					}
					switch {
					case r.URL.Path == "/v1/orders":
						return `{"id":"order_gst"}`
					case r.URL.Path == "/v2/checkout/orders":
						return `{"id":"ORDERGST"}`
					case r.URL.Path == "/v2/payments":
						return fmt.Sprintf(`{"payment":{"id":"square_gst","reference_id":%q,"status":"COMPLETED","amount_money":{"amount":%d,"currency":"INR"}}}`, payload["reference_id"], gross)
					default:
						t.Fatalf("unexpected provider request %s", r.URL.Path)
					}
					return ""
				})
				billing := invoices.Billing{Name: "Buyer", Email: "buyer@example.test", Address: "1 Main Street", City: "Chennai", State: "TN", Country: "IN", PostalCode: "600001"}
				switch gateway {
				case "razorpay":
					w := get(t, "/subscriptions/razorpay?product_id=1")
					if w.Code != 200 || payload["amount"] != float64(gross) {
						t.Fatalf("Razorpay charge %d %+v", w.Code, payload)
					}
				case "paypal":
					w := get(t, "/subscriptions/paypal?product_id=1")
					if w.Code != 200 {
						t.Fatal(w.Code, w.Body.String())
					}
					cookie := cookieNamed(w, AttemptCookie)
					id := strings.Split(cookie.Value, ".")[0]
					if err := invoices.SnapshotBilling(id, billing); err != nil {
						t.Fatal(err)
					}
					w = post(t, "/subscriptions/paypal/orders", nil, cookie)
					if w.Code != 200 {
						t.Fatal(w.Code, w.Body.String())
					}
					unit := payload["purchase_units"].([]interface{})[0].(map[string]interface{})
					amount := unit["amount"].(map[string]interface{})
					if amount["value"] != minorDecimal(gross, "INR") {
						t.Fatalf("PayPal total %v", amount)
					}
					breakdown := amount["breakdown"].(map[string]interface{})
					net := int64(10000)
					if inclusive {
						net = 8475
					}
					if breakdown["tax_total"].(map[string]interface{})["value"] != minorDecimal(gross-net, "INR") {
						t.Fatal("PayPal doubled/omitted GST", breakdown)
					}
				case "square":
					r := httptest.NewRequest(http.MethodGet, "/subscriptions/billing", nil)
					cw := httptest.NewRecorder()
					ref, err := storeBillingDraft(cw, r, billing)
					if err != nil {
						t.Fatal(err)
					}
					w := post(t, "/subscriptions/square", url.Values{"productId": {"1"}, "billing_ref": {ref}, "paymentToken": {"card-token"}, "verificationToken": {"verification-token"}}, cookieNamed(cw, "oph_billing"))
					if w.Code != 302 {
						t.Fatal(w.Code, w.Body.String())
					}
					if payload["amount_money"].(map[string]interface{})["amount"] != float64(gross) {
						t.Fatal("Square incorrect gross", payload)
					}
				case "stripe":
					testenv.StripeBackend(t, func(r *http.Request) (*http.Response, error) {
						switch r.URL.Path {
						case "/v1/prices/price_gst":
							return testenv.Response(200, `{"id":"price_gst","active":true,"type":"one_time","unit_amount":10000,"currency":"inr"}`), nil
						case "/v1/tax_rates/txr_gst":
							return testenv.Response(200, fmt.Sprintf(`{"id":"txr_gst","active":true,"inclusive":%t,"percentage":18}`, inclusive)), nil
						case "/v1/checkout/sessions":
							if err := r.ParseForm(); err != nil {
								t.Fatal(err)
							}
							if r.PostForm.Get("line_items[0][tax_rates][0]") != "txr_gst" {
								t.Fatal("missing native Stripe tax", r.PostForm)
							}
							return testenv.Response(200, `{"id":"cs_gst","url":"https://stripe.test/pay"}`), nil
						}
						return nil, fmt.Errorf("unexpected Stripe request %s", r.URL.Path)
					})
					w := post(t, "/subscriptions/create-checkout-session", url.Values{"productId": {"1"}})
					if w.Code != 303 {
						t.Fatal(w.Code, w.Body.String())
					}
				}
				if got := testenv.Scalar(t, "SELECT amount FROM payment_attempts ORDER BY created_at DESC LIMIT 1"); got != gross {
					t.Fatalf("expected attempt %d got %d", gross, got)
				}
			})
		}
	}
}

func TestGSTSquareRecurringFreezesBaseBeforeBilling(t *testing.T) {
	for _, schedule := range []string{"monthly", "yearly"} {
		t.Run(schedule, func(t *testing.T) {
			setupHandlers(t)
			testenv.Set("subscription_client_country", "IN")
			settings := invoices.Settings{Name: "Seller", Address: "Coimbatore", GSTIN: "33AAAAA0000A1Z9", State: "33", Email: "seller@example.test", Signatory: "Seller", Prefix: "OPH", CreditPrefix: "CN", Start: 1}
			testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", invoices.JSON(settings))
			testenv.Exec(t, `UPDATE products SET schedule=?,gst_enabled=1,gst_inclusive=0,gst_rate='18',gst_code='998439',square_price='{"IN":{"amount":10000,"currency":"INR"}}',square_subscription_plan_id='{"IN":"CATGST"}' WHERE id=1`, schedule)
			cadence := "MONTHLY"
			if schedule == "yearly" {
				cadence = "ANNUAL"
			}
			mockPayments(t, func(r *http.Request) string {
				switch {
				case strings.HasSuffix(r.URL.Path, "/catalog/object/CATGST"):
					return fmt.Sprintf(`{"object":{"id":"CATGST","subscription_plan_data":{"phases":[{"cadence":%q,"recurring_price_money":{"amount":11800,"currency":"INR"}}]}}}`, cadence)
				case strings.HasSuffix(r.URL.Path, "/customers"):
					return `{"customer":{"id":"CUSTOMER"}}`
				case strings.HasSuffix(r.URL.Path, "/cards"):
					var payload struct {
						Card struct {
							BillingAddress struct {
								State string `json:"administrative_district_level_1"`
							} `json:"billing_address"`
						} `json:"card"`
					}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Fatal(err)
					}
					if payload.Card.BillingAddress.State != "TN" {
						t.Fatal("numeric GST code sent to provider SDK", payload)
					}
					return `{"card":{"id":"CARD"}}`
				case strings.HasSuffix(r.URL.Path, "/subscriptions"):
					return `{"subscription":{"id":"SUBGST","status":"ACTIVE"}}`
				}
				t.Fatalf("unexpected %s", r.URL.Path)
				return ""
			})
			b := invoices.Billing{Name: "Buyer", Email: "buyer@example.test", Address: "1 Main Street", City: "Chennai", State: "TN", Country: "IN", PostalCode: "600001"}
			cw := httptest.NewRecorder()
			ref, err := storeBillingDraft(cw, httptest.NewRequest("GET", "/subscriptions/billing", nil), b)
			if err != nil {
				t.Fatal(err)
			}
			w := post(t, "/subscriptions/subscribe", url.Values{"productId": {"1"}, "billing_ref": {ref}, "paymentToken": {"token"}, "verificationToken": {"verification"}}, cookieNamed(cw, "oph_billing"))
			if w.Code != 302 {
				t.Fatal(w.Code, w.Body.String())
			}
			if testenv.Scalar(t, "SELECT amount FROM payment_attempts") != 11800 || testenv.Text(t, "SELECT invoice_billing FROM payment_attempts") == "" {
				t.Fatal("gross/billing not frozen")
			}
			if testenv.Scalar(t, "SELECT COUNT(*) FROM payment_transactions") != 0 || testenv.Scalar(t, "SELECT COUNT(*) FROM invoices") != 0 {
				t.Fatal("ACTIVE subscription authorized invoice before payment")
			}
		})
	}
}
