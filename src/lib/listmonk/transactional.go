package listmonk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

// TransactionalMessage uses Listmonk's transactional templates independently of
// subscriber synchronization. Attachments are sent as multipart file parts.
type TransactionalMessage struct {
	SubscriberEmails []string               `json:"subscriber_emails"`
	SubscriberMode   string                 `json:"subscriber_mode"`
	TemplateID       int                    `json:"template_id"`
	FromEmail        string                 `json:"from_email,omitempty"`
	Data             map[string]interface{} `json:"data,omitempty"`
	Headers          []map[string]string    `json:"headers,omitempty"`
	Messenger        string                 `json:"messenger,omitempty"`
	ContentType      string                 `json:"content_type"`
	AltBody          string                 `json:"altbody,omitempty"`
	Attachments      []Attachment           `json:"-"`
}

type Attachment struct {
	Name, ContentType string
	Data              []byte
}

// ValidateTransactionalConfig checks configuration without contacting Listmonk.
func ValidateTransactionalConfig(baseURL, apiToken string) error {
	u, err := url.Parse(baseURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("configure a valid Listmonk HTTP(S) URL")
	}
	header, err := authorizationHeader(strings.TrimSpace(apiToken))
	if err != nil {
		return err
	}
	key, token, _ := strings.Cut(strings.TrimPrefix(header, "token "), ":")
	if strings.TrimSpace(key) == "" || strings.TrimSpace(token) == "" || strings.ContainsAny(header, "\r\n") {
		return errors.New("Listmonk API token must contain an API username and generated token")
	}
	return nil
}

// SendTransactional confirms queue acceptance, not SMTP or inbox delivery. The
// caller owns durable retries; this client never retries a potentially accepted
// message or follows a redirect with its API credentials.
func SendTransactional(baseURL, apiToken string, message TransactionalMessage) error {
	return sendTransactional(baseURL, apiToken, message, 30*time.Second)
}

func sendTransactional(baseURL, apiToken string, message TransactionalMessage, timeout time.Duration) error {
	if err := ValidateTransactionalConfig(baseURL, apiToken); err != nil {
		return err
	}
	if message.TemplateID <= 0 || len(message.SubscriberEmails) == 0 {
		return errors.New("Listmonk requires a transactional template and recipient")
	}
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err = writer.WriteField("data", string(data)); err != nil {
		return err
	}
	for _, attachment := range message.Attachments {
		if attachment.Name == "" || len(attachment.Data) == 0 {
			return errors.New("Listmonk attachment requires a filename and content")
		}
		if _, _, err := mime.ParseMediaType(attachment.ContentType); err != nil {
			return errors.New("invalid Listmonk attachment content type")
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": attachment.Name}))
		header.Set("Content-Type", attachment.ContentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return err
		}
		if _, err = part.Write(attachment.Data); err != nil {
			return err
		}
	}
	if err = writer.Close(); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/tx", &body)
	if err != nil {
		return err
	}
	authHeader, _ := authorizationHeader(strings.TrimSpace(apiToken))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Listmonk submission failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return transactionalSubmissionError(resp)
	}
	var result struct {
		Data bool `json:"data"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 64<<10))
	if err := decoder.Decode(&result); err != nil || !result.Data {
		return errors.New("Listmonk did not confirm queue acceptance")
	}
	var extra interface{}
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("invalid Listmonk acceptance response")
	}
	return nil
}

// Recognize only known authentication/permission failures. Arbitrary response
// bodies must never reach logs or invoice status: they may contain private data.
func transactionalSubmissionError(resp *http.Response) error {
	var reason string
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		if resp.Header.Get("Cf-Mitigated") == "challenge" {
			reason = "Cloudflare challenge blocked the API request; allow server access to /api/tx without an interactive challenge"
		} else {
			var result struct {
				Message string `json:"message"`
			}
			if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&result) == nil {
				switch result.Message {
				case "invalid API credentials":
					reason = "invalid API credentials; check listmonk_API_token in secrets (API username:generated token) for the configured Listmonk instance"
				case "permission denied: tx:send":
					reason = "permission denied: tx:send; grant the Listmonk API user's role the tx:send permission"
				case "invalid session":
					reason = "invalid session; check that /api/tx reaches Listmonk and the proxy forwards the Authorization header"
				}
			}
			if reason == "" {
				reason = "access denied; check the API credentials, tx:send permission, and reverse-proxy access rules"
			}
		}
	}
	if reason != "" {
		return fmt.Errorf("Listmonk submission returned HTTP %d: %s", resp.StatusCode, reason)
	}
	return fmt.Errorf("Listmonk submission returned HTTP %d", resp.StatusCode)
}
