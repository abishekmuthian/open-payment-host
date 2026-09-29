package subscriptions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/abishekmuthian/open-payment-host/src/lib/listmonk"
	"github.com/abishekmuthian/open-payment-host/src/lib/mailchimp"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"net/http"
	"net/url"
	"strings"
)

// Mailing-list operations are idempotent upserts/removals, retried durably.
func syncPaymentMailingLists(p *products.Story, email, status string) error {
	if email == "" || status == "partially_refunded" {
		return nil
	}
	active := status == "active"
	if p.ListmonkListID > 0 && config.Get("listmonk_URL") != "" && config.Get("listmonk_API_token") != "" {
		var err error
		if active {
			_, err = listmonk.UpsertSubscriber(config.Get("listmonk_URL"), config.Get("listmonk_API_token"), email, "", int(p.ListmonkListID), listmonkPreconfirmSubscriptions())
		} else {
			err = listmonk.RemoveSubscriberFromList(config.Get("listmonk_URL"), config.Get("listmonk_API_token"), email, int(p.ListmonkListID))
		}
		if err != nil {
			return err
		}
	}
	token := config.Get("mailchimp_token")
	if p.MailchimpAudienceID != "" && token != "" {
		parts := strings.Split(token, "-")
		dc := parts[len(parts)-1]
		if !strings.HasPrefix(dc, "us") || strings.Trim(dc[2:], "0123456789") != "" {
			return errors.New("invalid Mailchimp data center")
		}
		state := "unsubscribed"
		if active {
			state = "subscribed"
		}
		body, err := json.Marshal(map[string]string{"email_address": email, "status": state, "status_if_new": state})
		if err != nil {
			return err
		}
		req, err := http.NewRequest(http.MethodPut, "https://"+dc+".api.mailchimp.com/3.0/lists/"+url.PathEscape(p.MailchimpAudienceID)+"/members/"+mailchimp.GetMD5Hash(strings.ToLower(email)), bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.SetBasicAuth("oph", token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := paymentHTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("Mailchimp returned HTTP %d", resp.StatusCode)
		}
	}
	return nil
}
