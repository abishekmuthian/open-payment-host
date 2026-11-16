package subscriptions

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/invoices"
)

func razorpayBillingPage(t *testing.T, country, schedule string, gst bool) string {
	t.Helper()
	setupHandlers(t)
	testenv.Set("subscription_client_country", country)
	seller := invoices.Settings{Name: "Example Seller", Address: "Coimbatore", GSTIN: "33AAAAA0000A1Z9", State: "33", Email: "seller@example.test", Signatory: "Example Seller", Prefix: "OPH", CreditPrefix: "CN", Start: 1, LUT: "TEST-LUT", LUTYear: invoices.FinancialYear(time.Now())}
	testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", invoices.JSON(seller))
	testenv.Exec(t, `UPDATE products SET name='Digital service #newsletter #monthly',schedule=?,gst_enabled=?,gst_inclusive=1,gst_export=1,gst_rate='18',gst_code='998439',razorpay_price='{"DF":{"amount":100,"currency":"INR","plan_id":"plan_form"}}' WHERE id=1`, schedule, gst)
	mockPayments(t, func(r *http.Request) string {
		switch r.URL.Path {
		case "/v1/orders":
			return `{"id":"order_form"}`
		case "/v1/subscriptions":
			return `{"id":"sub_form"}`
		case "/v1/plans/plan_form":
			return fmt.Sprintf(`{"id":"plan_form","period":%q,"interval":1,"item":{"amount":10000,"currency":"INR"}}`, schedule)
		default:
			t.Fatalf("unexpected provider request: %s", r.URL.Path)
			return ""
		}
	})
	w := get(t, "/subscriptions/razorpay?product_id=1")
	if w.Code != 200 {
		t.Fatalf("billing page returned %d: %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func TestRazorpayCheckoutTitleExcludesHashtags(t *testing.T) {
	for _, schedule := range []string{"onetime", "monthly", "yearly"} {
		t.Run(schedule, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(razorpayBillingPage(t, "US", schedule, false)))
			if err != nil {
				t.Fatal(err)
			}
			if title := doc.Find("meta[name='product_title']").AttrOr("content", ""); title != "Digital service" {
				t.Fatalf("Razorpay Checkout receives product title %q", title)
			}
			if testenv.Text(t, "SELECT name FROM products WHERE id=1") != "Digital service #newsletter #monthly" {
				t.Fatal("checkout changed the stored product hashtags")
			}
		})
	}
}

func TestStripeOnlyRequestsGSTINForIndianCheckout(t *testing.T) {
	for _, country := range []string{"IN", "US"} {
		for _, schedule := range []string{"onetime", "monthly", "yearly"} {
			for _, gst := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/gst=%t", country, schedule, gst), func(t *testing.T) {
					setupHandlers(t)
					testenv.Set("subscription_client_country", country)
					testenv.Set("stripe_tax_rate_IN", "txr_form")
					seller := invoices.Settings{Name: "Example Seller", Address: "Coimbatore", GSTIN: "33AAAAA0000A1Z9", State: "33", Email: "seller@example.test", Signatory: "Example Seller", Prefix: "OPH", CreditPrefix: "CN", Start: 1, LUT: "TEST-LUT", LUTYear: invoices.FinancialYear(time.Now())}
					testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", invoices.JSON(seller))
					testenv.Exec(t, `UPDATE products SET schedule=?,gst_enabled=?,gst_inclusive=1,gst_export=1,gst_rate='18',gst_code='998439',stripe_price='{"DF":"price_form"}' WHERE id=1`, schedule, gst)
					seen := false
					testenv.StripeBackend(t, func(r *http.Request) (*http.Response, error) {
						switch r.URL.Path {
						case "/v1/prices/price_form":
							if schedule == "onetime" {
								return testenv.Response(200, `{"id":"price_form","active":true,"type":"one_time","unit_amount":10000,"currency":"inr"}`), nil
							}
							interval := "month"
							if schedule == "yearly" {
								interval = "year"
							}
							return testenv.Response(200, fmt.Sprintf(`{"id":"price_form","active":true,"type":"recurring","unit_amount":10000,"currency":"inr","recurring":{"interval":%q,"interval_count":1}}`, interval)), nil
						case "/v1/tax_rates/txr_form":
							return testenv.Response(200, `{"id":"txr_form","active":true,"inclusive":true,"percentage":18}`), nil
						case "/v1/checkout/sessions":
							if err := r.ParseForm(); err != nil {
								t.Fatal(err)
							}
							seen = true
							want := ""
							if gst && country == "IN" {
								want = "true"
							}
							if got := r.PostForm.Get("tax_id_collection[enabled]"); got != want {
								t.Fatalf("tax ID collection=%q, want %q", got, want)
							}
							return testenv.Response(200, `{"id":"cs_form","url":"https://stripe.test/pay"}`), nil
						default:
							return nil, fmt.Errorf("unexpected Stripe request %s", r.URL.Path)
						}
					})
					w := post(t, "/subscriptions/create-checkout-session", map[string][]string{"productId": {"1"}})
					if w.Code != 303 || !seen {
						t.Fatal("Stripe Checkout was not created", w.Code)
					}
				})
			}
		}
	}
}

func TestRazorpayBillingCountryUsesNamedOptions(t *testing.T) {
	for _, country := range []string{"IN", "US", "GB", "XX"} {
		for _, schedule := range []string{"onetime", "monthly", "yearly"} {
			for _, gst := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/gst=%t", country, schedule, gst), func(t *testing.T) {
					doc, err := goquery.NewDocumentFromReader(strings.NewReader(razorpayBillingPage(t, country, schedule, gst)))
					if err != nil {
						t.Fatal(err)
					}
					fields := doc.Find("#razorpay-button-container [name='billing_country']")
					if fields.Length() != 1 {
						t.Fatal("billing must have exactly one country value")
					}
					if country == "IN" {
						if fields.AttrOr("type", "") != "hidden" || fields.AttrOr("value", "") != "IN" {
							t.Fatal("Indian billing must set its country automatically")
						}
					} else {
						if goquery.NodeName(fields) != "select" {
							t.Fatal("international billing asks for a raw country code")
						}
						if _, required := fields.Attr("required"); !required {
							t.Fatal("international country selection is not required")
						}
						if fields.Find("option[value='US']").Text() != "United States" || fields.Find("option[value='GB']").Text() != "United Kingdom" {
							t.Fatal("country names are not mapped to ISO codes")
						}
						selected := fields.Find("option[selected]").AttrOr("value", "")
						want := country
						if country == "XX" {
							want = ""
						}
						if selected != want {
							t.Fatalf("detected country selection=%q, want %q", selected, want)
						}
					}
					if (doc.Find("[name='billing_business']").Length() == 1) != gst || (doc.Find("[name='billing_gstin']").Length() == 1) != gst {
						t.Fatal("optional GST fields do not follow product invoicing settings")
					}
					if gst {
						for _, name := range []string{"business", "gstin"} {
							field := doc.Find("[name='billing_" + name + "']")
							_, disabled := field.Attr("disabled")
							if disabled != (country != "IN") || (field.ParentsFiltered("[hidden]").Length() > 0) != (country != "IN") {
								t.Fatal("business name and GSTIN must be hidden and disabled outside India")
							}
						}
					}
				})
			}
		}
	}
}
