package invoices

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
)

func customNumberingSettings() Settings {
	s := seller()
	s.NumberingStyle, s.SequenceDigits, s.Start = "prefix", 4, 17
	s.Prefix, s.CreditPrefix = "2026-AMJUL-", "CN-2026-"
	return s
}

func TestDocumentNumberFormatsAndBounds(t *testing.T) {
	at := time.Date(2026, 11, 14, 0, 0, 0, 0, india)
	s := seller()
	if got, err := s.documentNumber("invoice", 17, at); err != nil || got != "OPH/2627/000017" {
		t.Fatal("legacy format changed", got, err)
	}
	s = customNumberingSettings()
	for _, tc := range []struct {
		kind string
		seq  int64
		want string
	}{{"invoice", 17, "2026-AMJUL-0017"}, {"invoice", 18, "2026-AMJUL-0018"}, {"credit_note", 17, "CN-2026-0017"}, {"invoice", 99999, "2026-AMJUL-99999"}} {
		if got, err := s.documentNumber(tc.kind, tc.seq, at); err != nil || got != tc.want {
			t.Fatal(tc, got, err)
		}
	}
	if _, err := s.documentNumber("invoice", 100000, at); err == nil {
		t.Fatal("more than 16 characters accepted as sequence grows")
	}
	for _, bad := range []string{"2026 AMJUL-", "2026_AMJUL-", "2026.AMJUL-", "2026—AMJUL-", "<script>", ""} {
		s.Prefix = bad
		if s.Validate() == nil {
			t.Fatal("unsupported prefix accepted", bad)
		}
	}
	s = customNumberingSettings()
	s.CreditPrefix = strings.ToLower(s.Prefix)
	if s.Validate() == nil {
		t.Fatal("case-only prefix distinction accepted")
	}
	s = customNumberingSettings()
	s.NumberingStyle = "unknown"
	if s.Validate() == nil {
		t.Fatal("unknown numbering format accepted")
	}
	s = customNumberingSettings()
	s.SequenceDigits = 6
	if s.Validate() == nil {
		t.Fatal("prefix and padding exceeding 16 characters accepted")
	}
	s = seller()
	for _, tc := range []struct{ at, want string }{{"2026-03-31T18:29:59Z", "OPH/2526/000001"}, {"2026-03-31T18:30:00Z", "OPH/2627/000001"}} {
		at, _ := time.Parse(time.RFC3339, tc.at)
		if got, err := s.documentNumber("invoice", 1, at); err != nil || got != tc.want {
			t.Fatal("financial-year rollover", got, err)
		}
	}
}

func TestFinancialYearPrefixFormatRolloverAndBounds(t *testing.T) {
	s := customNumberingSettings()
	s.NumberingStyle, s.Prefix, s.CreditPrefix = "financial_year_prefix", "AMJUL", "CN"
	for _, tc := range []struct{ at, year string }{
		{"2026-03-31T18:29:59Z", "2025"},
		{"2026-03-31T18:30:00Z", "2026"},
		{"2027-01-01T00:00:00Z", "2026"},
		{"2027-03-31T18:29:59Z", "2026"},
		{"2027-03-31T18:30:00Z", "2027"},
	} {
		at, _ := time.Parse(time.RFC3339, tc.at)
		for _, kind := range []string{"invoice", "credit_note"} {
			prefix := "AMJUL"
			if kind == "credit_note" {
				prefix = "CN"
			}
			want := tc.year + "-" + prefix + "-0017"
			if got, err := s.documentNumber(kind, 17, at); err != nil || got != want {
				t.Fatalf("%s at %s: got %q, want %q: %v", kind, tc.at, got, want, err)
			}
		}
	}
	at := time.Date(2026, 11, 14, 0, 0, 0, 0, india)
	s.Prefix = "AMJULX"
	if got, err := s.documentNumber("invoice", 17, at); err != nil || got != "2026-AMJULX-0017" || len(got) != 16 {
		t.Fatal("16-character number rejected", got, err)
	}
	if _, err := s.documentNumber("invoice", 10000, at); err == nil {
		t.Fatal("sequence growth exceeded 16 characters")
	}
	s.Prefix = "AMJULXX"
	if s.Validate() == nil {
		t.Fatal("financial-year prefix format accepted an overlength number")
	}
}

func TestFinancialYearPrefixIssuanceAndCreditNote(t *testing.T) {
	setup(t)
	s := customNumberingSettings()
	s.NumberingStyle, s.Prefix, s.CreditPrefix = "financial_year_prefix", "AMJUL", "CN"
	testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", JSON(s))
	// A checkout created before the format change still uses current numbering.
	enqueue(t, "automatic-year", sample())
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	year := fmt.Sprintf("%04d", financialYearStart(time.Now()))
	if got := testenv.Text(t, "SELECT number FROM invoices WHERE kind='invoice'"); got != year+"-AMJUL-0017" {
		t.Fatal("automatic financial-year numbering not applied", got)
	}
	if err := query.Transaction(func(tx *query.Tx) error {
		return QueueRefundCreditNote(tx, "razorpay", "automatic-year", "automatic-refund", 5900, 5900)
	}); err != nil {
		t.Fatal(err)
	}
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if got := testenv.Text(t, "SELECT number FROM invoices WHERE kind='credit_note'"); got != year+"-CN-0017" {
		t.Fatal("credit note did not use financial-year-first numbering", got)
	}
}

func TestCustomNumberingContinuesAllGatewaysWithoutRewinding(t *testing.T) {
	sender := setup(t)
	frozen := sample()
	enqueue(t, "legacy-number", frozen)
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	legacyNumber := testenv.Text(t, "SELECT number FROM invoices WHERE transaction_id='legacy-number'")
	legacySnapshot := testenv.Text(t, "SELECT snapshot FROM invoices WHERE transaction_id='legacy-number'")
	s := customNumberingSettings()
	testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", JSON(s))
	for _, gateway := range []string{"stripe", "square", "paypal", "razorpay"} {
		if err := query.Transaction(func(tx *query.Tx) error {
			return QueuePaidInvoice(tx, "old-checkout-"+gateway, gateway, "new-number-"+gateway, JSON(frozen.Context), JSON(frozen.Billing), frozen.Currency, frozen.Gross, time.Now())
		}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for range 4 {
		wg.Go(func() { errors <- Process() })
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if testenv.Scalar(t, "SELECT COUNT(*) FROM invoices WHERE number IN ('2026-AMJUL-0017','2026-AMJUL-0018','2026-AMJUL-0019','2026-AMJUL-0020')") != 4 {
		t.Fatal("custom numbers were not consecutive across gateways and concurrent workers")
	}
	if legacyNumber != testenv.Text(t, "SELECT number FROM invoices WHERE transaction_id='legacy-number'") || legacySnapshot != testenv.Text(t, "SELECT snapshot FROM invoices WHERE transaction_id='legacy-number'") {
		t.Fatal("existing document changed with numbering settings")
	}
	s.Start = 1
	testenv.Exec(t, "UPDATE invoice_settings SET value=?", JSON(s))
	enqueue(t, "lower-floor", frozen)
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if testenv.Text(t, "SELECT number FROM invoices WHERE transaction_id='lower-floor'") != "2026-AMJUL-0021" {
		t.Fatal("counter rewound after lowering the minimum")
	}
	s.Start = 50
	testenv.Exec(t, "UPDATE invoice_settings SET value=?", JSON(s))
	enqueue(t, "raised-floor", frozen)
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if testenv.Text(t, "SELECT number FROM invoices WHERE transaction_id='raised-floor'") != "2026-AMJUL-0050" {
		t.Fatal("existing counter did not advance to configured minimum")
	}
	sender.fail = true
	enqueue(t, "reserved-number", frozen)
	if Process() == nil {
		t.Fatal("expected email failure after number reservation")
	}
	reserved := testenv.Text(t, "SELECT number FROM invoices WHERE transaction_id='reserved-number'")
	reservedPDF := testenv.Text(t, "SELECT hex(pdf) FROM invoices WHERE transaction_id='reserved-number'")
	s.Prefix, s.SequenceDigits, s.Start = "2026-NEW-", 5, 99
	testenv.Exec(t, "UPDATE invoice_settings SET value=?", JSON(s))
	testenv.Exec(t, "UPDATE invoices SET next_attempt_at=0 WHERE transaction_id='reserved-number'")
	sender.fail = false
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if reserved != "2026-AMJUL-0051" || reserved != testenv.Text(t, "SELECT number FROM invoices WHERE transaction_id='reserved-number'") || reservedPDF != testenv.Text(t, "SELECT hex(pdf) FROM invoices WHERE transaction_id='reserved-number'") {
		t.Fatal("retry changed a reserved number or PDF")
	}
	if err := query.Transaction(func(tx *query.Tx) error {
		return QueueRefundCreditNote(tx, "razorpay", "lower-floor", "custom-refund", 100, 100)
	}); err != nil {
		t.Fatal(err)
	}
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if testenv.Text(t, "SELECT number FROM invoices WHERE kind='credit_note'") != "CN-2026-00099" {
		t.Fatal("credit note did not use current numbering settings")
	}
	if len(sender.emails) != 9 || !bytes.HasPrefix(sender.emails[len(sender.emails)-1].Attachments[0].Data, []byte("%PDF")) {
		t.Fatal("numbered invoices or credit note were not submitted")
	}
}

func TestNumberingFailureDoesNotConsumeSequence(t *testing.T) {
	for _, problem := range []string{"too long", "duplicate", "case-only duplicate"} {
		t.Run(problem, func(t *testing.T) {
			setup(t)
			s := customNumberingSettings()
			if problem == "too long" {
				testenv.Exec(t, "INSERT INTO invoice_sequences(kind,financial_year,next_number) VALUES('invoice',?,100000)", FinancialYear(time.Now()))
			} else {
				enqueue(t, "already-used", sample())
				testenv.Exec(t, "UPDATE invoices SET number='2026-AMJUL-0017',status='issued',email_done=1")
				testenv.Exec(t, "INSERT INTO invoice_sequences(kind,financial_year,next_number) VALUES('invoice',?,17)", FinancialYear(time.Now()))
				if problem == "case-only duplicate" {
					s.Prefix = strings.ToLower(s.Prefix)
				}
			}
			testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", JSON(s))
			enqueue(t, "cannot-number", sample())
			before := testenv.Scalar(t, "SELECT next_number FROM invoice_sequences WHERE kind='invoice'")
			if Process() == nil {
				t.Fatal("invalid or duplicate number was issued")
			}
			if before != testenv.Scalar(t, "SELECT next_number FROM invoice_sequences WHERE kind='invoice'") || testenv.Text(t, "SELECT number FROM invoices WHERE transaction_id='cannot-number'") != "" || testenv.Text(t, "SELECT status FROM invoices WHERE transaction_id='cannot-number'") != "needs_review" {
				t.Fatal("failed numbering consumed a sequence or left no review task")
			}
		})
	}
}

func TestCustomNumberingSettingsSaveAndValidation(t *testing.T) {
	testenv.Setup(t, nil)
	adminID := testenv.SeedUser(t, 100, "admin@example.test", "test-admin-password")
	testenv.Route("POST", "/admin/invoicing", HandleSettings)
	s := customNumberingSettings()
	values := url.Values{"name": {s.Name}, "address": {s.Address}, "gstin": {s.GSTIN}, "state": {s.State}, "email": {s.Email}, "signatory": {s.Signatory}, "prefix": {s.Prefix}, "credit_prefix": {s.CreditPrefix}, "start": {"17"}, "numbering_style": {"prefix"}, "sequence_digits": {"4"}}
	w := testenv.Serve(t, multipartSettingsPost(t, values, adminID))
	if w.Code != http.StatusFound {
		t.Fatal("custom numbering settings rejected", w.Code, w.Body.String())
	}
	stored, err := LoadSettings()
	if err != nil || stored.Prefix != s.Prefix || stored.NumberingStyle != "prefix" || stored.SequenceDigits != 4 || stored.Start != 17 {
		t.Fatal("custom numbering not persisted", err)
	}
	for _, tc := range []struct{ key, value string }{{"prefix", "2026 AMJUL-"}, {"prefix", "2026-AMJUL-TOO-LONG-"}, {"credit_prefix", s.Prefix}, {"numbering_style", "unknown"}, {"sequence_digits", "0"}, {"sequence_digits", "7"}, {"sequence_digits", "99999999999999999999999"}, {"sequence_digits", "6"}, {"start", "99999999999999999999999"}} {
		input, _ := url.ParseQuery(values.Encode())
		input.Set(tc.key, tc.value)
		w := testenv.Serve(t, multipartSettingsPost(t, input, adminID))
		if w.Code != 400 {
			t.Fatal("invalid numbering settings accepted", fmt.Sprint(tc), w.Code)
		}
	}
	values.Set("numbering_style", "financial_year_prefix")
	values.Set("prefix", "AMJUL")
	values.Set("credit_prefix", "CN")
	w = testenv.Serve(t, multipartSettingsPost(t, values, adminID))
	if w.Code != http.StatusFound {
		t.Fatal("financial-year-prefix settings rejected", w.Code, w.Body.String())
	}
	stored, err = LoadSettings()
	if err != nil || stored.NumberingStyle != "financial_year_prefix" || stored.Prefix != "AMJUL" || stored.CreditPrefix != "CN" || stored.SequenceDigits != 4 || stored.Start != 17 {
		t.Fatal("financial-year-prefix settings not persisted", err)
	}
	testenv.Route("GET", "/admin/invoicing", HandleSettings)
	r := httptest.NewRequest(http.MethodGet, "/admin/invoicing", nil)
	testenv.LoggedIn(t, r, adminID)
	w = testenv.Serve(t, r)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(w.Body.String()))
	if err != nil || w.Code != 200 {
		t.Fatal("saved numbering settings did not render", err, w.Code)
	}
	selected := doc.Find("select[name='numbering_style'] option[selected]")
	if value, _ := selected.Attr("value"); selected.Length() != 1 || value != "financial_year_prefix" {
		t.Fatal("saved financial-year-prefix format not selected on reload")
	}
}
