package invoices

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
)

func signatureImages16() map[string]image.Image {
	bounds := image.Rect(0, 0, 3, 1)
	gray := image.NewGray16(bounds)
	gray.SetGray16(0, 0, color.Gray16{Y: 0x1234})
	gray.SetGray16(1, 0, color.Gray16{Y: 0x8000})
	gray.SetGray16(2, 0, color.Gray16{Y: 0xffff})
	rgba := image.NewRGBA64(bounds)
	rgba.SetRGBA64(1, 0, color.RGBA64{R: 0x1000, G: 0x2000, B: 0x3000, A: 0x8000})
	rgba.SetRGBA64(2, 0, color.RGBA64{R: 0x1234, G: 0x5678, B: 0xabcd, A: 0xffff})
	nrgba := image.NewNRGBA64(bounds)
	nrgba.SetNRGBA64(1, 0, color.NRGBA64{R: 0x1234, G: 0x5678, B: 0xabcd, A: 0x8000})
	nrgba.SetNRGBA64(2, 0, color.NRGBA64{R: 0x1234, G: 0x5678, B: 0xabcd, A: 0xffff})
	return map[string]image.Image{"gray": gray, "rgba": rgba, "nrgba": nrgba}
}

func signaturePNG16(t *testing.T, im image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	if b.Bytes()[24] != 16 {
		t.Fatal("fixture must be a 16-bit PNG")
	}
	return b.Bytes()
}

func TestNormaliseSignatureConverts16BitPNG(t *testing.T) {
	for name, im := range signatureImages16() {
		t.Run(name, func(t *testing.T) {
			data, err := NormaliseSignature(signaturePNG16(t, im))
			if err != nil {
				t.Fatal(err)
			}
			if data[24] > 8 {
				t.Fatal("signature still has a bit depth unsupported by the PDF renderer")
			}
			got, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if got.Bounds() != im.Bounds() {
				t.Fatal("signature dimensions changed")
			}
			for x := 0; x < im.Bounds().Dx(); x++ {
				r, g, b, a := im.At(x, 0).RGBA()
				gr, gg, gb, ga := got.At(x, 0).RGBA()
				for i, pair := range [][2]uint32{{r, gr}, {g, gg}, {b, gb}, {a, ga}} {
					delta := int64(pair[0]) - int64(pair[1])
					if delta < -257 || delta > 257 {
						t.Fatalf("pixel %d channel %d changed beyond 8-bit rounding: %v", x, i, pair)
					}
				}
			}
		})
	}
}

func TestRenderDocumentsWithLegacy16BitSignature(t *testing.T) {
	for name, im := range signatureImages16() {
		for _, kind := range []string{"invoice", "credit_note"} {
			t.Run(name+"/"+kind, func(t *testing.T) {
				s := sample()
				s.Context.Seller.Signature = signaturePNG16(t, im)
				if err := Calculate(&s); err != nil {
					t.Fatal(err)
				}
				pdf, err := renderPDF(s, "2026-AM-0017", kind, "2026-11-15")
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
					t.Fatal("missing PDF")
				}
			})
		}
	}
}

func TestReservedInvoiceWith16BitSignatureResumes(t *testing.T) {
	sender := setup(t)
	s := sample()
	s.Context.Seller.Signature = signaturePNG16(t, signatureImages16()["nrgba"])
	s.PaymentReference = "pay_signature"
	if err := Calculate(&s); err != nil {
		t.Fatal(err)
	}
	enqueue(t, "pay_signature", s)
	// Model a persisted rendering failure after number reservation, as produced
	// by older releases. Recovery must use the frozen snapshot and number.
	number, snapshot := "2026-AM-0017", JSON(s)
	testenv.Exec(t, "UPDATE invoices SET number=?,issued_at=?,snapshot=?,attempts=6", number, "2026-11-15", snapshot)
	testenv.Exec(t, "INSERT INTO invoice_sequences(kind,financial_year,next_number) VALUES('invoice',?,18)", FinancialYear(time.Now()))
	if err := Process(); err != nil {
		t.Fatal(err)
	}
	if len(sender.emails) != 1 || len(sender.emails[0].Attachments) != 1 || !bytes.HasPrefix(sender.emails[0].Attachments[0].Data, []byte("%PDF-")) {
		t.Fatal("reserved invoice was not submitted with its PDF")
	}
	if testenv.Text(t, "SELECT number FROM invoices") != number || testenv.Text(t, "SELECT snapshot FROM invoices") != snapshot {
		t.Fatal("recovery altered the reserved document identity")
	}
	if testenv.Text(t, "SELECT status FROM invoices") != "issued" || testenv.Scalar(t, "SELECT email_done FROM invoices") != 1 {
		t.Fatal("recovery did not finish the queued document")
	}
	if testenv.Scalar(t, "SELECT next_number FROM invoice_sequences") != 18 {
		t.Fatal("recovery consumed another invoice number")
	}
}
