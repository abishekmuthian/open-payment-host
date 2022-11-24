package paypal

import (
	"bufio"
	"errors"
	"io"
	"net/mail"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/gorilla/schema"
)

// Transaction is a PDT transaction.
// See https://developer.paypal.com/docs/classic/ipn/integration-guide/IPNandPDTVariables.
//
// BUG(arthurwhite): Can't handle multiple items from a shopping cart transaction.
type Transaction struct {
	ID                    string  `schema:"txn_id"`
	Type                  string  `schema:"txn_type"`
	Subject               string  `schema:"transaction_subject"`
	Business              Email   `schema:"business"`
	Custom                string  `schema:"custom"`
	Invoice               string  `schema:invoice`
	ReceiptID             string  `schema:receipt_ID`
	FirstName             string  `schema:"first_name"`
	HandlingAmount        float64 `schema:"handling_amount"`
	ItemNumber            string  `schema:"item_number"`
	ItemName              string  `schema:"item_name"`
	LastName              string  `schema:"last_name"`
	MerchantCurrency      string  `schema:"mc_currency"`
	MerchantFee           float64 `schema:"mc_fee"`
	MerchantGross         float64 `schema:"mc_gross"`
	PayerEmail            Email   `schema:"payer_email"`
	PayerID               string  `schema:"payer_id"`
	PayerStatus           string  `schema:"payer_status"`
	PaymentDate           Time    `schema:"payment_date"`
	PaymentFee            float64 `schema:"payment_fee"`
	PaymentGross          float64 `schema:"payment_gross"`
	PaymentStatus         string  `schema:"payment_status"`
	PaymentType           string  `schema:"payment_type"`
	ProtectionEligibility string  `schema:"protection_eligibility"`
	Quantity              int64   `schema:"quantity"`
	ReceiverID            string  `schema:"receiver_id"`
	ReceiverEmail         Email   `schema:"receiver_email"`
	ResidenceCountry      string  `schema:"residence_country"`
	Shipping              float64 `schema:"shipping"`
	Tax                   float64 `schema:"tax"`
	AddressCountry        string  `schema:"address_country"`
	TestIPN               int64   `schema:"test_ipn"`
	AddressStatus         string  `schema:"address_status"`
	AddressStreet         string  `schema:"address_street"`
	NotifyVersion         float64 `schema:"notify_version"`
	AddressCity           string  `schema:"address_city"`
	VerifySign            string  `schema:"verify_sign"`
	AddressState          string  `schema:"address_state"`
	Charset               string  `schema:"charset"`
	AddressName           string  `schema:"address_name"`
	AddressCountryCode    string  `schema:"address_country_code"`
	AddressZip            int64   `schema:"address_zip"`
	SubscriberID          string  `schema:"subscr_id"`
	TestPDT               int64   `schema:"test_pdt"`
}

const (
	timeLayout = "15:04:05 Jan 02, 2006 MST"
)

// Errors
var (
	ErrTransactionNotFound = errors.New("pdt: transaction not found")
)

func ParseTransaction(r io.Reader) (*Transaction, error) {
	bs := bufio.NewScanner(r)
	bs.Scan()
	if bs.Text() != "SUCCESS" {
		return nil, ErrTransactionNotFound
	}
	tx := new(Transaction)
	txValue := reflect.ValueOf(tx).Elem()
	for bs.Scan() {
		t := bs.Text()
		i := strings.IndexByte(t, '=')
		if i == -1 {
			continue
		}
		key := t[:i]
		val, err := url.QueryUnescape(t[i+1:])
		if err != nil {
			return nil, err
		}
		for i := 0; i < txValue.NumField(); i++ {
			field := txValue.Type().Field(i)
			if tagVal, ok := field.Tag.Lookup("schema"); !ok || tagVal != key {
				continue
			}
			switch field.Type.String() {
			case "string":
				txValue.Field(i).SetString(val)
			case "float64":
				f, err := strconv.ParseFloat(val, 64)
				if err != nil {
					continue
				}
				txValue.Field(i).SetFloat(f)
			case "paypal.Email":
				m, err := mail.ParseAddress(val)
				if err != nil {
					continue
				}

				value := Email{Email: m}

				txValue.Field(i).Set(reflect.ValueOf(value))
			case "paypal.Time":
				loc, err := time.LoadLocation("America/Los_Angeles")
				if err != nil {
					continue
				}

				t, err := time.ParseInLocation(timeLayout, string(val), loc)
				//t = t.UTC() //Convert the time to UTC
				if err != nil {
					continue
				}

				value := Time{Time: &t}

				txValue.Field(i).Set(reflect.ValueOf(value)) //Not converted to UTC

			}
		}
	}
	return tx, nil
}

// ReadNotification reads a notification from an //IPN request
func ReadNotification(vals url.Values) *Transaction {
	n := &Transaction{}
	decoder := schema.NewDecoder()
	err := decoder.Decode(n, vals) //errors due to missing fields in struct

	if err != nil {
		log.Error(log.V{"Paypal IPN Verification": "ReadNotification", "Error": err})
	}
	return n
}

type Time struct {
	Time *time.Time
}

type Email struct {
	Email *mail.Address
}

func (t *Time) UnmarshalText(text []byte) (err error) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return err
	}
	time, err := time.ParseInLocation(timeLayout, string(text), loc)
	//time = time.UTC() //Convert the time to UTC
	if err != nil {
		return err
	}
	t.Time = &time
	return nil
}

func (e *Email) UnmarshalText(text []byte) (err error) {
	e.Email, err = mail.ParseAddress(string(text))
	return
}
