// Package mail sends the site's emails: account verification, recovery
// links, invites and security notices. A Sender delivers a Message; the
// SMTP sender talks to Resend, the log sender writes the message to the log
// when SMTP is not configured (local development and the E2E tests read
// links from there). A Mailer renders the templates and sends.
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
)

// Message is a rendered email for 1 recipient.
type Message struct {
	To      string // a bare address, for example "alice@example.com"
	Subject string
	Text    string // the text/plain part
	HTML    string // the text/html part
}

// Sender delivers messages.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// ParseFrom parses a sender like "Go Practice <go-practice@yornik.eu>".
func ParseFrom(from string) (*mail.Address, error) {
	a, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("mail: sender %q: %w", from, err)
	}
	return a, nil
}

// headerSafe rejects line breaks: a value with CR or LF could add headers.
func headerSafe(name, v string) error {
	if strings.ContainsAny(v, "\r\n") {
		return fmt.Errorf("mail: %s contains a line break", name)
	}
	return nil
}

// recipient validates m.To and returns the bare address.
func (m Message) recipient() (string, error) {
	if err := headerSafe("recipient", m.To); err != nil {
		return "", err
	}
	a, err := mail.ParseAddress(m.To)
	if err != nil {
		return "", fmt.Errorf("mail: recipient %q: %w", m.To, err)
	}
	return a.Address, nil
}

// Encode renders m as an RFC 5322 message with CRLF line endings: the
// headers, then a multipart/alternative body with the text part first and
// the HTML part last, as clients prefer the last part they can show.
func (m Message) Encode(from *mail.Address, now time.Time) ([]byte, error) {
	to, err := m.recipient()
	if err != nil {
		return nil, err
	}
	if err := headerSafe("subject", m.Subject); err != nil {
		return nil, err
	}
	if m.Text == "" || m.HTML == "" {
		return nil, errors.New("mail: a message needs a text and an HTML part")
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, part := range []struct{ ctype, content string }{
		{"text/plain; charset=utf-8", m.Text},
		{"text/html; charset=utf-8", m.HTML},
	} {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", part.ctype)
		h.Set("Content-Transfer-Encoding", "quoted-printable")
		w, err := mw.CreatePart(h)
		if err != nil {
			return nil, fmt.Errorf("mail: encode: %w", err)
		}
		// Quoted-printable keeps lines under 76 characters, which SMTP
		// requires, and survives servers that are not 8-bit clean.
		qp := quotedprintable.NewWriter(w)
		if _, err := io.WriteString(qp, part.content); err != nil {
			return nil, fmt.Errorf("mail: encode: %w", err)
		}
		if err := qp.Close(); err != nil {
			return nil, fmt.Errorf("mail: encode: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("mail: encode: %w", err)
	}

	var b bytes.Buffer
	header := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	header("From", from.String())
	header("To", (&mail.Address{Address: to}).String())
	header("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", messageID(from.Address))
	header("MIME-Version", "1.0")
	// Auto-Submitted stops vacation responders from answering a robot.
	header("Auto-Submitted", "auto-generated")
	header("Content-Type", mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": mw.Boundary()}))
	b.WriteString("\r\n")
	b.Write(body.Bytes())
	return b.Bytes(), nil
}

// messageID returns a unique Message-ID in the sender's domain.
func messageID(from string) string {
	domain := "localhost"
	if _, d, ok := strings.Cut(from, "@"); ok && d != "" {
		domain = d
	}
	return "<" + strings.ToLower(rand.Text()) + "@" + domain + ">"
}
