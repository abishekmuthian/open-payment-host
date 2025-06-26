package subscriptions

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
)

// SendWebhook sends a webhook to the specified URL with the given payload.
func SendWebhook(url string, secret string, params map[string]interface{}) error {
	// payload := fmt.Sprint(params["subscription_id"], "|", params["custom_id"], "|", params["status"])

	// Marshal params to JSON
	jsonParams, err := json.Marshal(params)
	if err != nil {
		// Handle error appropriately
		log.Error(log.V{"Error marshaling params: ": err})
	}

	body := bytes.NewReader([]byte(jsonParams))

	signature := GenerateSignature([]byte(jsonParams), secret)

	request, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		// handle err
		log.Error(log.V{"SendWebhook error": err})
		return err
	}

	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Add("X-OPH-Signature", signature)

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Error(log.V{"SendWebhook error": err})

		return err
	}

	defer resp.Body.Close()

	return err

}

// GenerateSignature generates a signature for the given payload and secret.
func GenerateSignature(body []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	signature := hex.EncodeToString(h.Sum(nil))
	return signature
}
