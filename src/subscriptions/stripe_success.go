package subscriptions

import (
	"net/http"
	"strconv"

	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/s3"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/stripe/stripe-go/v72"
	stripesession "github.com/stripe/stripe-go/v72/checkout/session"
)

// HandleStripeSuccess handles the redirect from Stripe Checkout after payment.
// It verifies the session was paid via the Stripe API, then renders the
// payment success page with a presigned download URL if the product has
// S3/R2 fields set. This matches how Square/PayPal/Razorpay handle downloads
// — presign after payment verification, not before checkout.
func HandleStripeSuccess(w http.ResponseWriter, r *http.Request) error {
	stripe.Key = config.Get("stripe_secret")

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		return server.InternalError(nil, "Stripe success: missing session_id")
	}

	s, err := stripesession.Get(sessionID, nil)
	if err != nil {
		log.Error(log.V{"Stripe success, Error retrieving session": err})
		return server.InternalError(err)
	}

	if s.Status != "complete" || s.PaymentStatus != "paid" {
		log.Info(log.V{"Stripe success, Session not paid": s.ID, "status": s.Status, "payment_status": s.PaymentStatus})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Payment+not+completed")
	}

	// Extract product_id from session metadata (set at checkout.go:AddMetadata)
	productIDStr, ok := s.Metadata["product_id"]
	if !ok || productIDStr == "" {
		log.Error(log.V{"Stripe success, Missing product_id in session metadata": s.ID})
		return renderPaymentSuccessWithDownload(w, r, "")
	}

	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil {
		log.Error(log.V{"Stripe success, Error parsing product_id": err})
		return renderPaymentSuccessWithDownload(w, r, "")
	}

	product, err := products.Find(productID)
	if err != nil {
		log.Error(log.V{"Stripe success, Error finding product": err})
		return renderPaymentSuccessWithDownload(w, r, "")
	}

	if product.S3Bucket != "" && product.S3Key != "" {
		downloadUrl, err := s3.GeneratePresignedUrl(product.S3Bucket, product.S3Key)
		if err == nil {
			return renderPaymentSuccessWithDownload(w, r, downloadUrl)
		}
		log.Error(log.V{"Stripe success, Error generating download url": err})
	}

	return renderPaymentSuccessWithDownload(w, r, "")
}
