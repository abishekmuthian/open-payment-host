package smtp

import (
	"bytes"
	"encoding/base64"
	m "github.com/abishekmuthian/open-payment-host/src/lib/mail"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

func TestPDFMIME(t *testing.T) {
	e := m.New("buyer@example.test")
	e.Subject = "GST invoice"
	e.Body = "Attached"
	e.MessageID = "stable@example.test"
	e.Attachments = []m.Attachment{{Name: "OPH-2627-000001.pdf", ContentType: "application/pdf", Data: []byte("%PDF-test")}}
	data, err := Message(e, "seller@example.test")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(msg.Body, params["boundary"])
	_, _ = reader.NextPart()
	attachment, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, attachment))
	if err != nil || string(decoded) != "%PDF-test" || attachment.FileName() != e.Attachments[0].Name {
		t.Fatal("attachment corrupted", err)
	}
	if !strings.Contains(string(data), "Message-ID: <stable@example.test>") {
		t.Fatal("unstable message ID")
	}
}
