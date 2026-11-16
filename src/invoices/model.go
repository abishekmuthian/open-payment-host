// Package invoices implements Indian GST documents independently of payment providers.
package invoices

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"math/big"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/query"
)

type Settings struct {
	DeliveryProvider                                string
	ListmonkInvoiceTemplate, ListmonkCreditTemplate int
	ListmonkMessenger, ListmonkFrom                 string
	Name, Address, GSTIN, State, Email, Signatory   string
	Prefix, CreditPrefix                            string
	NumberingStyle                                  string
	SequenceDigits                                  int
	Start                                           int64
	LUT, LUTYear                                    string
	Signature                                       []byte
}
type Product struct {
	Enabled           bool
	Code, Rate        string
	Inclusive, Export bool
}
type Billing struct {
	Name, Business, Email, Address, City, State, PostalCode, Country, GSTIN string
}
type Context struct {
	ReviewReason string
	Seller       Settings
	Product      Product
	Description  string
	// Base is the expected net amount in exclusive mode, or gross amount in inclusive mode.
	Base    int64
	Country string
}

// descriptionDisplay matches the invoice line item, including older snapshots
// that still contain the product's trailing hashtags.
func (c Context) descriptionDisplay() string {
	description, _, _ := strings.Cut(c.Description, "#")
	return strings.TrimSpace(description)
}

type Snapshot struct {
	OriginalDate, RefundReference                      string
	NativeTax                                          *int64
	Context                                            Context
	Billing                                            Billing
	Currency, PaymentReference, PaidAt, OriginalNumber string
	Gross, Net, CGST, SGST, IGST                       int64
	Export                                             bool
	RefundAmount                                       int64
}

var rateRE = regexp.MustCompile(`^(0|[1-9][0-9]{0,2})(\.[0-9]{1,2})?$`)
var codeRE = regexp.MustCompile(`^[0-9]{4}([0-9]{2})?([0-9]{2})?$`)
var gstRE = regexp.MustCompile(`^[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z][1-9A-Z]Z[0-9A-Z]$`)
var india = time.FixedZone("Asia/Kolkata", 19800)

// StateCode accepts names, postal abbreviations and GST state codes.
func StateCode(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	for _, row := range strings.Split("01:JK:JAMMU AND KASHMIR|02:HP:HIMACHAL PRADESH|03:PB:PUNJAB|04:CH:CHANDIGARH|05:UT:UTTARAKHAND|06:HR:HARYANA|07:DL:DELHI|08:RJ:RAJASTHAN|09:UP:UTTAR PRADESH|10:BR:BIHAR|11:SK:SIKKIM|12:AR:ARUNACHAL PRADESH|13:NL:NAGALAND|14:MN:MANIPUR|15:MZ:MIZORAM|16:TR:TRIPURA|17:ML:MEGHALAYA|18:AS:ASSAM|19:WB:WEST BENGAL|20:JH:JHARKHAND|21:OR:ODISHA|22:CT:CHHATTISGARH|23:MP:MADHYA PRADESH|24:GJ:GUJARAT|26:DN:DADRA AND NAGAR HAVELI AND DAMAN AND DIU|27:MH:MAHARASHTRA|29:KA:KARNATAKA|30:GA:GOA|31:LD:LAKSHADWEEP|32:KL:KERALA|33:TN:TAMIL NADU|34:PY:PUDUCHERRY|35:AN:ANDAMAN AND NICOBAR ISLANDS|36:TG:TELANGANA|37:AP:ANDHRA PRADESH|38:LA:LADAKH", "|") {
		p := strings.Split(row, ":")
		if v == p[0] || v == p[1] || v == p[2] {
			return p[0]
		}
	}
	if v == "UK" {
		return "05"
	}
	if v == "OD" {
		return "21"
	}
	if v == "CG" {
		return "22"
	}
	if v == "TS" {
		return "36"
	}
	return ""
}
func ValidGSTIN(v string) bool {
	if !gstRE.MatchString(v) || StateCode(v[:2]) == "" {
		return false
	}
	const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	sum, factor := 0, 2
	for i := 13; i >= 0; i-- {
		n := strings.IndexByte(chars, v[i]) * factor
		sum += n/36 + n%36
		factor = 3 - factor
	}
	return v[14] == chars[(36-sum%36)%36]
}
func ValidateProduct(p Product) error {
	if !p.Enabled {
		return nil
	}
	if !codeRE.MatchString(p.Code) {
		return errors.New("HSN/SAC must contain 4, 6 or 8 digits")
	}
	r, ok := new(big.Rat).SetString(p.Rate)
	if !rateRE.MatchString(p.Rate) || !ok || r.Sign() < 0 || r.Cmp(big.NewRat(100, 1)) > 0 {
		return errors.New("GST rate must be between 0 and 100")
	}
	return nil
}
func (s Settings) Validate() error {
	if s.Name == "" || s.Address == "" || s.Signatory == "" || !ValidGSTIN(s.GSTIN) || StateCode(s.State) != s.GSTIN[:2] {
		return errors.New("set seller name, address, signatory, valid GSTIN and matching state")
	}
	if _, err := mail.ParseAddress(s.Email); err != nil {
		return errors.New("invalid seller email")
	}
	for _, kind := range []string{"invoice", "credit_note"} {
		if _, err := s.documentNumber(kind, s.Start, time.Now()); err != nil {
			return err
		}
	}
	return nil
}
func (b *Billing) Validate() error {
	for _, field := range []*string{&b.Name, &b.Business, &b.Email, &b.Address, &b.City, &b.State, &b.PostalCode, &b.Country, &b.GSTIN} {
		*field = strings.TrimSpace(*field)
	}
	b.Country = strings.ToUpper(strings.TrimSpace(b.Country))
	b.GSTIN = strings.ToUpper(strings.TrimSpace(b.GSTIN))
	if countryNames[b.Country] == "" || b.Name == "" || b.Address == "" || b.City == "" || b.PostalCode == "" {
		return errors.New("complete billing name, address, city, country and postcode")
	}
	for _, v := range []string{b.Name, b.Business, b.Email, b.Address, b.City, b.State, b.PostalCode, b.GSTIN} {
		if len(v) > 1000 {
			return errors.New("billing field too long")
		}
	}
	if a, err := mail.ParseAddress(b.Email); err != nil || a.Address != b.Email {
		return errors.New("invalid billing email")
	}
	if b.Country == "IN" {
		state := StateCode(b.State)
		if state == "" || !regexp.MustCompile(`^[1-9][0-9]{5}$`).MatchString(b.PostalCode) {
			return errors.New("select a valid Indian state and six-digit pincode")
		}
		if b.GSTIN != "" && (!ValidGSTIN(b.GSTIN) || b.GSTIN[:2] != state) {
			return errors.New("GSTIN is invalid or does not match billing state")
		}
	} else if b.GSTIN != "" {
		return errors.New("Indian GSTIN requires an Indian billing address")
	}
	return nil
}
func NormaliseSignature(data []byte) ([]byte, error) {
	if len(data) > 1<<20 {
		return nil, errors.New("signature must be at most 1 MB")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") || cfg.Width > 2048 || cfg.Height > 2048 || cfg.Width < 1 || cfg.Height < 1 {
		return nil, errors.New("signature must be a PNG/JPEG no larger than 2048 × 2048")
	}
	im, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	// The PDF renderer accepts at most 8 bits per channel. png.Encode preserves
	// 16-bit decoded images unless we explicitly convert them; keep transparency.
	normalised := image.NewNRGBA(im.Bounds())
	draw.Draw(normalised, normalised.Bounds(), im, im.Bounds().Min, draw.Src)
	var out bytes.Buffer
	if err := png.Encode(&out, normalised); err != nil {
		return nil, err
	}
	if out.Len() > 1<<20 {
		return nil, errors.New("normalised signature must be at most 1 MB; use a smaller image")
	}
	return out.Bytes(), nil
}
func financialYearStart(t time.Time) int {
	y := t.In(india).Year()
	if t.In(india).Month() < 4 {
		y--
	}
	return y
}

func FinancialYear(t time.Time) string {
	y := financialYearStart(t)
	return fmt.Sprintf("%02d%02d", y%100, (y+1)%100)
}
func LoadSettings() (Settings, error) {
	var s Settings
	var body string
	err := query.Transaction(func(tx *query.Tx) error {
		return tx.QueryRow("SELECT value FROM invoice_settings WHERE id=1").Scan(&body)
	})
	if err != nil {
		return s, err
	}
	err = json.Unmarshal([]byte(body), &s)
	return s, err
}
func JSON(v interface{}) string { b, _ := json.Marshal(v); return string(b) }
func round(r *big.Rat) (int64, error) {
	if r.Sign() < 0 {
		return 0, errors.New("negative amount")
	}
	n := new(big.Int).Mul(r.Num(), big.NewInt(2))
	n.Add(n, r.Denom())
	n.Quo(n, new(big.Int).Mul(r.Denom(), big.NewInt(2)))
	if !n.IsInt64() {
		return 0, errors.New("amount overflow")
	}
	return n.Int64(), nil
}

// Total resolves the amount charged exactly once; export prices are not taxed.
func Total(base int64, p Product, country string) (int64, error) {
	if base <= 0 {
		return 0, errors.New("invalid amount")
	}
	if !p.Enabled || p.Inclusive || country != "IN" {
		return base, nil
	}
	if err := ValidateProduct(p); err != nil {
		return 0, err
	}
	rate, _ := new(big.Rat).SetString(p.Rate)
	mult := new(big.Rat).Add(big.NewRat(1, 1), new(big.Rat).Quo(rate, big.NewRat(100, 1)))
	return round(new(big.Rat).Mul(new(big.Rat).SetInt64(base), mult))
}
func Calculate(s *Snapshot) error {
	s.Currency = strings.ToUpper(strings.TrimSpace(s.Currency))
	if s.Context.ReviewReason != "" {
		return errors.New(s.Context.ReviewReason)
	}
	if err := s.Context.Seller.Validate(); err != nil {
		return err
	}
	if err := ValidateProduct(s.Context.Product); err != nil {
		return err
	}
	if err := s.Billing.Validate(); err != nil {
		return err
	}
	s.Export = s.Billing.Country != "IN"
	if s.Context.Country != "" && (s.Context.Country == "IN") != (s.Billing.Country == "IN") {
		return errors.New("billing country conflicts with checkout tax treatment")
	}
	if !s.Context.Product.Enabled {
		return errors.New("GST invoicing is not enabled")
	}
	if s.Gross <= 0 {
		return errors.New("invalid invoice total")
	}
	if s.Export {
		paid, err := time.Parse(time.RFC3339, s.PaidAt)
		if err != nil {
			return errors.New("missing payment date")
		}
		if !s.Context.Product.Export || s.Context.Seller.LUT == "" || s.Context.Seller.LUTYear != FinancialYear(paid) {
			return errors.New("valid LUT and product export eligibility required")
		}
		if s.NativeTax != nil && *s.NativeTax != 0 {
			return errors.New("export payment unexpectedly collected tax")
		}
		s.Net = s.Gross
		return nil
	}
	if s.Currency != "INR" {
		return errors.New("domestic foreign-currency invoice requires accounting review")
	}
	expected, err := Total(s.Context.Base, s.Context.Product, "IN")
	if err != nil {
		return err
	}
	if s.Context.Base > 0 && expected != s.Gross {
		return errors.New("paid total does not match frozen GST price")
	}
	rate, _ := new(big.Rat).SetString(s.Context.Product.Rate)
	div := new(big.Rat).Add(big.NewRat(100, 1), rate)
	tax, err := round(new(big.Rat).Quo(new(big.Rat).Mul(new(big.Rat).SetInt64(s.Gross), rate), div))
	if err != nil {
		return err
	}
	if !s.Context.Product.Inclusive && s.Context.Base > 0 {
		tax = s.Gross - s.Context.Base
	}
	if s.NativeTax != nil {
		delta := *s.NativeTax - tax
		if delta < 0 {
			delta = -delta
		}
		if *s.NativeTax < 0 || *s.NativeTax > s.Gross || delta > 1 || (!s.Context.Product.Inclusive && s.Context.Base > 0 && delta != 0) {
			return errors.New("provider tax does not match frozen GST calculation")
		}
		tax = *s.NativeTax
	}
	s.Net = s.Gross - tax
	if s.Context.Seller.GSTIN[:2] == StateCode(s.Billing.State) {
		s.CGST = tax / 2
		s.SGST = tax - s.CGST
	} else {
		s.IGST = tax
	}
	return nil
}
func Money(n int64, currency string) string {
	digits := 2
	switch strings.ToUpper(currency) {
	case "JPY", "KRW", "VND", "CLP", "XAF", "XOF", "XPF", "BIF", "DJF", "GNF", "KMF", "PYG", "RWF", "UGX", "VUV":
		digits = 0
	case "BHD", "JOD", "KWD", "OMR", "TND":
		digits = 3
	}
	scale := int64(1)
	for i := 0; i < digits; i++ {
		scale *= 10
	}
	if digits == 0 {
		return strconv.FormatInt(n, 10)
	}
	return fmt.Sprintf("%d.%0*d", n/scale, digits, n%scale)
}

// BaseFromGross recovers an exclusive base for provider plans that expose native
// tax metadata. The native metadata must be independently checked by the caller.
func BaseFromGross(gross int64, rate string) (int64, error) {
	r, ok := new(big.Rat).SetString(rate)
	if !ok || r.Sign() < 0 {
		return 0, errors.New("invalid GST rate")
	}
	tax, err := round(new(big.Rat).Quo(new(big.Rat).Mul(new(big.Rat).SetInt64(gross), r), new(big.Rat).Add(big.NewRat(100, 1), r)))
	return gross - tax, err
}

func StateName(code string) string {
	if canonical := StateCode(code); canonical != "" {
		code = canonical
	}
	for _, row := range strings.Split("01:JK:JAMMU AND KASHMIR|02:HP:HIMACHAL PRADESH|03:PB:PUNJAB|04:CH:CHANDIGARH|05:UT:UTTARAKHAND|06:HR:HARYANA|07:DL:DELHI|08:RJ:RAJASTHAN|09:UP:UTTAR PRADESH|10:BR:BIHAR|11:SK:SIKKIM|12:AR:ARUNACHAL PRADESH|13:NL:NAGALAND|14:MN:MANIPUR|15:MZ:MIZORAM|16:TR:TRIPURA|17:ML:MEGHALAYA|18:AS:ASSAM|19:WB:WEST BENGAL|20:JH:JHARKHAND|21:OR:ODISHA|22:CT:CHHATTISGARH|23:MP:MADHYA PRADESH|24:GJ:GUJARAT|26:DN:DADRA AND NAGAR HAVELI AND DAMAN AND DIU|27:MH:MAHARASHTRA|29:KA:KARNATAKA|30:GA:GOA|31:LD:LAKSHADWEEP|32:KL:KERALA|33:TN:TAMIL NADU|34:PY:PUDUCHERRY|35:AN:ANDAMAN AND NICOBAR ISLANDS|36:TG:TELANGANA|37:AP:ANDHRA PRADESH|38:LA:LADAKH", "|") {
		p := strings.Split(row, ":")
		if p[0] == code {
			return p[2]
		}
	}
	return code
}

// ValidateDelivery validates non-secret delivery settings without making delivery
// configuration a precondition for generating an invoice PDF.
func (s Settings) ValidateDelivery() error {
	switch s.DeliveryProvider {
	case "listmonk":
		if s.ListmonkInvoiceTemplate <= 0 || s.ListmonkCreditTemplate <= 0 {
			return errors.New("enter positive Listmonk invoice and credit-note template IDs")
		}
		if len(s.ListmonkMessenger) > 255 || strings.ContainsAny(s.ListmonkMessenger, "\r\n") {
			return errors.New("invalid Listmonk messenger")
		}
		if s.ListmonkFrom != "" {
			if strings.ContainsAny(s.ListmonkFrom, "\r\n") {
				return errors.New("invalid Listmonk sender address")
			}
			if _, err := mail.ParseAddress(s.ListmonkFrom); err != nil {
				return errors.New("invalid Listmonk sender address")
			}
		}
		return nil
	case "", "default":
		return nil
	default:
		return errors.New("unknown invoice email provider")
	}
}
