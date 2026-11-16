package invoices

import (
	"database/sql"
	"errors"

	"github.com/abishekmuthian/open-payment-host/src/lib/listmonk"
	"github.com/abishekmuthian/open-payment-host/src/lib/mail"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
)

func sendInvoiceMail(e *mail.Email, snapshot Snapshot, number, kind string) error {
	enabled, err := config.GetBoolStrict("automated_invoice")
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("invoice email is disabled; set automated_invoice to yes in secrets")
	}
	// Resolve current delivery settings outside payment/database transactions.
	// A corrupt/unavailable settings record must never silently switch providers.
	settings, err := LoadSettings()
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := settings.ValidateDelivery(); err != nil {
		return err
	}
	if settings.DeliveryProvider == "listmonk" {
		template := settings.ListmonkInvoiceTemplate
		if kind == "credit_note" {
			template = settings.ListmonkCreditTemplate
		}
		message := listmonk.TransactionalMessage{
			SubscriberEmails: e.Recipients,
			SubscriberMode:   "external",
			TemplateID:       template,
			FromEmail:        settings.ListmonkFrom,
			Messenger:        settings.ListmonkMessenger,
			ContentType:      "html",
			AltBody:          e.Body,
			Headers:          []map[string]string{{"Message-ID": "<" + e.MessageID + ">"}, {"Reply-To": e.ReplyTo}},
			Data: map[string]interface{}{
				"customer_name":           snapshot.Billing.Name,
				"product_name":            snapshot.Context.descriptionDisplay(),
				"seller_name":             snapshot.Context.Seller.Name,
				"document_number":         number,
				"original_invoice_number": snapshot.OriginalNumber,
			},
		}
		for _, a := range e.Attachments {
			message.Attachments = append(message.Attachments, listmonk.Attachment{Name: a.Name, ContentType: a.ContentType, Data: a.Data})
		}
		return listmonk.SendTransactional(config.Get("listmonk_URL"), config.Get("listmonk_API_token"), message)
	}
	// The current service is initialized from secrets at application startup.
	// Legacy invoice-page SMTP values must not override that configuration.
	// Invoice sending is explicitly enabled in either environment, independently
	// of the environment-based logging used by other application emails.
	if mail.Service == nil {
		return errors.New("mail service is not configured")
	}
	return mail.Service.Send(e)
}
