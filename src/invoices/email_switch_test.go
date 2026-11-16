package invoices

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/mail"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
)

func TestAutomatedInvoiceEmailIndependentOfEnvironment(t *testing.T) {
	for _, provider := range []string{"default", "listmonk"} {
		for _, production := range []bool{false, true} {
			for _, flag := range []string{"yes", "no"} {
				t.Run(fmt.Sprintf("%s/production=%t/flag=%q", provider, production, flag), func(t *testing.T) {
					sender := setup(t)
					if production {
						testenv.Production(t)
					}
					mail.Production = production
					testenv.Set("automated_invoice", flag)
					var calls atomic.Int64
					testenv.MockTransport(t, func(r *http.Request) (*http.Response, error) {
						if r.URL.String() != "https://listmonk.example.test/api/tx" {
							t.Errorf("unexpected HTTP request to %s", r.URL.Host)
						}
						calls.Add(1)
						return testenv.Response(200, `{"data":true}`), nil
					})
					testenv.Set("listmonk_URL", "https://listmonk.example.test")
					testenv.Set("listmonk_API_token", "oph:test-token")
					s := seller()
					s.DeliveryProvider = provider
					s.ListmonkInvoiceTemplate, s.ListmonkCreditTemplate = 41, 42
					testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", JSON(s))
					enqueue(t, "switch-payment", sample())
					if err := Process(); err != nil {
						t.Fatal(err)
					}
					if err := query.Transaction(func(tx *query.Tx) error {
						return QueueRefundCreditNote(tx, "razorpay", "switch-payment", "switch-refund", 11800, 11800)
					}); err != nil {
						t.Fatal(err)
					}
					if err := Process(); err != nil {
						t.Fatal(err)
					}
					if testenv.Scalar(t, "SELECT COUNT(*) FROM invoices WHERE status='issued' AND pdf IS NOT NULL") != 2 {
						t.Fatal("email setting blocked invoice or credit-note PDF generation")
					}
					assertSent := func(want int64) {
						t.Helper()
						if testenv.Scalar(t, "SELECT SUM(email_done) FROM invoices") != want {
							t.Fatal("submission status does not match actual email submission")
						}
						if provider == "listmonk" {
							if calls.Load() != want || len(sender.emails) != 0 {
								t.Fatalf("Listmonk submissions=%d, current service=%d; want %d", calls.Load(), len(sender.emails), want)
							}
						} else if int64(len(sender.emails)) != want || calls.Load() != 0 {
							t.Fatalf("current service submissions=%d, Listmonk=%d; want %d", len(sender.emails), calls.Load(), want)
						}
						if mail.Production != production {
							t.Fatal("invoice sending changed the global mail environment")
						}
					}
					if flag == "yes" {
						assertSent(2)
					} else {
						assertSent(0)
						if testenv.Scalar(t, "SELECT SUM(attempts + lease_until + next_attempt_at) FROM invoices") != 0 {
							t.Fatal("disabled email consumed retries or retained a lease")
						}
						before := testenv.Text(t, "SELECT group_concat(number || ':' || hex(pdf), '|') FROM (SELECT number,pdf FROM invoices ORDER BY kind)")
						testenv.Set("automated_invoice", "yes")
						if err := Process(); err != nil {
							t.Fatal(err)
						}
						assertSent(2)
						if before != testenv.Text(t, "SELECT group_concat(number || ':' || hex(pdf), '|') FROM (SELECT number,pdf FROM invoices ORDER BY kind)") {
							t.Fatal("enabling email changed issued numbers or PDFs")
						}
					}
					if err := Process(); err != nil {
						t.Fatal(err)
					}
					assertSent(2)
				})
			}
		}
	}
}

func TestInvoiceEmailRejectsInvalidBooleanWithoutConsumingRetry(t *testing.T) {
	for _, value := range []string{"", "true", "false", "1", "0", "on", "off", "YES", "yes ", "invalid"} {
		t.Run(value, func(t *testing.T) {
			sender := setup(t)
			testenv.Set("automated_invoice", value)
			enqueue(t, "invalid-email-flag", sample())
			if err := Process(); err == nil || !strings.Contains(err.Error(), "automated_invoice") {
				t.Fatal("invalid boolean did not return a configuration error", err)
			}
			if len(sender.emails) != 0 || testenv.Scalar(t, "SELECT email_done + attempts + next_attempt_at + lease_until FROM invoices") != 0 {
				t.Fatal("invalid configuration sent email or consumed a retry")
			}
			if testenv.Scalar(t, "SELECT pdf IS NOT NULL FROM invoices") != 1 {
				t.Fatal("invalid email configuration blocked the PDF")
			}
			number := testenv.Text(t, "SELECT number FROM invoices")
			testenv.Set("automated_invoice", "yes")
			if err := Process(); err != nil {
				t.Fatal(err)
			}
			if len(sender.emails) != 1 || number != testenv.Text(t, "SELECT number FROM invoices") {
				t.Fatal("correcting configuration did not resume the same invoice")
			}
		})
	}
}
