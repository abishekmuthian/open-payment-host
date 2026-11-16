package invoices

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/signintech/gopdf"
)

// Page dictionaries are uncompressed in gopdf output. Count /Page, excluding
// /Pages, without requiring external PDF tools in the Go regression suite.
func pdfPageCount(pdf []byte) int {
	return len(regexp.MustCompile(`/Type\s*/Page\b`).FindAll(pdf, -1))
}

func signatureLayoutSnapshot(t *testing.T, width, height int) Snapshot {
	t.Helper()
	s := sample()
	s.Context.Seller.Address = "1 Example Road\nCoimbatore, Tamil Nadu"
	s.Context.Seller.Signatory = "Example Seller"
	s.Billing.Business = "Example Company"
	s.Billing.GSTIN = "33AAAAA0000A1Z9"
	s.OriginalNumber, s.OriginalDate = "2026-AM-0022", "2026-11-15"
	s.RefundReference, s.PaymentReference = "refund_example_123456", "pay_example_123456"
	if width > 0 {
		im := image.NewNRGBA(image.Rect(0, 0, width, height))
		for x := width / 10; x < width*9/10; x++ {
			for y := height / 3; y < height/3+max(1, height/20); y++ {
				im.Set(x, y, color.NRGBA{R: 25, G: 25, B: 25, A: 255})
			}
		}
		var data bytes.Buffer
		if err := png.Encode(&data, im); err != nil {
			t.Fatal(err)
		}
		s.Context.Seller.Signature = data.Bytes()
	}
	if err := Calculate(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCreditNoteSignatureStaysOnFirstPageWhenItFits(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
	}{{"wide", 300, 100}, {"tall", 100, 300}, {"no signature", 0, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			s := signatureLayoutSnapshot(t, tc.width, tc.height)
			pdf, err := renderPDF(s, "2026-CN-AM-0019", "credit_note", "2026-11-16")
			if err != nil {
				t.Fatal(err)
			}
			if pages := pdfPageCount(pdf); pages != 1 {
				t.Fatalf("ordinary credit note has %d pages; signature and caption fit on one page", pages)
			}
		})
	}
}

func TestSignatureCaptionPagination(t *testing.T) {
	for _, kind := range []string{"invoice", "credit_note"} {
		for _, mode := range []string{"domestic", "interstate", "export"} {
			for _, tc := range []struct {
				name, signatory             string
				addressLines, width, height int
				pages, exportCreditPages    int
			}{
				{"ordinary", "Example Seller", 0, 300, 100, 1, 1},
				{"tall signature", "Example Seller", 0, 100, 300, 1, 2},
				{"no signature", "Example Seller", 0, 0, 0, 1, 1},
				// The export/LUT declaration leaves insufficient room for this
				// wrapped caption on a credit note, so it must move as a unit.
				{"wrapped name", strings.Repeat("Alexandria Example ", 6), 0, 300, 100, 1, 2},
				{"long details", strings.Repeat("Registered signatory name ", 14), 8, 300, 100, 2, 2},
			} {
				t.Run(kind+"/"+mode+"/"+tc.name, func(t *testing.T) {
					s := signatureLayoutSnapshot(t, tc.width, tc.height)
					s.Context.Seller.Signatory = tc.signatory
					s.Billing.Address += strings.Repeat("\nAdditional address details", tc.addressLines)
					if kind == "invoice" {
						s.OriginalNumber, s.OriginalDate, s.RefundReference = "", "", ""
					}
					s.CGST, s.SGST, s.IGST = 0, 0, 0
					if mode == "interstate" {
						s.Billing.State, s.Billing.GSTIN = "KA", ""
					}
					if mode == "export" {
						s.Context.Country, s.Billing.Country, s.Currency = "US", "US", "USD"
						s.Billing.State, s.Billing.PostalCode, s.Billing.GSTIN = "MA", "02420", ""
					}
					if err := Calculate(&s); err != nil {
						t.Fatal(err)
					}
					pdf, err := renderPDF(s, "2026-AM-0022", kind, "2026-11-16")
					if err != nil {
						t.Fatal(err)
					}
					want := tc.pages
					if kind == "credit_note" && mode == "export" {
						want = tc.exportCreditPages
					}
					if pages := pdfPageCount(pdf); pages != want {
						t.Fatalf("pages=%d, want %d", pages, want)
					}
				})
			}
		}
	}
}

func TestUnrenderableSignatureCaptionFailsWithoutTruncation(t *testing.T) {
	s := signatureLayoutSnapshot(t, 300, 100)
	s.Context.Seller.Signatory = strings.Repeat("Signatory\n", 60)
	if _, err := renderPDF(s, "2026-AM-0022", "credit_note", "2026-11-16"); err == nil || !strings.Contains(err.Error(), "signatory details exceed one PDF page") {
		t.Fatal("oversized signature block was split or truncated instead of rejected", err)
	}
}

func TestIssuedMultipageCreditNoteIsNotRegenerated(t *testing.T) {
	sender := setup(t)
	enqueue(t, "paid-signature-layout", sample())
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if err := query.Transaction(func(tx *query.Tx) error {
		return QueueRefundCreditNote(tx, "razorpay", "paid-signature-layout", "refund-signature-layout", 3300, 3300)
	}); err != nil {
		t.Fatal(err)
	}
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	// Model a previously issued two-page PDF. Downloads and resends must use
	// these exact bytes even though the new renderer could produce one page.
	legacy := &gopdf.GoPdf{}
	legacy.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	legacy.AddPage()
	legacy.AddPage()
	issuedPDF, err := legacy.GetBytesPdfReturnErr()
	if err != nil || pdfPageCount(issuedPDF) != 2 {
		t.Fatal("invalid legacy PDF fixture", err)
	}
	id := testenv.Text(t, "SELECT id FROM invoices WHERE kind='credit_note'")
	snapshot := testenv.Text(t, "SELECT snapshot FROM invoices WHERE id=?", id)
	number := testenv.Text(t, "SELECT number FROM invoices WHERE id=?", id)
	testenv.Exec(t, "UPDATE invoices SET pdf=?,email_done=0 WHERE id=?", issuedPDF, id)
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if len(sender.emails) != 3 || !bytes.Equal(sender.emails[2].Attachments[0].Data, issuedPDF) || testenv.Text(t, "SELECT snapshot FROM invoices WHERE id=?", id) != snapshot || testenv.Text(t, "SELECT number FROM invoices WHERE id=?", id) != number {
		t.Fatal("resend regenerated an issued credit note or changed its identity")
	}
	w := httptest.NewRecorder()
	if err := Download(w, id, ""); err != nil || !bytes.Equal(w.Body.Bytes(), issuedPDF) {
		t.Fatal("download changed the issued PDF", err)
	}
}
