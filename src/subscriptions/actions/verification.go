package actions

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/subscriptions"

	"github.com/abishekmuthian/open-payment-host/src/lib/paypal"
)

// HandleVerificationShow shows the subscriptions page by responding to the GET request
func HandleVerificationShow(w http.ResponseWriter, r *http.Request) error {

	var verificationTitle, verificationMessage string

	subscription := subscriptions.New()

	// FIXME: Verify the email of the merchant

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// FIXME: Authorize user (Check implementation)

	// Getting the current user id
	currentUser := session.CurrentUser(w, r)

	// Get the transaction id
	transactionId := params.Get("tx")

	if transactionId != "" {
		log.Info(log.V{"Payment PDT Verification, transaction id is ": transactionId})

		resp, err := http.PostForm(config.Get("paypal_PDT_hostname"), url.Values{
			"cmd": {"_notify-synch"},
			"at":  {config.Get("paypal_token")},
			"tx":  {transactionId},
		})

		if err != nil {
			// Explore response object
			log.Error(log.V{"msg": "Payment PDT Verification", "error": err, "Response": resp.Body})
			verificationTitle = config.Get("paypal_verification_failure_title")
			verificationMessage = config.Get("paypal_verification_failure_message")
		} else {
			log.Info(log.V{"Payment PDT Verification": "parsing response"})
			transaction, err := paypal.ParseTransaction(resp.Body)
			if err != nil {
				log.Error(log.V{"Paypal PDT Verfication": err})
				verificationTitle = config.Get("paypal_verification_failure_title")
				verificationMessage = config.Get("paypal_verification_failure_message")
			} else {
				log.Info(log.V{"Paypal PDT Verification": "Parsed", "Result": fmt.Sprintf("%+v\n", transaction)})

				if strings.Contains(transaction.PayerEmail.Email.String(), "example.com") {
					log.Info(log.V{"Paypal PDT Verification": "Test PDT"})
					transaction.TestPDT = 1
				}

				err = recordPaymentTransaction(transaction, subscription, currentUser.ID)
				if err != nil {
					log.Error(log.V{"Paypal PDT Verification": err})
					verificationTitle = config.Get("paypal_verification_failure_title")
					verificationMessage = config.Get("paypal_verification_failure_message")
				} else {
					verificationTitle = config.Get("paypal_verification_success_title")
					verificationMessage = config.Get("paypal_verification_success_message")
				}
			}
		}
	} else {
		log.Error(log.V{"Payment Failed, ": "No transaction id found"})
		verificationTitle = config.Get("paypal_verification_failure_title")
		verificationMessage = config.Get("paypal_verification_failure_message")
	}

	// Render the template
	view := view.NewRenderer(w, r)
	view.AddKey("currentUser", currentUser)
	view.AddKey("title", verificationTitle)
	view.AddKey("message", verificationMessage)

	return view.Render()
}

// HandleVerification verifies the IPN message by responding to the POST request
func HandleVerification(w http.ResponseWriter, r *http.Request) error {

	subscription := subscriptions.New()

	// *********************************************************
	// HANDSHAKE STEP 1 -- Write back an empty HTTP 200 response
	// *********************************************************
	log.Info(log.V{"Paypal IPN Verification": "Write Status 200"})
	w.WriteHeader(http.StatusOK)

	// *********************************************************
	// HANDSHAKE STEP 2 -- Send POST data (IPN message) back as verification
	// *********************************************************
	// Get Content-Type of request to be parroted back to paypal
	contentType := r.Header.Get("Content-Type")
	// Read the raw POST body
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error(log.V{"Paypal IPN Verfication": err})
		return nil
	}
	// Prepend POST body with required field
	body = append([]byte("cmd=_notify-validate&"), body...)
	// Make POST request to paypal
	resp, err := http.Post(config.Get("paypal_IPN_hostname"), contentType, bytes.NewBuffer(body))
	if err != nil {
		log.Error(log.V{"Paypal IPN Verfication": err})
		return nil
	}

	// *********************************************************
	// HANDSHAKE STEP 3 -- Read response for VERIFIED or INVALID
	// *********************************************************
	verifyStatus, _ := ioutil.ReadAll(resp.Body)

	// *********************************************************
	// Test for VERIFIED
	// *********************************************************
	if string(verifyStatus) != "VERIFIED" {
		log.Error(log.V{"Paypal IPN Verification": "Error checking for Verified", "Response: %v": string(verifyStatus)})
		log.Error(log.V{"Paypal IPN Verification": "This indicates that an attempt was made to spoof this interface, or we have a bug."})
		return nil
	}

	// We can now assume that the POSTed information in `body` is VERIFIED to be from Paypal.
	log.Info(log.V{"Paypal IPN Verification": "Checking for Verified", "Response: %v": string(verifyStatus)})

	form, err := url.ParseQuery(string(body))
	if err != nil {
		log.Error(log.V{"Paypal IPN Verification": err})
		return nil
	}

	transaction := paypal.ReadNotification(form)

	if err != nil {
		log.Error(log.V{"Paypal IPN Verification": err})
		return nil
	} else {
		log.Info(log.V{"Paypal IPN Verification": "Parsed", "Result": fmt.Sprintf("%+v\n", transaction)})

		err = recordPaymentTransaction(transaction, subscription, 0)
		if err != nil {
			log.Error(log.V{"Paypal IPN Verification": err})
		} else {
			log.Info(log.V{"Paypal IPN Verification": "Successful"})
		}
	}

	return nil
}

// HandleRazorpayPaymentSuccess enables the subscription features after razorpay payment
func HandleRazorpayPaymentVerification(w http.ResponseWriter, r *http.Request) error {

	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	log.Info(log.V{"Razorpay": "Payment Success", "Params": params})

	secret := config.Get("razorpay_key_secret")
	data := params.Get("razorpay_payment_id") + "|" + params.Get("subscription_id")
	fmt.Printf("Secret: %s Data: %s\n", secret, data)

	// Create a new HMAC by defining the hash type and the key (as byte array)
	h := hmac.New(sha256.New, []byte(secret))

	// Write Data to it
	h.Write([]byte(data))

	// Get result and encode as hexadecimal string
	sha := hex.EncodeToString(h.Sum(nil))

	fmt.Println("Result: " + sha)

	return err
}

// recordPaymentTransaction adds the transaction to database
func recordPaymentTransaction(transaction *paypal.Transaction, subscription *subscriptions.Subscription, userID int64) error {
	// Add an entry in the subscriptions table
	// FIXME: adjust query to do this for us we should use ?,?,? here...

	// FIXME: Verfiy email of the merchant
	// Params not validated using ValidateParams as user did not create these?

	transactionParams := make(map[string]string)

	transactionParams["txn_id"] = transaction.ID
	transactionParams["txn_type"] = transaction.Type
	transactionParams["transaction_subject"] = transaction.Subject
	if transaction.Business.Email != nil {
		transactionParams["business"] = transaction.Business.Email.Address
	}
	transactionParams["custom"] = transaction.Custom
	transactionParams["invoice"] = transaction.Invoice
	transactionParams["receipt_id"] = transaction.ReceiptID
	transactionParams["first_name"] = transaction.FirstName
	transactionParams["handling_amount"] = strconv.FormatFloat(transaction.HandlingAmount, 'E', -1, 64)
	transactionParams["item_number"] = transaction.ItemNumber
	transactionParams["item_name"] = transaction.ItemName
	transactionParams["last_name"] = transaction.LastName
	transactionParams["mc_currency"] = transaction.MerchantCurrency
	transactionParams["mc_fee"] = strconv.FormatFloat(transaction.MerchantFee, 'E', -1, 64)
	transactionParams["mc_gross"] = strconv.FormatFloat(transaction.MerchantGross, 'E', -1, 64)
	transactionParams["payer_email"] = transaction.PayerEmail.Email.Address
	transactionParams["payer_id"] = transaction.PayerID
	transactionParams["payer_status"] = transaction.PayerStatus
	transactionParams["payment_date"] = query.TimeString(transaction.PaymentDate.Time.UTC())
	transactionParams["payment_fee"] = strconv.FormatFloat(transaction.PaymentFee, 'E', -1, 64)
	transactionParams["payment_gross"] = strconv.FormatFloat(transaction.PaymentGross, 'E', -1, 64)
	transactionParams["payment_status"] = transaction.PaymentStatus
	transactionParams["payment_type"] = transaction.PaymentType
	transactionParams["protection_eligibility"] = transaction.ProtectionEligibility
	transactionParams["quantity"] = strconv.FormatInt(transaction.Quantity, 10)
	transactionParams["receiver_id"] = transaction.ReceiverID
	if transaction.ReceiverEmail.Email != nil {
		transactionParams["receiver_email"] = transaction.ReceiverEmail.Email.Address
	}
	transactionParams["residence_country"] = transaction.ResidenceCountry
	transactionParams["shipping"] = strconv.FormatFloat(transaction.Shipping, 'E', -1, 64)
	transactionParams["tax"] = strconv.FormatFloat(transaction.Tax, 'E', -1, 64)
	transactionParams["address_country"] = transaction.AddressCountry
	transactionParams["test_ipn"] = strconv.FormatInt(transaction.TestIPN, 10)
	transactionParams["address_status"] = transaction.AddressStatus
	transactionParams["address_street"] = transaction.AddressStreet
	transactionParams["notify_version"] = strconv.FormatFloat(transaction.NotifyVersion, 'E', -1, 64)
	transactionParams["address_city"] = transaction.AddressCity
	transactionParams["verify_sign"] = transaction.VerifySign
	transactionParams["address_state"] = transaction.AddressState
	transactionParams["charset"] = transaction.Charset
	transactionParams["address_name"] = transaction.AddressName
	transactionParams["address_country_code"] = transaction.AddressCountryCode
	transactionParams["address_zip"] = strconv.FormatInt(transaction.AddressZip, 10)
	transactionParams["subscr_id"] = transaction.SubscriberID
	transactionParams["user_id"] = strconv.FormatInt(userID, 10)
	transactionParams["test_pdt"] = strconv.FormatInt(transaction.TestPDT, 10)

	_, err := subscription.Create(transactionParams)

	return err
}
