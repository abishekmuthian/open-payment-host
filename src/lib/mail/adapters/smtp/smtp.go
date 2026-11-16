// Package smtp delivers MIME messages through an authenticated TLS SMTP server.
package smtp

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	m "github.com/abishekmuthian/open-payment-host/src/lib/mail"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

type Service struct{ Host, Port, Username, Password, From, Mode string }

func (s *Service) Send(e *m.Email) error {
	if s.Host == "" || s.From == "" || (s.Mode != "starttls" && s.Mode != "tls") {
		return errors.New("configure SMTP host, from and TLS mode")
	}
	for _, v := range append([]string{s.From, e.ReplyTo, e.Subject, e.MessageID}, e.Recipients...) {
		if strings.ContainsAny(v, "\r\n") {
			return errors.New("invalid email header")
		}
	}
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return err
	}
	var recipients []string
	for _, r := range e.Recipients {
		a, err := mail.ParseAddress(r)
		if err != nil {
			return err
		}
		recipients = append(recipients, a.Address)
	}
	if len(recipients) == 0 {
		return errors.New("missing recipients")
	}
	if _, err = strconv.Atoi(s.Port); err != nil {
		return errors.New("invalid SMTP port")
	}
	addr := net.JoinHostPort(s.Host, s.Port)
	dial := net.Dialer{Timeout: 20 * time.Second}
	var conn net.Conn
	tlsConfig := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}
	if s.Mode == "tls" {
		conn, err = tls.DialWithDialer(&dial, "tcp", addr, tlsConfig)
	} else {
		conn, err = dial.Dial("tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	if err = conn.SetDeadline(time.Now().Add(60 * time.Second)); err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	if s.Mode == "starttls" {
		if err = c.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if s.Username != "" {
		if err = c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return err
		}
	}
	data, err := Message(e, from.String())
	if err != nil {
		return err
	}
	if err = c.Mail(from.Address); err != nil {
		return err
	}
	for _, r := range recipients {
		if err = c.Rcpt(r); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(data); err != nil {
		_ = w.Close()
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
func Message(e *m.Email, from string) ([]byte, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	head := textproto.MIMEHeader{}
	head.Set("Content-Type", "text/html; charset=UTF-8")
	head.Set("Content-Transfer-Encoding", "base64")
	part, err := w.CreatePart(head)
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(encoded([]byte(e.Body))); err != nil {
		return nil, err
	}
	for _, a := range e.Attachments {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", a.ContentType)
		h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": a.Name}))
		h.Set("Content-Transfer-Encoding", "base64")
		part, err = w.CreatePart(h)
		if err != nil {
			return nil, err
		}
		if _, err = part.Write(encoded(a.Data)); err != nil {
			return nil, err
		}
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=%q\r\n", from, strings.Join(e.Recipients, ", "), mime.QEncoding.Encode("utf-8", e.Subject), time.Now().Format(time.RFC1123Z), w.Boundary())
	if e.ReplyTo != "" {
		fmt.Fprintf(&out, "Reply-To: %s\r\n", e.ReplyTo)
	}
	if e.MessageID != "" {
		fmt.Fprintf(&out, "Message-ID: <%s>\r\n", e.MessageID)
	}
	out.WriteString("\r\n")
	out.Write(body.Bytes())
	return out.Bytes(), nil
}
func encoded(b []byte) []byte {
	v := base64.StdEncoding.EncodeToString(b)
	var out bytes.Buffer
	for len(v) > 76 {
		out.WriteString(v[:76] + "\r\n")
		v = v[76:]
	}
	out.WriteString(v + "\r\n")
	return out.Bytes()
}
