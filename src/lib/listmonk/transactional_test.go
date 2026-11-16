package listmonk

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTransactionalPDFSubmission(t *testing.T) {
	pdf := []byte("%PDF-1.7\n\x00\xff original invoice bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/listmonk/api/tx" || r.Header.Get("Authorization") != "token oph:secret" {
			t.Error("incorrect transactional endpoint or authentication")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		var message TransactionalMessage
		if err := json.Unmarshal([]byte(r.FormValue("data")), &message); err != nil {
			t.Error(err)
		}
		if message.TemplateID != 17 || message.SubscriberMode != "external" || len(message.SubscriberEmails) != 1 || message.SubscriberEmails[0] != "buyer@example.test" || message.Messenger != "billing" || message.Data["document_number"] != "OPH/2627/000001" {
			t.Errorf("incorrect message: %+v", message)
		}
		if message.Headers[0]["Message-ID"] != "<stable@example.test>" {
			t.Error("message correlation header lost")
		}
		files := r.MultipartForm.File["file"]
		if len(files) != 1 || files[0].Filename != "OPH-2627-000001.pdf" || files[0].Header.Get("Content-Type") != "application/pdf" {
			t.Error("incorrect PDF attachment")
		} else {
			f, err := files[0].Open()
			if err != nil {
				t.Error(err)
			} else {
				defer f.Close()
				got, err := io.ReadAll(f)
				if err != nil || !bytes.Equal(got, pdf) {
					t.Error("PDF bytes changed")
				}
			}
		}
		_, _ = io.WriteString(w, `{"data":true}`)
	}))
	defer server.Close()
	err := SendTransactional(server.URL+"/listmonk/", "token oph:secret", TransactionalMessage{
		TemplateID: 17, SubscriberEmails: []string{"buyer@example.test"}, SubscriberMode: "external",
		ContentType: "html", Messenger: "billing", Headers: []map[string]string{{"Message-ID": "<stable@example.test>"}},
		Data:        map[string]interface{}{"document_number": "OPH/2627/000001"},
		Attachments: []Attachment{{Name: "OPH-2627-000001.pdf", ContentType: "application/pdf", Data: pdf}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTransactionalRejectsUnconfirmedResponsesWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"denied", `private recipient@example.test secret`, 403},
		{"server failure", `private recipient@example.test secret`, 503},
		{"rate limited", `{}`, 429},
		{"redirect", `{}`, 302},
		{"empty", ``, 204},
		{"false", `{"data":false}`, 200},
		{"missing", `{}`, 200},
		{"html", `<html>private recipient@example.test secret</html>`, 200},
		{"wrong type", `{"data":"true"}`, 200},
		{"trailing data", `{"data":true} garbage`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			err := SendTransactional(server.URL, "oph:secret", TransactionalMessage{TemplateID: 1, SubscriberEmails: []string{"buyer@example.test"}})
			if err == nil || calls != 1 {
				t.Fatalf("unconfirmed submission accepted or retried: %v, calls=%d", err, calls)
			}
			if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "secret") {
				t.Fatal("response data leaked through error")
			}
		})
	}
}

func TestTransactionalTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()
	err := sendTransactional(server.URL, "oph:secret", TransactionalMessage{TemplateID: 1, SubscriberEmails: []string{"buyer@example.test"}}, 5*time.Millisecond)
	var timeout net.Error
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatalf("expected bounded request timeout: %v", err)
	}
}

func TestTransactionalAccessErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, mitigation, want string
		status                       int
	}{
		{"credentials", `{"message":"invalid API credentials","private":"recipient@example.test secret"}`, "", "invalid API credentials; check listmonk_API_token", 403},
		{"permission", `{"message":"permission denied: tx:send"}`, "", "grant the Listmonk API user's role the tx:send permission", 403},
		{"session", `{"message":"invalid session"}`, "", "proxy forwards the Authorization header", 403},
		{"challenge", `<html>recipient@example.test secret</html>`, "challenge", "Cloudflare challenge blocked the API request", 403},
		{"unknown JSON", `{"message":"denied recipient@example.test secret"}`, "", "access denied; check the API credentials", 403},
		{"unknown HTML", `<html>recipient@example.test secret</html>`, "", "access denied; check the API credentials", 403},
		{"oversized", `{"padding":"` + strings.Repeat("x", 64<<10) + `","message":"permission denied: tx:send"}`, "", "access denied; check the API credentials", 403},
		{"unauthorized", `{"message":"invalid API credentials"}`, "", "invalid API credentials; check listmonk_API_token", 401},
		{"unrelated status", `{"message":"invalid API credentials"}`, "", "Listmonk submission returned HTTP 500", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Cf-Mitigated", tc.mitigation)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			err := SendTransactional(server.URL, "oph:secret", TransactionalMessage{TemplateID: 1, SubscriberEmails: []string{"buyer@example.test"}})
			if err == nil || !strings.Contains(err.Error(), tc.want) || calls != 1 {
				t.Fatalf("expected actionable error without retry, got %v (calls=%d)", err, calls)
			}
			if strings.Contains(err.Error(), "recipient@example.test") || strings.Contains(err.Error(), "oph:secret") {
				t.Fatal("private response data leaked through error")
			}
		})
	}
}

func TestTransactionalConfig(t *testing.T) {
	for _, tc := range []struct{ url, token string }{
		{"", "oph:secret"}, {"ftp://example.test", "oph:secret"},
		{"https://user:pass@example.test", "oph:secret"}, {"https://example.test?token=secret", "oph:secret"},
		{"https://example.test", ""}, {"https://example.test", "invalid"},
		{"https://example.test", "oph:"}, {"https://example.test", ":secret"},
		{"https://example.test", "oph:secret\r\nOther: value"},
	} {
		if ValidateTransactionalConfig(tc.url, tc.token) == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
}
