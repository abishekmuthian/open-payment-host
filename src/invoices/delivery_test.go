package invoices

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/listmonk"
	"github.com/abishekmuthian/open-payment-host/src/lib/mail"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
)

type submittedDocument struct {
	message  listmonk.TransactionalMessage
	filename string
	pdf      []byte
}

func TestListmonkDocumentSubmissionAndRecovery(t *testing.T) {
	smtp := setup(t)
	var fail atomic.Bool
	fail.Store(true)
	var mu sync.Mutex
	var received []submittedDocument
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tx" || r.Header.Get("Authorization") != "token oph:secret" {
			t.Error("wrong Listmonk request or authentication")
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		var got submittedDocument
		if err := json.Unmarshal([]byte(r.FormValue("data")), &got.message); err != nil {
			t.Error(err)
		}
		f, header, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer f.Close()
		got.filename = header.Filename
		got.pdf, err = io.ReadAll(f)
		if err != nil {
			t.Error(err)
		}
		mu.Lock()
		received = append(received, got)
		mu.Unlock()
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		_, _ = io.WriteString(w, `{"data":true}`)
	}))
	defer server.Close()
	testenv.Set("listmonk_URL", server.URL)
	testenv.Set("listmonk_API_token", "oph:secret")
	s := seller()
	s.DeliveryProvider = "listmonk"
	s.ListmonkInvoiceTemplate, s.ListmonkCreditTemplate = 41, 42
	s.ListmonkMessenger, s.ListmonkFrom = "billing", "billing@example.test"
	testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", JSON(s))
	document := sample()
	document.Context.Description = "  Product & training #digital #education "
	enqueue(t, "listmonk-payment", document)
	enqueue(t, "listmonk-payment", document)
	if Process() == nil {
		t.Fatal("Listmonk failure was hidden")
	}
	if testenv.Scalar(t, "SELECT COUNT(*) FROM invoices") != 1 || testenv.Text(t, "SELECT status FROM invoices") != "issued" || testenv.Scalar(t, "SELECT email_done FROM invoices") != 0 || len(smtp.emails) != 0 {
		t.Fatal("PDF generation, deduplication, or selected provider changed on failure")
	}
	number := testenv.Text(t, "SELECT number FROM invoices")
	fail.Store(false)
	s.Name = "Changed seller after issuance"
	testenv.Exec(t, "UPDATE invoice_settings SET value=?", JSON(s))
	testenv.Exec(t, "UPDATE invoices SET next_attempt_at=0")
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if testenv.Scalar(t, "SELECT email_done FROM invoices") != 1 || number != testenv.Text(t, "SELECT number FROM invoices") {
		t.Fatal("persisted submission did not resume")
	}
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	first, retry, count := received[0], received[1], len(received)
	mu.Unlock()
	if count != 2 || !bytes.Equal(first.pdf, retry.pdf) || retry.filename != numberToFilename(number) || !bytes.HasPrefix(retry.pdf, []byte("%PDF")) {
		t.Fatal("retry regenerated the PDF or duplicated an accepted submission")
	}
	msg := retry.message
	if msg.TemplateID != 41 || msg.SubscriberMode != "external" || msg.FromEmail != "billing@example.test" || msg.Messenger != "billing" || msg.SubscriberEmails[0] != buyer().Email || msg.Data["seller_name"] != seller().Name || msg.Data["customer_name"] != buyer().Name || msg.Data["document_number"] != number {
		t.Fatalf("invoice message lost frozen data or delivery settings: %+v", msg)
	}
	if msg.Headers[0]["Message-ID"] != first.message.Headers[0]["Message-ID"] || msg.Headers[1]["Reply-To"] != seller().Email {
		t.Fatal("message identity or seller reply address changed")
	}
	if msg.Data["product_name"] != "Product & training" || !strings.Contains(msg.AltBody, "for Product & training from Seller") || strings.Contains(msg.AltBody, "#") || !strings.Contains(msg.AltBody, "Thank you for your purchase!") || !strings.Contains(msg.AltBody, "If you need any help with the product, please reply to this email any time.") {
		t.Fatal("invoice email lost the clean product description or purchase/support copy")
	}
	if !strings.Contains(testenv.Text(t, "SELECT snapshot FROM invoices"), "#digital #education") {
		t.Fatal("email formatting changed the frozen description")
	}
	if err := query.Transaction(func(tx *query.Tx) error {
		return QueueRefundCreditNote(tx, "razorpay", "listmonk-payment", "listmonk-refund", 3300, 3300)
	}); err != nil {
		t.Fatal(err)
	}
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	credit := received[len(received)-1]
	mu.Unlock()
	if credit.message.TemplateID != 42 || credit.message.Data["original_invoice_number"] != number || credit.message.Data["seller_name"] != seller().Name || bytes.Equal(credit.pdf, retry.pdf) || !strings.Contains(credit.message.AltBody, "credit note") || !strings.Contains(credit.message.AltBody, number) {
		t.Fatal("credit note used the wrong template, snapshot, or PDF")
	}
	if credit.message.Data["product_name"] != "Product & training" || !strings.Contains(credit.message.AltBody, "for Product & training against invoice") || strings.Contains(credit.message.AltBody, "#") || strings.Contains(credit.message.AltBody, "If you need any help with the product") || credit.message.Headers[1]["Reply-To"] != seller().Email {
		t.Fatal("credit note lost product/reply details or retained the old support text")
	}
	if !strings.Contains(credit.message.AltBody, "Sorry you didn't like my product. Let me know how I can make it better by replying to this email.") || strings.Contains(msg.AltBody, "Sorry you didn't like my product.") {
		t.Fatal("feedback request must appear only in the credit-note email")
	}
	refund := strings.Index(credit.message.AltBody, "Your refund has been processed and the amount will be credited back to your account within 5-7 working days.")
	if refund < 0 || refund > strings.Index(credit.message.AltBody, "Sorry you didn't like my product.") || strings.Contains(msg.AltBody, "Your refund has been processed") {
		t.Fatal("refund message must precede feedback and appear only in credit notes")
	}
}

func TestResendIssuedDocumentPreservesIdentityAndLease(t *testing.T) {
	sender := setup(t)
	testenv.Auth(t)
	testenv.Templates(t)
	adminID := testenv.SeedUser(t, 100, "admin@example.test", "test-admin-password")
	testenv.Route("POST", "/admin/invoices/{id:[a-zA-Z0-9-]+}", HandleDocument)
	testenv.Route("GET", "/admin/invoices/{id:[a-zA-Z0-9-]+}", HandleDocument)
	enqueue(t, "resend-payment", sample())
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	id := testenv.Text(t, "SELECT id FROM invoices")
	number := testenv.Text(t, "SELECT number FROM invoices")
	snapshot := testenv.Text(t, "SELECT snapshot FROM invoices")
	post := func() int {
		return testenv.Serve(t, testenv.AuthedPOST(t, "/admin/invoices/"+id, url.Values{"action": {"resend"}, "billing_name": {"Changed buyer"}}, adminID)).Code
	}
	testenv.Exec(t, "UPDATE invoices SET lease_until=?", time.Now().Add(time.Minute).Unix())
	if post() != 400 || testenv.Scalar(t, "SELECT email_done FROM invoices") != 1 {
		t.Fatal("active worker lease was bypassed")
	}
	testenv.Exec(t, "UPDATE invoices SET lease_until=0")
	r := httptest.NewRequest(http.MethodGet, "/admin/invoices/"+id, nil)
	testenv.LoggedIn(t, r, adminID)
	w := testenv.Serve(t, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Resend email") || !strings.Contains(w.Body.String(), "Submitted to email service") {
		t.Fatal("accepted document has no explicit resend action", w.Code)
	}
	if post() != 302 || testenv.Scalar(t, "SELECT email_done FROM invoices") != 0 || post() != 400 {
		t.Fatal("resend did not requeue exactly once")
	}
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if len(sender.emails) != 2 || !bytes.Equal(sender.emails[0].Attachments[0].Data, sender.emails[1].Attachments[0].Data) || sender.emails[0].MessageID != sender.emails[1].MessageID || number != testenv.Text(t, "SELECT number FROM invoices") || snapshot != testenv.Text(t, "SELECT snapshot FROM invoices") {
		t.Fatal("resend changed immutable contents or message identity")
	}
	reader := testenv.SeedUser(t, 20, "reader@example.test", "test-reader-password")
	if testenv.Serve(t, testenv.AuthedPOST(t, "/admin/invoices/"+id, url.Values{"action": {"resend"}}, reader)).Code != 401 {
		t.Fatal("non-admin resend allowed")
	}
	r = httptest.NewRequest(http.MethodPost, "/admin/invoices/"+id, strings.NewReader("action=resend"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	testenv.LoggedIn(t, r, adminID)
	if testenv.Serve(t, r).Code != 401 {
		t.Fatal("resend without CSRF allowed")
	}
}

func multipartSettingsPost(t *testing.T, values url.Values, adminID int64) *http.Request {
	t.Helper()
	r := testenv.AuthedPOST(t, "/admin/invoicing", values, adminID)
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, values := range r.PostForm {
		for _, v := range values {
			if err := writer.WriteField(k, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	r.Body = io.NopCloser(&body)
	r.ContentLength = int64(body.Len())
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Form, r.PostForm = nil, nil
	return r
}

func TestDeliverySettingsAndPartialAuthorization(t *testing.T) {
	testenv.Setup(t, map[string]string{"listmonk_URL": "https://listmonk.example.test", "listmonk_API_token": "oph:secret"})
	adminID := testenv.SeedUser(t, 100, "admin@example.test", "test-admin-password")
	readerID := testenv.SeedUser(t, 20, "reader@example.test", "test-reader-password")
	testenv.Route("POST", "/admin/invoicing/delivery", HandleDeliveryFields)
	testenv.Route("POST", "/admin/invoicing", HandleSettings)
	s := seller()
	testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", JSON(s))
	values := url.Values{"name": {s.Name}, "address": {s.Address}, "gstin": {s.GSTIN}, "state": {s.State}, "email": {s.Email}, "signatory": {s.Signatory}, "prefix": {s.Prefix}, "credit_prefix": {s.CreditPrefix}, "start": {"1"}, "delivery_provider": {"listmonk"}, "listmonk_invoice_template": {"41"}, "listmonk_credit_template": {"42"}}
	w := testenv.Serve(t, multipartSettingsPost(t, values, adminID))
	if w.Code != 302 {
		t.Fatal("Listmonk settings not saved", w.Code, w.Body.String())
	}
	stored, err := LoadSettings()
	if err != nil || stored.DeliveryProvider != "listmonk" || stored.ListmonkInvoiceTemplate != 41 {
		t.Fatal("provider settings not saved", err)
	}
	for _, provider := range []string{"listmonk", "default"} {
		w = testenv.Serve(t, testenv.AuthedPOST(t, "/admin/invoicing/delivery", url.Values{"delivery_provider": {provider}}, adminID))
		body := w.Body.String()
		if w.Code != 200 || strings.Contains(body, "<form") || strings.Contains(body, "oph:secret") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("partial leaked data or page layout", w.Code)
		}
		if strings.Contains(body, `name="listmonk_invoice_template"`) != (provider == "listmonk") || strings.Contains(body, `name="smtp_`) {
			t.Fatal("inactive provider fields rendered")
		}
		if provider == "default" && (!strings.Contains(body, "in secrets") || strings.Contains(body, "<input") || strings.Contains(body, "<select")) {
			t.Fatal("current mail service must explain secrets configuration without editable fields")
		}
	}
	for _, invalid := range []string{"0", "-1", "oops", "999999999999999999999999999"} {
		values.Set("listmonk_invoice_template", invalid)
		if testenv.Serve(t, multipartSettingsPost(t, values, adminID)).Code != 400 {
			t.Fatal("invalid template ID accepted")
		}
	}
	values.Set("listmonk_invoice_template", "41")
	values.Set("delivery_provider", "default")
	if testenv.Serve(t, multipartSettingsPost(t, values, adminID)).Code != 302 {
		t.Fatal("switching to current mail service failed")
	}
	// A stale settings page cannot restore a removed SMTP override.
	values.Set("smtp_host", "obsolete.example.test")
	values.Set("smtp_port", "not a port")
	values.Set("smtp_from", "obsolete@example.test")
	values.Set("smtp_mode", "plain")
	if testenv.Serve(t, multipartSettingsPost(t, values, adminID)).Code != 302 {
		t.Fatal("obsolete form fields blocked current mail service settings")
	}
	stored, _ = LoadSettings()
	if stored.ListmonkInvoiceTemplate != 41 || stored.ListmonkCreditTemplate != 42 {
		t.Fatal("inactive Listmonk settings lost")
	}
	if raw := testenv.Text(t, "SELECT value FROM invoice_settings"); strings.Contains(raw, "SMTPHost") || strings.Contains(raw, "obsolete.example.test") {
		t.Fatal("removed SMTP fields were persisted")
	}
	if testenv.Serve(t, testenv.AuthedPOST(t, "/admin/invoicing/delivery", url.Values{"delivery_provider": {"unknown"}}, adminID)).Code != 400 {
		t.Fatal("unknown provider rendered")
	}
	if testenv.Serve(t, testenv.AuthedPOST(t, "/admin/invoicing/delivery", nil, readerID)).Code != 401 {
		t.Fatal("non-admin provider access")
	}
	r := httptest.NewRequest(http.MethodPost, "/admin/invoicing/delivery", nil)
	testenv.LoggedIn(t, r, adminID)
	if testenv.Serve(t, r).Code != 401 {
		t.Fatal("provider partial allowed without CSRF")
	}
}

func TestCurrentMailServiceIgnoresLegacyInvoiceSMTPOverrides(t *testing.T) {
	sender := setup(t)
	var legacy map[string]interface{}
	if err := json.Unmarshal([]byte(JSON(seller())), &legacy); err != nil {
		t.Fatal(err)
	}
	legacy["SMTPHost"], legacy["SMTPPort"] = "obsolete.example.test", "not a port"
	legacy["SMTPFrom"], legacy["SMTPMode"] = "obsolete@example.test", "plain"
	legacy["DeliveryProvider"] = "default"
	testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", JSON(legacy))
	enqueue(t, "current-service", sample())
	if err := Process(); err != nil {
		t.Fatal("legacy invoice-page SMTP values blocked current mail service", err)
	}
	if len(sender.emails) != 1 || testenv.Scalar(t, "SELECT email_done FROM invoices") != 1 {
		t.Fatal("invoice did not use the configured application mail sender")
	}
}

func TestInvoiceDeliveryConfigurationDoesNotBlockPDFs(t *testing.T) {
	sender := setup(t)
	s := seller()
	s.DeliveryProvider, s.ListmonkInvoiceTemplate, s.ListmonkCreditTemplate = "listmonk", 41, 42
	testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", JSON(s))
	enqueue(t, "missing-config", sample())
	if Process() == nil || testenv.Scalar(t, "SELECT pdf IS NOT NULL FROM invoices") != 1 || testenv.Scalar(t, "SELECT email_done FROM invoices") != 0 || len(sender.emails) != 0 {
		t.Fatal("missing Listmonk credentials blocked PDF or switched sender")
	}
	testenv.Exec(t, "UPDATE invoice_settings SET value='corrupt'")
	testenv.Exec(t, "UPDATE invoices SET next_attempt_at=0")
	if Process() == nil || len(sender.emails) != 0 {
		t.Fatal("corrupt settings silently switched providers")
	}
	mail.Production = false
	testenv.Exec(t, "UPDATE invoices SET next_attempt_at=0")
	if Process() == nil || len(sender.emails) != 0 || testenv.Scalar(t, "SELECT email_done FROM invoices") != 0 {
		t.Fatal("development bypassed delivery configuration validation")
	}
	testenv.Set("automated_invoice", "no")
	testenv.Exec(t, "UPDATE invoices SET next_attempt_at=0")
	if err := Process(); err != nil || len(sender.emails) != 0 || testenv.Scalar(t, "SELECT email_done FROM invoices") != 0 {
		t.Fatal("disabled invoice email was submitted or marked submitted", err)
	}
}

func TestCanonicalTransactionalTemplates(t *testing.T) {
	testenv.Root(t)
	data := map[string]interface{}{"Tx": map[string]interface{}{"Data": map[string]string{
		"customer_name": "Buyer <script>", "product_name": "Product <script> & tools", "seller_name": "Seller & Co", "document_number": "OPH/2627/000001", "original_invoice_number": "OPH/2627/000002",
	}}}
	for _, name := range []string{"oph_invoice", "oph_credit_note"} {
		body, err := os.ReadFile("src/invoices/email_templates/" + name + ".html")
		if err != nil {
			t.Fatal(err)
		}
		tpl, err := template.New(name).Option("missingkey=error").Parse(string(body))
		if err != nil {
			t.Fatal(err)
		}
		var rendered bytes.Buffer
		if err := tpl.Execute(&rendered, data); err != nil || strings.Contains(rendered.String(), "<script>") || !strings.Contains(rendered.String(), "Buyer &lt;script&gt;") {
			t.Fatal("template variables missing or unescaped", err)
		}
		if !strings.Contains(rendered.String(), "Product &lt;script&gt; &amp; tools") {
			t.Fatal("product name was missing or unescaped")
		}
		if strings.Contains(rendered.String(), "If you need any help with the product, please reply to this email any time.") != (name == "oph_invoice") {
			t.Fatal("generic support text must appear only in the invoice template")
		}
		if name == "oph_invoice" && !strings.Contains(rendered.String(), "Thank you for your purchase!") {
			t.Fatal("invoice template lacks purchase thank-you")
		}
		if strings.Contains(rendered.String(), "Sorry you didn't like my product. Let me know how I can make it better by replying to this email.") != (name == "oph_credit_note") {
			t.Fatal("feedback request must appear only in the credit-note template")
		}
		refund := strings.Index(rendered.String(), "Your refund has been processed and the amount will be credited back to your account within 5-7 working days.")
		if (refund >= 0) != (name == "oph_credit_note") || (name == "oph_credit_note" && refund > strings.Index(rendered.String(), "Sorry you didn't like my product.")) {
			t.Fatal("refund message must precede feedback and appear only in credit notes")
		}
	}
}
