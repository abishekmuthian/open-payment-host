package invoices

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/mail"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
)

func seller() Settings {
	return Settings{Name: "Seller", Address: "Coimbatore, Tamil Nadu", GSTIN: "33AAAAA0000A1Z9", State: "33", Email: "seller@example.test", Signatory: "Seller", Prefix: "OPH", CreditPrefix: "CN", Start: 1, LUT: "LUT-TEST", LUTYear: FinancialYear(time.Now())}
}
func buyer() Billing {
	return Billing{Name: "Buyer", Email: "buyer@example.test", Address: "1 Main Street", City: "Chennai", State: "TN", PostalCode: "600001", Country: "IN"}
}
func sample() Snapshot {
	return Snapshot{Context: Context{Seller: seller(), Product: Product{Enabled: true, Code: "998439", Rate: "18", Inclusive: true, Export: true}, Description: "Digital service", Base: 11800, Country: "IN"}, Billing: buyer(), Gross: 11800, Currency: "INR", PaidAt: time.Now().UTC().Format(time.RFC3339)}
}
func TestGSTCalculation(t *testing.T) {
	for _, tc := range []struct {
		name, state                        string
		inclusive                          bool
		gross, base, net, cgst, sgst, igst int64
	}{{"inclusive", "TN", true, 10000, 10000, 8475, 762, 763, 0}, {"exclusive", "TN", false, 11800, 10000, 10000, 900, 900, 0}, {"interstate", "HR", false, 11800, 10000, 10000, 0, 0, 1800}, {"rounding", "TN", true, 17500, 17500, 14831, 1334, 1335, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			s := sample()
			s.Context.Product.Inclusive = tc.inclusive
			s.Context.Base = tc.base
			s.Gross = tc.gross
			s.Billing.State = tc.state
			if err := Calculate(&s); err != nil {
				t.Fatal(err)
			}
			if s.Net != tc.net || s.CGST != tc.cgst || s.SGST != tc.sgst || s.IGST != tc.igst {
				t.Fatalf("bad breakdown %+v", s)
			}
			total, err := Total(tc.base, s.Context.Product, "IN")
			if err != nil || total != tc.gross {
				t.Fatalf("charge=%d %v", total, err)
			}
		})
	}
	s := sample()
	s.Billing.Country = "US"
	s.Billing.State = "MA"
	s.Billing.PostalCode = "02420"
	s.Context.Country = "US"
	s.Currency = "USD"
	if err := Calculate(&s); err != nil || !s.Export || s.Net != s.Gross {
		t.Fatalf("export %v %+v", err, s)
	}
	s.Context.Seller.LUTYear = "0000"
	if Calculate(&s) == nil {
		t.Fatal("expired LUT accepted")
	}
	s = sample()
	s.Gross++
	if Calculate(&s) == nil {
		t.Fatal("mismatch accepted")
	}
	s = sample()
	s.Context.Country = "US"
	if Calculate(&s) == nil {
		t.Fatal("geolocation/billing mismatch accepted")
	}
}
func TestGSTValidationAndRollover(t *testing.T) {
	if !ValidGSTIN("33AAAAA0000A1Z9") || ValidGSTIN("33AAAAA0000A1Z0") {
		t.Fatal("GSTIN checksum")
	}
	b := buyer()
	b.GSTIN = "33AAAAA0000A1Z9"
	b.State = "HR"
	if b.Validate() == nil {
		t.Fatal("state mismatch")
	}
	for _, tc := range []struct{ at, want string }{{"2026-03-31T18:29:59Z", "2526"}, {"2026-03-31T18:30:00Z", "2627"}} {
		at, _ := time.Parse(time.RFC3339, tc.at)
		if FinancialYear(at) != tc.want {
			t.Fatal(tc)
		}
	}
	if _, err := Total(9223372036854775807, Product{Enabled: true, Rate: "18", Code: "998439"}, "IN"); err == nil {
		t.Fatal("overflow accepted")
	}
}
func TestSignatureValidation(t *testing.T) {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 100, 40)))
	if _, err := NormaliseSignature(b.Bytes()); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{[]byte("<svg></svg>"), make([]byte, (1<<20)+1)} {
		if _, err := NormaliseSignature(data); err == nil {
			t.Fatal("bad signature accepted")
		}
	}
}

type recordingSender struct {
	emails []*mail.Email
	fail   bool
	mu     sync.Mutex
}

func (s *recordingSender) Send(e *mail.Email) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("SMTP unavailable")
	}
	s.emails = append(s.emails, e)
	return nil
}
func setup(t *testing.T) *recordingSender {
	t.Helper()
	testenv.Config(t, map[string]string{"automated_invoice": "yes"})
	testenv.DB(t)
	prior, prod := mail.Service, mail.Production
	sender := &recordingSender{}
	mail.Service = sender
	mail.Production = true
	t.Cleanup(func() { mail.Service = prior; mail.Production = prod })
	return sender
}
func enqueue(t *testing.T, txid string, s Snapshot) {
	t.Helper()
	if err := query.Transaction(func(tx *query.Tx) error {
		return QueuePaidInvoice(tx, "attempt", "razorpay", txid, JSON(s.Context), JSON(s.Billing), s.Currency, s.Gross, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
}
func TestInvoiceDeliveryRefundsAndImmutability(t *testing.T) {
	sender := setup(t)
	s := sample()
	s.Context.Description = "  Lifetime access #digital #subscription "
	var signature bytes.Buffer
	_ = png.Encode(&signature, image.NewRGBA(image.Rect(0, 0, 140, 35)))
	s.Context.Seller.Signature = signature.Bytes()
	enqueue(t, "pay_1", s)
	enqueue(t, "pay_1", s)
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if len(sender.emails) != 1 || len(sender.emails[0].Attachments) != 1 || !bytes.HasPrefix(sender.emails[0].Attachments[0].Data, []byte("%PDF-")) {
		t.Fatal("missing PDF email")
	}
	email := sender.emails[0]
	if !strings.Contains(email.Body, "for Lifetime access from Seller") || strings.Contains(email.Body, "#") || !strings.Contains(email.Body, "Thank you for your purchase!") || !strings.Contains(email.Body, "If you need any help with the product, please reply to this email any time.") || email.ReplyTo != seller().Email {
		t.Fatal("current-service invoice email lost product or purchase/support copy")
	}
	number := testenv.Text(t, "SELECT number FROM invoices")
	if !strings.HasSuffix(number, "/000001") {
		t.Fatal(number)
	}
	original := append([]byte(nil), sender.emails[0].Attachments[0].Data...)
	testenv.Exec(t, "UPDATE invoices SET email_done=0")
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, sender.emails[1].Attachments[0].Data) {
		t.Fatal("issued PDF changed")
	}
	for i, refund := range []int64{3300, 8500} {
		cumulative := int64(3300)
		if i == 1 {
			cumulative = 11800
		}
		err := query.Transaction(func(tx *query.Tx) error {
			return QueueRefundCreditNote(tx, "razorpay", "pay_1", []string{"refund_1", "refund_2"}[i], refund, cumulative)
		})
		if err != nil {
			t.Fatal(err)
		}
		if err = Process(); err != nil {
			t.Fatal(err)
		}
		credit := sender.emails[len(sender.emails)-1]
		if !strings.Contains(credit.Body, "for Lifetime access against invoice") || strings.Contains(credit.Body, "#") || strings.Contains(credit.Body, "If you need any help with the product") || credit.ReplyTo != seller().Email {
			t.Fatal("current-service credit note lost product/reply details or retained old support text")
		}
		if !strings.Contains(credit.Body, "Sorry you didn't like my product. Let me know how I can make it better by replying to this email.") || strings.Contains(email.Body, "Sorry you didn't like my product.") {
			t.Fatal("current-service feedback request must appear only in credit notes")
		}
		refund := strings.Index(credit.Body, "Your refund has been processed and the amount will be credited back to your account within 5-7 working days.")
		if refund < 0 || refund > strings.Index(credit.Body, "Sorry you didn't like my product.") || strings.Contains(email.Body, "Your refund has been processed") {
			t.Fatal("current-service refund message must precede feedback only in credit notes")
		}
	}
	rows, err := query.New("invoices", "id").Where("kind='credit_note'").Results()
	if err != nil {
		t.Fatal(err)
	}
	var gross, net, cgst, sgst int64
	for _, r := range rows {
		var doc Snapshot
		if err = json.Unmarshal([]byte(r["snapshot"].(string)), &doc); err != nil {
			t.Fatal(err)
		}
		gross += doc.Gross
		net += doc.Net
		cgst += doc.CGST
		sgst += doc.SGST
		if doc.OriginalDate == "" || doc.RefundReference == "" {
			t.Fatal("credit note lacks original date/refund reference")
		}
		if doc.OriginalNumber != number {
			t.Fatal("missing original reference")
		}
	}
	if gross != 11800 || net != 10000 || cgst != 900 || sgst != 900 {
		t.Fatalf("refund sums %d %d %d %d", gross, net, cgst, sgst)
	}
}
func TestInvoiceRetryAndMissingBilling(t *testing.T) {
	sender := setup(t)
	sender.fail = true
	enqueue(t, "pay_fail", sample())
	if Process() == nil {
		t.Fatal("SMTP failure hidden")
	}
	if testenv.Text(t, "SELECT status FROM invoices") != "issued" {
		t.Fatal("PDF not retained")
	}
	number := testenv.Text(t, "SELECT number FROM invoices")
	sender.fail = false
	testenv.Exec(t, "UPDATE invoices SET next_attempt_at=0")
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if number != testenv.Text(t, "SELECT number FROM invoices") {
		t.Fatal("number changed")
	}
	s := sample()
	s.Billing = Billing{}
	enqueue(t, "missing", s)
	if Process() == nil {
		t.Fatal("missing billing accepted")
	}
	if testenv.Text(t, "SELECT status FROM invoices WHERE transaction_id='missing'") != "needs_review" {
		t.Fatal("no admin task")
	}
}
func TestConcurrentNumberingAndAtomicQueue(t *testing.T) {
	setup(t)
	s := sample()
	enqueue(t, "one", s)
	enqueue(t, "two", s)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = Process() }()
	}
	wg.Wait()
	if testenv.Scalar(t, "SELECT COUNT(DISTINCT number) FROM invoices WHERE number<>''") != 2 {
		t.Fatal("concurrent numbers")
	}
	err := query.Transaction(func(tx *query.Tx) error {
		if err := QueuePaidInvoice(tx, "a", "stripe", "rolled-back", JSON(s.Context), JSON(s.Billing), s.Currency, s.Gross, time.Now()); err != nil {
			return err
		}
		return errors.New("rollback")
	})
	if err == nil || testenv.Scalar(t, "SELECT COUNT(*) FROM invoices WHERE transaction_id='rolled-back'") != 0 {
		t.Fatal("queue escaped transaction")
	}
}

func TestNativeTaxAndLowercaseCurrency(t *testing.T) {
	s := sample()
	s.Currency = "inr"
	if err := Calculate(&s); err != nil || s.Currency != "INR" {
		t.Fatal("lowercase provider currency", err)
	}
	s = sample()
	s.Context.Product.Rate = "12"
	s.Context.Base = 14
	s.Gross = 14
	tax := int64(2)
	s.NativeTax = &tax
	if err := Calculate(&s); err != nil || s.Net != 12 {
		t.Fatal("inclusive half-cent rounding", err, s.Net)
	}
	tax = 10
	if Calculate(&s) == nil {
		t.Fatal("conflicting native tax accepted")
	}
}

func TestAdminSignatureUploadAndPrivateAccess(t *testing.T) {
	testenv.Setup(t, nil)
	adminID := testenv.SeedUser(t, 100, "admin@example.test", "test-admin-password")
	testenv.Route("GET", "/admin/invoicing/signature", HandleSignature)
	testenv.Route("POST", "/admin/invoicing", HandleSettings)
	var imageData bytes.Buffer
	_ = png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 120, 40)))
	settings := seller()
	values := url.Values{"name": {settings.Name}, "address": {settings.Address}, "gstin": {settings.GSTIN}, "state": {settings.State}, "email": {settings.Email}, "signatory": {settings.Signatory}, "prefix": {settings.Prefix}, "credit_prefix": {settings.CreditPrefix}, "start": {"1"}}
	r := testenv.AuthedPOST(t, "/admin/invoicing", values, adminID)
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, values := range r.PostForm {
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	part, err := writer.CreateFormFile("signature", "signature.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(imageData.Bytes())
	_ = writer.Close()
	r.Body = io.NopCloser(bytes.NewReader(body.Bytes()))
	r.ContentLength = int64(body.Len())
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Form = nil
	r.PostForm = nil
	w := testenv.Serve(t, r)
	if w.Code != 302 {
		t.Fatal(w.Code, w.Body.String())
	}
	stored, err := LoadSettings()
	if err != nil || len(stored.Signature) == 0 {
		t.Fatal("signature not stored", err)
	}
	r = httptest.NewRequest(http.MethodGet, "/admin/invoicing/signature", nil)
	if w = testenv.Serve(t, r); w.Code != 401 {
		t.Fatal("private signature exposed")
	}
	r = httptest.NewRequest(http.MethodGet, "/admin/invoicing/signature", nil)
	testenv.LoggedIn(t, r, adminID)
	w = testenv.Serve(t, r)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !bytes.Equal(w.Body.Bytes(), stored.Signature) {
		t.Fatal("admin signature preview", w.Code)
	}
}

func TestAdminCannotAlterIssuedDocument(t *testing.T) {
	setup(t)
	testenv.Templates(t)
	testenv.Auth(t)
	adminID := testenv.SeedUser(t, 100, "admin@example.test", "test-admin-password")
	testenv.Route("POST", "/admin/invoices/{id:[a-zA-Z0-9-]+}", HandleDocument)
	enqueue(t, "issued", sample())
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	id := testenv.Text(t, "SELECT id FROM invoices")
	snapshot := testenv.Text(t, "SELECT snapshot FROM invoices")
	number := testenv.Text(t, "SELECT number FROM invoices")
	r := testenv.AuthedPOST(t, "/admin/invoices/"+id, url.Values{"billing_name": {"Attacker"}, "invoice_rate": {"1"}, "invoice_base": {"1"}}, adminID)
	w := testenv.Serve(t, r)
	if w.Code != 302 {
		t.Fatal(w.Code, w.Body.String())
	}
	if snapshot != testenv.Text(t, "SELECT snapshot FROM invoices") || number != testenv.Text(t, "SELECT number FROM invoices") {
		t.Fatal("issued document mutated")
	}
}

func TestBillingStatePreservedForProviderSDKs(t *testing.T) {
	b := buyer()
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	if b.State != "TN" {
		t.Fatal("provider state abbreviation replaced by GST numeric code")
	}
	s := sample()
	s.Billing = b
	if err := Calculate(&s); err != nil || s.IGST != 0 || s.CGST != 900 {
		t.Fatal("canonical GST comparison", err)
	}
}

func TestReviewedLegacyProfileAppliesToFutureRenewals(t *testing.T) {
	setup(t)
	testenv.Templates(t)
	testenv.Auth(t)
	adminID := testenv.SeedUser(t, 100, "admin@example.test", "test-admin-password")
	testenv.Route("POST", "/admin/invoices/{id:[a-zA-Z0-9-]+}", HandleDocument)
	testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", JSON(seller()))
	testenv.Exec(t, "INSERT INTO payment_attempts(id,schedule,browser_token_hash,status) VALUES('attempt','monthly','unused','completed')")
	s := sample()
	s.Context.ReviewReason = "legacy review required"
	s.Context.Base = 0
	s.Billing = Billing{}
	enqueue(t, "legacy", s)
	_ = Process()
	id := testenv.Text(t, "SELECT id FROM invoices")
	form := url.Values{"billing_name": {"Buyer"}, "billing_email": {"buyer@example.test"}, "billing_address": {"1 Main Street"}, "billing_city": {"Chennai"}, "billing_state": {"TN"}, "billing_country": {"IN"}, "billing_postcode": {"600001"}, "invoice_code": {"998439"}, "invoice_rate": {"18"}, "invoice_base": {"10000"}, "save_profile": {"on"}}
	w := testenv.Serve(t, testenv.AuthedPOST(t, "/admin/invoices/"+id, form, adminID))
	if w.Code != 302 {
		t.Fatal(w.Code, w.Body.String())
	}
	context := testenv.Text(t, "SELECT invoice_context FROM payment_attempts WHERE id='attempt'")
	billing := testenv.Text(t, "SELECT invoice_billing FROM payment_attempts WHERE id='attempt'")
	if context == "" || billing == "" {
		t.Fatal("reviewed profile not saved")
	}
	if err := query.Transaction(func(tx *query.Tx) error {
		return QueuePaidInvoice(tx, "attempt", "razorpay", "future", context, billing, "INR", 11800, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if testenv.Scalar(t, "SELECT COUNT(*) FROM invoices WHERE status='issued'") != 2 {
		t.Fatal("future renewal required repeat manual review")
	}
}

func TestDeliverySettingsValidation(t *testing.T) {
	s := seller()
	if err := s.ValidateDelivery(); err != nil {
		t.Fatal(err)
	}
	s.DeliveryProvider = "listmonk"
	s.ListmonkInvoiceTemplate, s.ListmonkCreditTemplate = 41, 42
	if err := s.ValidateDelivery(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Settings){
		func(s *Settings) { s.DeliveryProvider = "unknown" },
		func(s *Settings) { s.ListmonkInvoiceTemplate = 0 },
		func(s *Settings) { s.ListmonkCreditTemplate = -1 },
		func(s *Settings) { s.ListmonkMessenger = "email\r\nInjected: value" },
		func(s *Settings) { s.ListmonkFrom = "not an email" },
		func(s *Settings) { s.ListmonkFrom = "sender@example.test\r\nInjected: value" },
	} {
		bad := s
		change(&bad)
		if bad.ValidateDelivery() == nil {
			t.Fatal("invalid delivery settings accepted")
		}
	}
}
