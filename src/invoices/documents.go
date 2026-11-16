package invoices

import (
	"bytes"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/mail"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/resource"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/google/uuid"
	"github.com/signintech/gopdf"
)

//go:embed fonts/DejaVuSans.ttf
var font []byte

func QueuePaidInvoice(tx *query.Tx, attempt, gateway, transaction, context, billing, currency string, amount int64, paidAt time.Time, nativeTax ...*int64) error {
	if context == "" {
		return nil
	}
	s := Snapshot{Currency: strings.ToUpper(currency), Gross: amount, PaymentReference: transaction, PaidAt: paidAt.UTC().Format(time.RFC3339)}
	if err := json.Unmarshal([]byte(context), &s.Context); err != nil {
		return err
	}
	if len(nativeTax) > 0 {
		s.NativeTax = nativeTax[0]
	}
	if billing != "" {
		if err := json.Unmarshal([]byte(billing), &s.Billing); err != nil {
			return err
		}
	}
	return queue(tx, "invoice", attempt, gateway, transaction, "", "", s)
}
func queue(tx *query.Tx, kind, attempt, gateway, transaction, refund, original string, s Snapshot) error {
	_, err := tx.Exec(`INSERT INTO invoices(id,kind,attempt_id,gateway,transaction_id,refund_id,original_id,created_at,snapshot) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(kind,gateway,transaction_id,refund_id) DO NOTHING`, uuid.NewString(), kind, attempt, gateway, transaction, refund, original, time.Now().UTC().Format(time.RFC3339Nano), JSON(s))
	return err
}

// QueueRefundCreditNote allocates tax from the original immutable invoice. The
// cumulative target method makes many partial refunds sum exactly to its totals.
func QueueRefundCreditNote(tx *query.Tx, gateway, transaction, refund string, amount, cumulative int64) error {
	var id, attempt, body string
	err := tx.QueryRow("SELECT id,attempt_id,snapshot FROM invoices WHERE kind='invoice' AND gateway=? AND transaction_id=?", gateway, transaction).Scan(&id, &attempt, &body)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var s Snapshot
	if err = json.Unmarshal([]byte(body), &s); err != nil {
		return err
	}
	s.RefundAmount = amount
	s.RefundReference = refund
	// Calculation is deferred until the original document has been issued.
	s.Gross = cumulative
	return queue(tx, "credit_note", attempt, gateway, transaction, refund, id, s)
}

func renderPDF(s Snapshot, number, kind, issued string) ([]byte, error) {
	p := &gopdf.GoPdf{}
	p.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	if err := p.AddTTFFontData("invoice", font); err != nil {
		return nil, err
	}
	var drawErr error
	newPage := func() {
		p.AddPage()
		_ = p.SetFont("invoice", "", 9)
		p.SetTextColor(90, 90, 90)
		p.SetXY(42, 20)
		if e := p.Cell(&gopdf.Rect{W: 510, H: 14}, number+"  |  "+kind); e != nil {
			drawErr = errors.Join(drawErr, e)
		}
		p.SetTextColor(25, 25, 25)
	}
	newPage()
	// Wrap explicitly, including long unbroken identifiers, so no customer text
	// is silently truncated by a fixed-height PDF cell.
	text := func(x, y, width float64, value string, size int) float64 {
		if drawErr != nil {
			return y
		}
		lines, e := wrappedPDFLines(p, width, value, size)
		if e != nil {
			drawErr = e
			return y
		}
		for _, line := range lines {
			if y > pdfLastLineY {
				newPage()
				y = pdfPageTextTop
				if e := p.SetFont("invoice", "", size); e != nil {
					drawErr = e
					return y
				}
			}
			p.SetXY(x, y)
			if e := p.Cell(&gopdf.Rect{W: width, H: float64(size) + 5}, line); e != nil {
				drawErr = e
			}
			y += float64(size) + 5
		}
		return y
	}
	title := "Original Tax Invoice"
	status := "Paid"
	if kind == "credit_note" {
		title = "Credit Note"
		status = "Refund processed"
	}
	titleY := text(42, 53, 510, s.Context.Seller.Name, 20) + 5
	metaY := text(42, titleY, 510, title, 13) + 22
	// Draw the short right column before the wrapping left column.
	_ = text(350, metaY, 200, "Invoice: "+number+"\nDate: "+issued+"\n"+status, 10)
	y := text(42, metaY, 285, s.Context.Seller.Name+"\n"+s.Context.Seller.Address+"\n"+s.Context.Seller.Email+"\nGSTIN: "+s.Context.Seller.GSTIN, 10) + 22
	if y > 600 {
		newPage()
		y = 48
	}
	place := s.Billing.State + ", " + CountryName(s.Billing.Country)
	if s.Billing.Country == "IN" {
		place = StateName(s.Billing.State) + " (" + StateCode(s.Billing.State) + ")"
	}
	_ = text(350, y, 200, "Place of supply: "+place, 10)
	bill := "Bill to:\n" + s.Billing.Name
	if s.Billing.Business != "" {
		bill += "\n" + s.Billing.Business
	}
	state := s.Billing.State
	if s.Billing.Country == "IN" {
		state = StateName(state)
	}
	bill += "\n" + s.Billing.Address + "\n" + s.Billing.City + ", " + state + " " + s.Billing.PostalCode + "\n" + CountryName(s.Billing.Country) + "\n" + s.Billing.Email
	if s.Billing.GSTIN != "" {
		bill += "\nGSTIN: " + s.Billing.GSTIN
	}
	y = text(42, y, 285, bill, 10) + 25
	if s.OriginalNumber != "" {
		y = text(42, y, 510, "Against invoice: "+s.OriginalNumber+" dated "+s.OriginalDate+"\nRefund reference: "+s.RefundReference+"\nReason: Payment refund", 10) + 12
	}
	if y > 610 {
		newPage()
		y = 48
	}
	p.Line(42, y, 550, y)
	y += 10
	_ = text(42, y, 260, "Description", 11)
	_ = text(320, y, 90, "HSN/SAC", 11)
	_ = text(420, y, 130, "Amount ("+s.Currency+")", 11)
	y += 25
	amountY := text(440, y, 110, Money(s.Net, s.Currency), 11)
	codeY := text(320, y, 90, s.Context.Product.Code, 11)
	descY := text(42, y, 260, s.Context.descriptionDisplay(), 11)
	y = max(descY, codeY, amountY) + 18
	p.Line(42, y, 550, y)
	y += 15
	total := func(label string, amount int64) {
		if y > 720 {
			newPage()
			y = 48
		}
		_ = text(300, y, 140, label, 11)
		_ = text(440, y, 110, Money(amount, s.Currency), 11)
		y += 24
	}
	total("Taxable value", s.Net)
	if s.Export {
		total("IGST (0%)", 0)
	} else if s.IGST != 0 {
		total("IGST ("+s.Context.Product.Rate+"%)", s.IGST)
	} else {
		rate, _ := new(big.Rat).SetString(s.Context.Product.Rate)
		half := new(big.Rat).Quo(rate, big.NewRat(2, 1)).FloatString(2)
		total("CGST ("+half+"%)", s.CGST)
		label := "SGST"
		switch StateCode(s.Billing.State) {
		case "04", "26", "31", "35", "38":
			label = "UTGST"
		}
		total(label+" ("+half+"%)", s.SGST)
	}
	total("Total "+s.Currency, s.Gross)
	if s.Export {
		y += 10
		y = text(42, y, 510, "SUPPLY MEANT FOR EXPORT UNDER LETTER OF UNDERTAKING WITHOUT PAYMENT OF INTEGRATED TAX\nLUT ARN: "+s.Context.Seller.LUT+"  |  Financial year: "+s.Context.Seller.LUTYearDisplay(), 9) + 12
	}
	y = text(42, y+10, 510, "Reverse charge: No\nPayment reference: "+s.PaymentReference+"\nPayment date: "+s.PaidAt, 9) + 20
	if drawErr != nil {
		return nil, drawErr
	}
	caption := s.Context.Seller.Signatory + "\nAuthorised Signatory"
	captionLines, err := wrappedPDFLines(p, 510, caption, 10)
	if err != nil {
		return nil, err
	}
	var signatureImage gopdf.ImageHolder
	var signatureWidth, signatureHeight float64
	if len(s.Context.Seller.Signature) > 0 {
		// Older snapshots can contain 16-bit PNGs. Convert only the rendering
		// copy so pending documents recover without changing their frozen data.
		signature, e := NormaliseSignature(s.Context.Seller.Signature)
		if e != nil {
			return nil, fmt.Errorf("prepare signature for PDF: %w", e)
		}
		signatureImage, e = gopdf.ImageHolderByBytes(signature)
		if e != nil {
			return nil, e
		}
		cfg, _, e := image.DecodeConfig(bytes.NewReader(signature))
		if e != nil {
			return nil, e
		}
		signatureWidth, signatureHeight = 130, 130*float64(cfg.Height)/float64(cfg.Width)
		if signatureHeight > 55 {
			signatureHeight = 55
			signatureWidth = signatureHeight * float64(cfg.Width) / float64(cfg.Height)
		}
	}
	captionOffset := 0.0
	if signatureImage != nil {
		captionOffset = signatureHeight + 10
	}
	lastLineOffset := captionOffset + float64(len(captionLines)-1)*15
	if pdfPageTextTop+lastLineOffset > pdfLastLineY {
		return nil, errors.New("signature and signatory details exceed one PDF page")
	}
	if y+lastLineOffset > pdfLastLineY {
		newPage()
		y = pdfPageTextTop
	}
	if signatureImage != nil {
		if err := p.ImageByHolder(signatureImage, 42, y, &gopdf.Rect{W: signatureWidth, H: signatureHeight}); err != nil {
			return nil, err
		}
		y += captionOffset
	}
	_ = text(42, y, 510, caption, 10)
	if drawErr != nil {
		return nil, drawErr
	}
	return p.GetBytesPdfReturnErr()
}

var workerOnce sync.Once

func Start() {
	workerOnce.Do(func() {
		go func() {
			for {
				if err := Process(); err != nil {
					log.Error(log.V{"invoice delivery": err})
				}
				time.Sleep(30 * time.Second)
			}
		}()
	})
}

// Process resumes persisted work. A lease protects against concurrent processes;
// issued bytes and numbers are persisted before email submission, which is at least once.
func Process() error {
	if _, err := query.Exec("DELETE FROM invoice_billing WHERE expires_at<?", time.Now().Unix()); err != nil {
		return err
	}
	rows, err := query.New("invoices", "id").Select("SELECT id FROM invoices").Where("status='pending' AND pdf IS NULL AND next_attempt_at<=? AND lease_until<?", time.Now().Unix(), time.Now().Unix()).Order("created_at,id").Limit(100).Results()
	if err != nil {
		return err
	}
	var errs []error
	// Generate the batch before submitting email, so a failed mail service cannot delay
	// availability of these PDFs. Claims and persistence remain per document.
	for _, r := range rows {
		if err := process(resource.ValidateString(r["id"]), false); err != nil {
			errs = append(errs, err)
		}
	}
	// Keep email queued while disabled. PDFs remain available, and enabling the
	// switch later resumes submission without consuming retries in the meantime.
	enabled, err := config.GetBoolStrict("automated_invoice")
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	if !enabled {
		return errors.Join(errs...)
	}
	rows, err = query.New("invoices", "id").Select("SELECT id FROM invoices").Where("status='issued' AND pdf IS NOT NULL AND email_done=0 AND next_attempt_at<=? AND lease_until<?", time.Now().Unix(), time.Now().Unix()).Order("created_at,id").Limit(8).Results()
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	deliveryErrors := make([]error, len(rows))
	var wg sync.WaitGroup
	for i, r := range rows {
		wg.Add(1)
		go func(i int, id string) { defer wg.Done(); deliveryErrors[i] = process(id, true) }(i, resource.ValidateString(r["id"]))
	}
	wg.Wait()
	return errors.Join(append(errs, deliveryErrors...)...)
}

func process(id string, send bool) error {
	claimed := false
	err := query.Transaction(func(tx *query.Tx) error {
		res, e := tx.Exec("UPDATE invoices SET lease_until=? WHERE id=? AND lease_until<? AND email_done=0 AND status IN ('pending','issued') AND next_attempt_at<=?", time.Now().Add(5*time.Minute).Unix(), id, time.Now().Unix(), time.Now().Unix())
		if e != nil {
			return e
		}
		n, e := res.RowsAffected()
		claimed = n == 1
		return e
	})
	if err != nil || !claimed {
		return err
	}
	var s Snapshot
	var body, number, kind, status, original, issued string
	var pdf []byte
	var attempts int
	err = query.Transaction(func(tx *query.Tx) error {
		return tx.QueryRow("SELECT snapshot,number,kind,status,original_id,issued_at,pdf,attempts FROM invoices WHERE id=?", id).Scan(&body, &number, &kind, &status, &original, &issued, &pdf, &attempts)
	})
	if err == nil {
		err = json.Unmarshal([]byte(body), &s)
	}
	currentSeller, sellerErr := LoadSettings()
	if err == nil && number == "" {
		err = query.Transaction(func(tx *query.Tx) error {
			// Serialize number allocation with SQLite writers before reading counters.
			if _, e := tx.Exec("UPDATE invoices SET id=id WHERE id=?", id); e != nil {
				return e
			}
			if sellerErr != nil && !errors.Is(sellerErr, sql.ErrNoRows) {
				return sellerErr
			}
			if kind == "invoice" {
				if e := Calculate(&s); e != nil {
					return e
				}
			} else {
				var b, origNum, origIssued string
				if e := tx.QueryRow("SELECT snapshot,number,issued_at FROM invoices WHERE id=?", original).Scan(&b, &origNum, &origIssued); e != nil {
					return e
				}
				if origNum == "" {
					return errors.New("original invoice is not issued")
				}
				var orig Snapshot
				if e := json.Unmarshal([]byte(b), &orig); e != nil {
					return e
				}
				cumulative, refund := s.Gross, s.RefundAmount
				if cumulative > orig.Gross || refund <= 0 || cumulative < refund {
					return errors.New("invalid refund allocation")
				}
				allocate := func(v int64) int64 {
					a, _ := round(new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(v), big.NewInt(cumulative)), big.NewInt(orig.Gross)))
					b, _ := round(new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(v), big.NewInt(cumulative-refund)), big.NewInt(orig.Gross)))
					return a - b
				}
				refundRef := s.RefundReference
				s = orig
				s.RefundReference = refundRef
				s.OriginalDate = origIssued
				if sellerErr == nil && currentSeller.GSTIN == orig.Context.Seller.GSTIN {
					s.Context.Seller.Signature = currentSeller.Signature
					s.Context.Seller.Signatory = currentSeller.Signatory
					s.Context.Seller.CreditPrefix = currentSeller.CreditPrefix
					s.Context.Seller.Start = currentSeller.Start
				}
				s.NativeTax = nil
				s.OriginalNumber = origNum
				s.RefundAmount = refund
				s.Gross = refund
				s.CGST = allocate(orig.CGST)
				s.SGST = allocate(orig.SGST)
				s.IGST = allocate(orig.IGST)
				s.Net = refund - s.CGST - s.SGST - s.IGST
			}
			// Resolve numbering when reserving a number, including renewals with an
			// older checkout snapshot. Issued/reserved numbers never enter this path.
			if sellerErr == nil && currentSeller.GSTIN == s.Context.Seller.GSTIN {
				s.Context.Seller.Prefix = currentSeller.Prefix
				s.Context.Seller.CreditPrefix = currentSeller.CreditPrefix
				s.Context.Seller.NumberingStyle = currentSeller.NumberingStyle
				s.Context.Seller.SequenceDigits = currentSeller.SequenceDigits
				s.Context.Seller.Start = currentSeller.Start
			}
			now := time.Now()
			fy := FinancialYear(now)
			if _, e := tx.Exec("INSERT INTO invoice_sequences(kind,financial_year,next_number) VALUES(?,?,?) ON CONFLICT(kind,financial_year) DO NOTHING", kind, fy, s.Context.Seller.Start); e != nil {
				return e
			}
			var seq int64
			if e := tx.QueryRow("SELECT next_number FROM invoice_sequences WHERE kind=? AND financial_year=?", kind, fy).Scan(&seq); e != nil {
				return e
			}
			// A migration from externally issued invoices can advance the counter.
			// Lowering the configured floor can never rewind an existing series.
			if seq < s.Context.Seller.Start {
				seq = s.Context.Seller.Start
			}
			nextNumber, err := s.Context.Seller.documentNumber(kind, seq, now)
			if err != nil {
				return err
			}
			var duplicates int
			if err := tx.QueryRow("SELECT COUNT(*) FROM invoices WHERE LOWER(number)=LOWER(?) AND id<>?", nextNumber, id).Scan(&duplicates); err != nil {
				return err
			}
			if duplicates != 0 {
				return errors.New("invoice number is already reserved; choose a distinct prefix or advance the minimum next sequence")
			}
			number = nextNumber
			issued = now.In(india).Format("2006-01-02")
			if _, e := tx.Exec("UPDATE invoice_sequences SET next_number=? WHERE kind=? AND financial_year=?", seq+1, kind, fy); e != nil {
				return e
			}
			_, e := tx.Exec("UPDATE invoices SET number=?,issued_at=?,snapshot=? WHERE id=?", number, issued, JSON(s), id)
			return e
		})
		if err != nil {
			_, updateErr := query.Exec("UPDATE invoices SET status='needs_review',last_error=?,lease_until=0 WHERE id=?", err.Error(), id)
			return errors.Join(err, updateErr)
		}
	}
	if err == nil && len(pdf) == 0 {
		pdf, err = renderPDF(s, number, kind, issued)
		if err == nil {
			_, err = query.Exec("UPDATE invoices SET pdf=?,status='issued' WHERE id=?", pdf, id)
		}
	}
	if err == nil && !send {
		_, err = query.Exec("UPDATE invoices SET lease_until=0,last_error='' WHERE id=?", id)
		return err
	}
	if err == nil {
		e := mail.New(s.Billing.Email)
		e.ReplyTo = s.Context.Seller.Email
		e.Subject = number + " — " + s.Context.Seller.Name
		e.Body = fmt.Sprintf("Your invoice %s for %s from %s is attached as a PDF.\n\nThank you for your purchase!", number, s.Context.descriptionDisplay(), s.Context.Seller.Name)
		if kind == "credit_note" {
			e.Body = fmt.Sprintf("Your credit note %s for %s against invoice %s from %s is attached as a PDF.", number, s.Context.descriptionDisplay(), s.OriginalNumber, s.Context.Seller.Name)
			e.Body += "\n\nYour refund has been processed and the amount will be credited back to your account within 5-7 working days."
			e.Body += "\n\nSorry you didn't like my product. Let me know how I can make it better by replying to this email."
		} else {
			e.Body += "\n\nIf you need any help with the product, please reply to this email any time."
		}
		e.MessageID = id + "@open-payment-host"
		e.Attachments = []mail.Attachment{{Name: numberToFilename(number), ContentType: "application/pdf", Data: pdf}}
		err = sendInvoiceMail(e, s, number, kind)
	}
	if err == nil {
		_, err = query.Exec("UPDATE invoices SET email_done=1,lease_until=0,last_error='' WHERE id=?", id)
		return err
	}
	attempts++
	delay := 30 * time.Second
	for i := 1; i < attempts && delay < time.Hour; i++ {
		delay *= 2
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	if attempts >= 20 {
		status = "delivery_failed"
	} else if len(pdf) > 0 {
		status = "issued"
	} else {
		status = "pending"
	}
	_, e := query.Exec("UPDATE invoices SET lease_until=0,attempts=?,next_attempt_at=?,last_error=?,status=? WHERE id=?", attempts, time.Now().Add(delay).Unix(), "Document generation or email delivery failed; check server configuration", status, id)
	return errors.Join(err, e)
}
func numberToFilename(n string) string {
	out := []rune(n)
	for i, c := range out {
		if c == '/' {
			out[i] = '-'
		}
	}
	return string(out) + ".pdf"
}
