package invoices

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
)

func TestLUTYearNormalizationAndDisplay(t *testing.T) {
	for _, tc := range []struct{ input, compact, display string }{
		{"", "", ""}, {"  ", "", ""},
		{"2627", "2627", "2026-27"}, {"2026-27", "2627", "2026-27"},
		{" \t2026-27\n", "2627", "2026-27"},
		{"0001", "0001", "2000-01"}, {"2000-01", "0001", "2000-01"},
		{"9900", "9900", "2099-00"}, {"2099-00", "9900", "2099-00"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			compact, err := normaliseLUTYear(tc.input)
			if err != nil || compact != tc.compact {
				t.Fatalf("normalised=%q, want %q: %v", compact, tc.compact, err)
			}
			s := Settings{LUTYear: tc.input}
			if display := s.LUTYearDisplay(); display != tc.display || s.LUTYear != tc.input {
				t.Fatalf("display=%q, want %q; stored year=%q", display, tc.display, s.LUTYear)
			}
		})
	}
	for _, invalid := range []string{"2026", "202627", "26-27", "2026-2027", "2026-28", "2628", "0000", "1999-00", "2100-01", "2026–27", "abcd", "２６２７", "<script>"} {
		if _, err := normaliseLUTYear(invalid); err == nil {
			t.Errorf("invalid financial year accepted: %q", invalid)
		}
		if (Settings{LUTYear: invalid}).LUTYearDisplay() != invalid {
			t.Errorf("invalid legacy year was silently changed: %q", invalid)
		}
	}
}

func TestLUTYearSettingsRoundTrip(t *testing.T) {
	testenv.Setup(t, nil)
	adminID := testenv.SeedUser(t, 100, "admin@example.test", "test-admin-password")
	testenv.Route("GET", "/admin/invoicing", HandleSettings)
	testenv.Route("POST", "/admin/invoicing", HandleSettings)
	s := seller()
	s.LUTYear = "2627"
	testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", JSON(s))
	assertDisplay := func(want string) {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/admin/invoicing", nil)
		testenv.LoggedIn(t, r, adminID)
		w := testenv.Serve(t, r)
		if w.Code != 200 {
			t.Fatalf("settings returned %d", w.Code)
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(w.Body.String()))
		if err != nil {
			t.Fatal(err)
		}
		field := doc.Find("input[name='lut_year']")
		if field.Length() != 1 || field.AttrOr("value", "missing") != want || !strings.Contains(field.Parent().Text(), "1 April 2026 to 31 March 2027") || strings.Contains(field.Parent().Text(), "e.g. 2627") {
			t.Fatal("LUT year guidance or displayed value is incorrect")
		}
		if _, required := field.Attr("required"); required {
			t.Fatal("LUT year must remain optional for domestic sellers")
		}
	}
	assertDisplay("2026-27")
	if stored, _ := LoadSettings(); stored.LUTYear != "2627" {
		t.Fatal("viewing settings rewrote the stored year")
	}
	values := url.Values{"name": {s.Name}, "address": {s.Address}, "gstin": {s.GSTIN}, "state": {s.State}, "email": {s.Email}, "signatory": {s.Signatory}, "prefix": {s.Prefix}, "credit_prefix": {s.CreditPrefix}, "start": {"1"}, "lut": {s.LUT}}
	for _, input := range []string{"2026-27", "2627", " \t2026-27\n"} {
		values.Set("lut_year", input)
		w := testenv.Serve(t, multipartSettingsPost(t, values, adminID))
		if w.Code != http.StatusFound {
			t.Fatalf("year %q rejected: %d", input, w.Code)
		}
		if stored, _ := LoadSettings(); stored.LUTYear != "2627" {
			t.Fatal("year was not saved in compact form")
		}
		assertDisplay("2026-27")
	}
	for _, invalid := range []string{"2026-28", "2628", "202627", "2026", "1999-00", "2100-01"} {
		before := testenv.Text(t, "SELECT value FROM invoice_settings WHERE id=1")
		values.Set("lut_year", invalid)
		w := testenv.Serve(t, multipartSettingsPost(t, values, adminID))
		if w.Code != 400 || testenv.Text(t, "SELECT value FROM invoice_settings WHERE id=1") != before {
			t.Fatalf("invalid year %q was accepted or changed saved settings", invalid)
		}
	}
	values.Set("lut_year", "2026-27")
	if w := testenv.Serve(t, multipartSettingsPost(t, values, 0)); w.Code != http.StatusUnauthorized {
		t.Fatal("anonymous LUT settings update allowed")
	}
	values.Set("lut", "")
	values.Set("lut_year", "")
	if w := testenv.Serve(t, multipartSettingsPost(t, values, adminID)); w.Code != http.StatusFound {
		t.Fatal("blank optional LUT fields rejected")
	}
	assertDisplay("")
}

func TestLUTYearExportEligibilityAtFinancialYearBoundary(t *testing.T) {
	for _, tc := range []struct{ paidAt, currentYear, otherYear string }{
		{"2026-03-31T18:29:59Z", "2025-26", "2026-27"},
		{"2026-03-31T18:30:00Z", "2026-27", "2025-26"},
	} {
		for _, year := range []string{tc.currentYear, tc.otherYear} {
			s := sample()
			s.Context.Country, s.Billing.Country, s.Currency = "US", "US", "USD"
			s.PaidAt = tc.paidAt
			var err error
			s.Context.Seller.LUTYear, err = normaliseLUTYear(year)
			if err != nil {
				t.Fatal(err)
			}
			err = Calculate(&s)
			if (err == nil) != (year == tc.currentYear) {
				t.Fatalf("LUT %q at %s: %v", year, tc.paidAt, err)
			}
		}
	}
}

func TestExportLUTDisplayPreservesIssuedDocumentsAndRefundYear(t *testing.T) {
	sender := setup(t)
	s := sample()
	s.Context.Country, s.Billing.Country, s.Currency = "US", "US", "USD"
	testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", JSON(s.Context.Seller))
	enqueue(t, "export-lut-year", s)
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	number := testenv.Text(t, "SELECT number FROM invoices")
	snapshot := testenv.Text(t, "SELECT snapshot FROM invoices")
	originalPDF := append([]byte(nil), sender.emails[0].Attachments[0].Data...)
	updated := s.Context.Seller
	updated.LUTYear = FinancialYear(time.Now().AddDate(1, 0, 0))
	testenv.Exec(t, "UPDATE invoice_settings SET value=?", JSON(updated))
	testenv.Exec(t, "UPDATE invoices SET email_done=0")
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if len(sender.emails) != 2 || !bytes.Equal(originalPDF, sender.emails[1].Attachments[0].Data) || testenv.Text(t, "SELECT number FROM invoices") != number || testenv.Text(t, "SELECT snapshot FROM invoices") != snapshot {
		t.Fatal("resend changed issued PDF bytes, number, or frozen LUT year")
	}
	if err := query.Transaction(func(tx *query.Tx) error {
		return QueueRefundCreditNote(tx, "razorpay", "export-lut-year", "refund-lut-year", s.Gross, s.Gross)
	}); err != nil {
		t.Fatal(err)
	}
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	var credit Snapshot
	if err := json.Unmarshal([]byte(testenv.Text(t, "SELECT snapshot FROM invoices WHERE kind='credit_note'")), &credit); err != nil {
		t.Fatal(err)
	}
	if !credit.Export || credit.Context.Seller.LUTYear != s.Context.Seller.LUTYear || credit.Context.Seller.LUTYearDisplay() != s.Context.Seller.LUTYearDisplay() || len(sender.emails) != 3 || !bytes.HasPrefix(sender.emails[2].Attachments[0].Data, []byte("%PDF-")) {
		t.Fatal("credit note lost the original export's LUT year or PDF")
	}
}
