package subscriptions

import (
	"encoding/json"
	"net/http"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
)

// HandlePaymentStatus reports the status of a payment attempt for PayPal,
// Razorpay and Square subscription payments awaiting provider confirmation.
// The request must carry the attempt cookie which was bound at checkout start,
// so only the browser which started the payment can poll its state.
// It responds to GET /subscriptions/payment-status?attempt_id=<id>
func HandlePaymentStatus(w http.ResponseWriter, r *http.Request) error {
	paymentResponseHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	attemptId := params.Get("attempt_id")
	attempt, err := FindAttempt(attemptId)
	if err != nil {
		log.Error(log.V{"Payment status, attempt not found": attemptId, "error": err})
		return server.NotFoundError(err, "Payment could not be verified", "Sorry, this payment could not be verified.")
	}

	// The attempt cookie must match - only the browser which started this
	// checkout may poll its status
	if !bindAttemptToRequest(r, attempt) {
		return server.NotAuthorizedError(nil)
	}

	response := map[string]string{
		"status": attempt.Status,
	}

	if attempt.Status == AttemptStatusCompleted {
		if err := issueCompletionCookie(w, r, attempt); err != nil {
			return server.InternalError(err)
		}
		response["success_url"] = "/subscriptions/success?attempt_id=" + attempt.Id
	}

	return json.NewEncoder(w).Encode(response)
}
