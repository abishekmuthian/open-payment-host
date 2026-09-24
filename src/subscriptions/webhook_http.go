package subscriptions

import (
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"io"
	"net/http"
)

func readWebhookBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return nil, http.ErrNotSupported
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		http.Error(w, "Invalid event body", 400)
	}
	return body, err
}
func webhookResult(w http.ResponseWriter, err error) error {
	if err != nil {
		log.Error(log.V{"payment webhook processing failed": err})
		http.Error(w, "Event processing failed; retry later", 503)
		return nil
	}
	w.WriteHeader(http.StatusOK)
	return nil
}
