package mail

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"
)

const site = "https://go-practice.dev"

var when = time.Date(2026, 10, 19, 9, 30, 0, 0, time.UTC)

func TestComposeEveryEmail(t *testing.T) {
	link := site + "/verify-email?token=a&b=<c>"
	tests := []struct {
		email    Email
		subject  string
		rest     bool     // HUSH ends with "Please rest."
		security bool     // a security notice: "What happened:" and a WARNING before the button
		button   string   // the label of the 1 button
		text     []string // substrings of the text part
		html     []string // substrings of the HTML part
	}{
		{VerifyEmail{Name: "Alice", Link: link, ValidFor: 48 * time.Hour}, "Verify your email address", true, false, "Verify your address",
			[]string{"Hello, Alice. Before I can send you anything important", "What this email is for: confirm that alice@example.com", link,
				"How long the link works: 48 hours. It works once.", "If you did not ask for this: ignore this email. Nothing changes."},
			[]string{`href="https://go-practice.dev/verify-email?token=a&amp;b=%3cc%3e"`, "48 hours", "<b>alice@example.com</b>"}},
		{Recovery{Name: "Анна", Link: link, ValidFor: 15 * time.Minute}, "Recover your Go Practice account", false, false, "Add a new passkey",
			[]string{"Hello, Анна. Someone asked for a new way into this account.", link, "How long the link works: 15 minutes.", "so do not forward it"},
			[]string{"Add a new passkey", "15 minutes"}},
		{Invite{InvitedBy: "Yornik", Note: "Start with <01>", Link: link, Expires: when.Add(7 * 24 * time.Hour), ValidFor: 7 * 24 * time.Hour, Exercises: 50},
			"Yornik invited you to Go Practice", true, false, "Create your account",
			[]string{"Good evening. Yornik asked me to bring you this invite", "Go Practice: 50 Go exercises", "A note from Yornik:\n“Start with <01>”",
				"How long the link works: 7 days, until 26 October 2026. It works once.", "If you did not expect it: ignore this email.", link},
			[]string{"Go Practice: 50 Go exercises", "“Start with &lt;01&gt;”", "<b>A note from Yornik:</b>", "7 days, until 26 October 2026"}},
		{Invite{InvitedBy: "Yornik", Link: link, Expires: when}, "Yornik invited you to Go Practice", true, false, "Create your account",
			[]string{"Go Practice: Go exercises", "How long the link works: until 19 October 2026."}, nil},
		{PasskeyAdded{Name: "Alice", Passkey: "MacBook", When: when}, "A passkey was added to your account", false, true, "Open your account page",
			[]string{"What happened: a new passkey, “MacBook”, was added", "19 October 2026 at 09:30 UTC", site + "/account", "If you added it: you need to do nothing."},
			[]string{"<b>“MacBook”</b>", `href="https://go-practice.dev/account"`}},
		{PasskeyRemoved{Name: "Alice", Passkey: "Old key", When: when}, "A passkey was removed from your account", false, true, "Open your account page",
			[]string{"the passkey “Old key” was removed", "cannot sign in any more", "09:30 UTC"}, []string{"cannot sign in any more"}},
		{EmailChangedOld{EmailChange{Name: "Alice", Old: "a@example.com", New: "b@example.com", When: when, Pending: true}}, "The email address of your account is changing", false, true, "Open your account page",
			[]string{"from a@example.com to b@example.com", "takes effect when the new address is verified", "09:30 UTC"},
			[]string{"<b>b@example.com</b>", "takes effect"}},
		{EmailChangedOld{EmailChange{Name: "Alice", Old: "a@example.com", New: "b@example.com", When: when}}, "The email address of your account is changing", false, true, "Open your account page",
			[]string{"go to b@example.com"}, []string{"go to b@example.com"}},
		{EmailChangedNew{EmailChange{Name: "Alice", Old: "a@example.com", New: "b@example.com", When: when}}, "This is now your Go Practice email address", false, true, "Open your account page",
			[]string{"b@example.com became the email address", "replaces a@example.com", "09:30 UTC"}, []string{"<b>b@example.com</b> became"}},
		{RecoveryUsed{Name: "Alice", Passkey: "Phone", When: when, RemovedOthers: 2}, "Your account was recovered", false, true, "Recover my account",
			[]string{"a recovery link sent to this address added a new passkey, “Phone”", "2 old passkeys were removed", site + "/recover",
				"What the link does: it opens the recovery page. It does not sign you in, and it holds no token."},
			[]string{"2 old passkeys were removed"}},
		{RecoveryUsed{Name: "Alice", Passkey: "Phone", When: when, RemovedOthers: 1}, "Your account was recovered", false, true, "Recover my account",
			[]string{"1 old passkey was removed"}, nil},
		{RecoveryUsed{Name: "Alice", Passkey: "Phone", When: when}, "Your account was recovered", false, true, "Recover my account",
			[]string{"If this was you: you need to do nothing."}, nil},
	}
	seen := map[string]bool{}
	for _, tt := range tests {
		name := tt.email.templateName()
		seen[name] = true
		m, err := Compose(site+"/", "alice@example.com", tt.email)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if m.Subject != tt.subject {
			t.Errorf("%s: subject %q, want %q", name, m.Subject, tt.subject)
		}
		for _, want := range tt.text {
			if !strings.Contains(m.Text, want) {
				t.Errorf("%s: text lacks %q:\n%s", name, want, m.Text)
			}
		}
		for _, want := range tt.html {
			if !strings.Contains(m.HTML, want) {
				t.Errorf("%s: HTML lacks %q", name, want)
			}
		}
		checkStructure(t, name, m, tt.rest, tt.security, tt.button)
	}
	for _, name := range templateNames {
		if !seen[name] {
			t.Errorf("template %s has no test", name)
		}
	}

	if _, err := Compose(site, "not an address", VerifyEmail{}); err == nil {
		t.Error("Compose accepted a bad recipient")
	}
	// A display name with a line break cannot add a header.
	m, err := Compose(site, "a@example.com", Invite{InvitedBy: "Eve\r\nBcc: x@example.com", Link: link, Expires: when})
	if err != nil || strings.ContainsAny(m.Subject, "\r\n") {
		t.Errorf("subject %q, %v", m.Subject, err)
	}
}

// checkStructure checks the structure of VESPER.md 13.2 that every mail
// shares: the heading is the subject, HUSH's paragraph sits in its own
// cyan-ruled block that holds no link, the plain part follows it, 1 amber
// button with white text names the action and the full URL stands under
// it, the footer says that replies are not read, and the markup is mail
// markup: inline styles, no images, no web fonts, light only.
func checkStructure(t *testing.T, name string, m Message, rest, security bool, button string) {
	t.Helper()
	// The plain-text part, in its order.
	lines := strings.Split(m.Text, "\n")
	if len(lines) < 4 || lines[0] != m.Subject || lines[1] != "" || lines[2] != "  HUSH, Vesper Labs:" || !strings.HasPrefix(lines[3], "  ") {
		t.Errorf("%s: the text part does not open with the heading and HUSH's paragraph:\n%s", name, m.Text)
	}
	hushEnd := strings.Index(m.Text, "\n\n"+map[bool]string{true: "What happened:", false: "What this email is for:"}[security])
	if hushEnd < 0 {
		t.Errorf("%s: the plain part does not start with its lead:\n%s", name, m.Text)
		return
	}
	hush := m.Text[:hushEnd]
	if strings.Contains(hush, "http") || strings.Contains(hush, "@") {
		t.Errorf("%s: HUSH's paragraph holds a link or an address:\n%s", name, hush)
	}
	if strings.Contains(hush, "Please rest.") != rest || strings.Contains(m.HTML, "Please rest.") != rest {
		t.Errorf("%s: \"Please rest.\" is %v, want %v", name, !rest, rest)
	}
	at := func(s string) int { return strings.Index(m.Text, s) }
	if at(button+":\n") < 0 {
		t.Errorf("%s: the text part lacks %q with its URL", name, button+":")
	}
	if security {
		if w := at("\nWARNING: "); w < 0 || w > at(button+":") || at("What happened:") > w {
			t.Errorf("%s: a security notice needs What happened, then the WARNING, then the button:\n%s", name, m.Text)
		}
	} else if strings.Contains(m.Text, "WARNING") {
		t.Errorf("%s: a WARNING outside a security notice", name)
	}
	if !strings.HasSuffix(m.Text, "\n--\nReplies to this address are not read.\nGo Practice sent this email automatically.\nhttps://go-practice.dev/\n") ||
		strings.Contains(m.Text, "\n\n\n") {
		t.Errorf("%s: text footer or blank lines:\n%s", name, m.Text)
	}

	// The HTML part.
	h := m.HTML
	for _, want := range []string{
		"<title>" + htmlEscape(m.Subject) + "</title>",
		`color:#070707;">` + htmlEscape(m.Subject) + "</h1>",
		`<meta name="color-scheme" content="light only">`, `<meta name="supported-color-schemes" content="light only">`,
		"height:2px;line-height:2px;font-size:0;background-color:#006B7D;", // the 2 px cyan top rule
		"border-left:3px solid #006B7D;background-color:#E3F3F6;",          // HUSH's block
		`<b>Vesper</b> Labs`, // the text wordmark
		`style="margin:0 0 16px;font-size:16px;line-height:24px;color:#070707;"`,                // the shared paragraph
		`<b style="color:#070707;">Replies to this address are not read.</b>`,                   // the footer
		"<!--[if mso]><table role=\"presentation\" width=\"560\"",                               // Outlook holds the card at 560 px
		"background-color:#AD4F00;", "color:#FFFFFF;text-decoration:none;\">" + button + "</a>", // the 1 button
		"word-break:break-all;", // the full URL under it
	} {
		if !strings.Contains(h, want) {
			t.Errorf("%s: HTML lacks %q", name, want)
		}
	}
	if n := strings.Count(h, `bgcolor="#AD4F00"`); n != 1 {
		t.Errorf("%s: %d buttons, want 1", name, n)
	}
	block := h[strings.Index(h, "HUSH, Vesper Labs"):]
	block = block[:strings.Index(block, "</td></tr></table>")]
	if strings.Contains(block, "<a ") || strings.Contains(block, "http") {
		t.Errorf("%s: HUSH's block holds a link: %s", name, block)
	}
	if strings.Index(h, "HUSH, Vesper Labs") > strings.Index(h, `bgcolor="#AD4F00"`) {
		t.Errorf("%s: the button is inside or before HUSH's block", name)
	}
	for _, bad := range []string{"<img", "<style", "class=", "@font-face", "url(", "<script", "border-radius", "ZgotmplZ", "<c>"} {
		if strings.Contains(h, bad) {
			t.Errorf("%s: HTML holds %q", name, bad)
		}
	}
	// No unrendered actions, no contractions, no exclamation marks in the
	// words of the mail.
	for _, part := range []string{m.Text, m.Subject, visibleText(h)} {
		if strings.Contains(part, "{{") || strings.Contains(part, "n't") || strings.Contains(part, "<no value>") || strings.Contains(part, "!") {
			t.Errorf("%s: %q", name, part)
		}
	}
}

// htmlEscape escapes like html/template does in text.
func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;").Replace(s)
}

// visibleText drops the tags, the comments and the doctype of an HTML part.
func visibleText(h string) string {
	var b strings.Builder
	in := false
	for _, r := range h {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestLifetime(t *testing.T) {
	tests := map[time.Duration]string{
		15 * time.Minute:   "15 minutes",
		time.Minute:        "1 minute",
		time.Hour:          "1 hour",
		2 * time.Hour:      "2 hours",
		90 * time.Minute:   "90 minutes",
		24 * time.Hour:     "24 hours",
		48 * time.Hour:     "48 hours", // auth.VerifyTTL, as the check-email page writes it
		72 * time.Hour:     "3 days",
		7 * 24 * time.Hour: "7 days",
	}
	for d, want := range tests {
		if got := Lifetime(d); got != want {
			t.Errorf("Lifetime(%v) = %q, want %q", d, got, want)
		}
	}
}

// TestFooterNamesTheSender sends through a Mailer whose sender knows its
// address: the footer names it in both parts.
func TestFooterNamesTheSender(t *testing.T) {
	var buf bytes.Buffer
	s, err := NewSender(Config{From: DefaultFrom}, slog.New(slog.NewJSONHandler(&buf, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ls := s.(*LogSender)
	m, err := compose(site, ls.Address(), "alice@example.com", VerifyEmail{Name: "Alice", Link: site + "/verify-email?token=x", ValidFor: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Text, "\nGo Practice sent this email automatically from hush@vesper-labs.org.\n") ||
		!strings.Contains(m.HTML, "Go Practice sent this email automatically from hush@vesper-labs.org. The site is <a href=\"https://go-practice.dev/\"") {
		t.Errorf("the footer does not name the sender:\n%s", m.Text)
	}
	if err := NewMailer(s, site).Send(context.Background(), "alice@example.com", VerifyEmail{Name: "Alice", Link: site + "/verify-email?token=x", ValidFor: time.Hour}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "automatically from hush@vesper-labs.org.") {
		t.Errorf("the Mailer did not pass the sender's address: %s", buf.String())
	}
}

// parsed is an encoded message read back with the standard library.
type parsed struct {
	header mail.Header
	text   string
	html   string
}

func parse(t *testing.T, raw []byte) parsed {
	t.Helper()
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	mt, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/alternative" {
		t.Fatalf("Content-Type %q: %v", msg.Header.Get("Content-Type"), err)
	}
	p := parsed{header: msg.Header}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	var parts []string
	for {
		part, err := mr.NextPart() // decodes quoted-printable
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, part.Header.Get("Content-Type"))
		switch part.Header.Get("Content-Type") {
		case "text/plain; charset=utf-8":
			p.text = string(b)
		case "text/html; charset=utf-8":
			p.html = string(b)
		}
	}
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "text/plain") || !strings.HasPrefix(parts[1], "text/html") {
		t.Fatalf("parts %q, want text then HTML", parts)
	}
	return p
}

func TestEncode(t *testing.T) {
	from, err := ParseFrom(DefaultFrom)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Compose(site, "alice@example.com", VerifyEmail{Name: "Ålice", Link: site + "/verify-email?token=" + strings.Repeat("x", 120), ValidFor: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	m.Subject = "Verify your address, Ålice"
	raw, err := m.Encode(from, when)
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range bytes.Split(raw, []byte("\r\n")) {
		if len(line) > 998 || bytes.ContainsRune(line, '\n') {
			t.Fatalf("line %d is too long or has a bare LF: %q", i, line)
		}
	}
	p := parse(t, raw)
	dec := new(mime.WordDecoder)
	subject, err := dec.DecodeHeader(p.header.Get("Subject"))
	if err != nil || subject != m.Subject {
		t.Errorf("Subject = %q (%v), want %q", subject, err, m.Subject)
	}
	// The display name holds a middle dot, so it goes as an RFC 2047
	// word: a header carries ASCII only.
	rawFrom := p.header.Get("From")
	if !strings.HasPrefix(rawFrom, "=?utf-8?") || strings.ContainsFunc(rawFrom, func(r rune) bool { return r > 127 }) {
		t.Errorf("From = %q, want an encoded word", rawFrom)
	}
	if a, err := mail.ParseAddress(rawFrom); err != nil || a.Name != "HUSH · Vesper Labs" || a.Address != "hush@vesper-labs.org" {
		t.Errorf("From reads as %+v, %v", a, err)
	}
	checks := map[string]string{
		"To":             "<alice@example.com>",
		"Date":           "Mon, 19 Oct 2026 09:30:00 +0000",
		"MIME-Version":   "1.0",
		"Auto-Submitted": "auto-generated",
	}
	for k, want := range checks {
		if got := p.header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	id := p.header.Get("Message-ID")
	if !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, "@vesper-labs.org>") {
		t.Errorf("Message-ID = %q", id)
	}
	if date, err := p.header.Date(); err != nil || !date.Equal(when) {
		t.Errorf("Date = %v, %v", date, err)
	}
	// Quoted-printable round trip: the long link comes back whole.
	if strings.ReplaceAll(p.text, "\r\n", "\n") != m.Text {
		t.Errorf("text part changed in transit:\n%q\n%q", p.text, m.Text)
	}
	if strings.ReplaceAll(p.html, "\r\n", "\n") != m.HTML {
		t.Error("HTML part changed in transit")
	}

	again, _ := m.Encode(from, when)
	if parse(t, again).header.Get("Message-ID") == id {
		t.Error("2 messages share a Message-ID")
	}

	bad := []Message{
		{To: "alice@example.com\r\nBcc: eve@example.com", Subject: "x", Text: "t", HTML: "h"},
		{To: "alice@example.com", Subject: "x\nBcc: eve@example.com", Text: "t", HTML: "h"},
		{To: "nobody", Subject: "x", Text: "t", HTML: "h"},
		{To: "alice@example.com", Subject: "x", Text: "", HTML: "h"},
	}
	for _, b := range bad {
		if _, err := b.Encode(from, when); err == nil {
			t.Errorf("Encode(%+v) succeeded", b)
		}
	}
}

func TestImplicitTLSPort(t *testing.T) {
	for port, want := range map[int]bool{465: true, 587: false, 25: false} {
		s, err := NewSMTPSender(Config{Host: "smtp.resend.com", Port: port, From: "a@yornik.eu"})
		if err != nil || s.implicitTLS != want {
			t.Errorf("port %d: implicit TLS %v, %v", port, s.implicitTLS, err)
		}
	}
}

func TestNewSender(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	tests := []struct {
		cfg     Config
		wantLog bool
		wantErr bool
	}{
		{Config{From: "Go Practice <go-practice@yornik.eu>"}, true, false},
		{Config{Host: "smtp.resend.com", Port: 465, Username: "resend", Password: "REPLACE_ME", From: "a@b.c"}, true, false},
		{Config{Host: "smtp.resend.com", Port: 465, Username: "resend", From: "a@b.c"}, true, false},
		{Config{Host: "smtp.resend.com", Port: 465, Username: "resend", Password: "re_123", From: "a@b.c"}, false, false},
		{Config{Host: "smtp.resend.com", Port: 0, Password: "re_123", From: "a@b.c"}, false, true},
		{Config{Host: "smtp.resend.com", Port: 465, Password: "re_123", From: "not an address"}, false, true},
	}
	for i, tt := range tests {
		s, err := NewSender(tt.cfg, logger)
		if (err != nil) != tt.wantErr {
			t.Errorf("%d: err = %v", i, err)
			continue
		}
		if err != nil {
			continue
		}
		if _, isLog := s.(*LogSender); isLog != tt.wantLog {
			t.Errorf("%d: %T, want log sender %v", i, s, tt.wantLog)
		}
		// Only the SMTP sender delivers: a mailed link proves an address
		// only when it left the server.
		if Delivers(s) == tt.wantLog || NewMailer(s, site).Delivers() == tt.wantLog {
			t.Errorf("%d: %T delivers %v, want %v", i, s, Delivers(s), !tt.wantLog)
		}
	}
}

// quietSender sends nothing and does not say whether it delivers.
type quietSender struct{}

func (quietSender) Send(context.Context, Message) error { return nil }

func TestDeliversNeedsTheClaim(t *testing.T) {
	if Delivers(quietSender{}) || NewMailer(quietSender{}, site).Delivers() {
		t.Error("a sender that does not claim delivery counts as delivering")
	}
	if Delivers(NewLogSender(slog.New(slog.DiscardHandler))) {
		t.Error("the log sender counts as delivering")
	}
}

func TestLogSender(t *testing.T) {
	var buf bytes.Buffer
	s := NewLogSender(slog.New(slog.NewJSONHandler(&buf, nil)))
	m, err := Compose(site, "alice@example.com", Recovery{Name: "Alice", Link: site + "/recover/confirm?token=abc", ValidFor: 15 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	mailer := NewMailer(s, site)
	if err := mailer.Send(context.Background(), "alice@example.com", Recovery{Name: "Alice", Link: site + "/recover/confirm?token=abc", ValidFor: 15 * time.Minute}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{`"to":"alice@example.com"`, `"subject":"` + m.Subject + `"`, "/recover/confirm?token=abc"} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q: %s", want, out)
		}
	}
	if err := s.Send(context.Background(), Message{To: "bad"}); err == nil {
		t.Error("log sender accepted a bad recipient")
	}
}

// fakeSMTP is a minimal SMTP server for tests: EHLO, STARTTLS, AUTH PLAIN,
// MAIL, RCPT, DATA, QUIT.
type fakeSMTP struct {
	t           *testing.T
	ln          net.Listener
	tlsConfig   *tls.Config
	implicitTLS bool
	offerTLS    bool
	user, pass  string
	hang        bool // stop answering after the greeting

	mu       sync.Mutex
	from     string
	rcpt     []string
	data     []byte
	authOK   bool
	sawTLS   bool
	sessions int
}

// newFakeSMTP starts a server; configure runs before it accepts.
func newFakeSMTP(t *testing.T, implicitTLS bool, configure ...func(*fakeSMTP)) (*fakeSMTP, *x509.CertPool) {
	t.Helper()
	cert, pool := selfSigned(t)
	f := &fakeSMTP{t: t, implicitTLS: implicitTLS, offerTLS: true, user: "resend", pass: "re_secret",
		tlsConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}}
	for _, c := range configure {
		c(f)
	}
	var err error
	if implicitTLS {
		f.ln, err = tls.Listen("tcp", "127.0.0.1:0", f.tlsConfig)
	} else {
		f.ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.ln.Close() })
	go f.serve()
	return f, pool
}

func (f *fakeSMTP) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeSMTP) serve() {
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.session(c)
	}
}

func (f *fakeSMTP) session(c net.Conn) {
	defer func() { _ = c.Close() }()
	f.mu.Lock()
	f.sessions++
	f.mu.Unlock()
	_, isTLS := c.(*tls.Conn)
	r, w := bufio.NewReader(c), bufio.NewWriter(c)
	reply := func(s string) {
		_, _ = w.WriteString(s + "\r\n")
		_ = w.Flush()
	}
	reply("220 localhost ESMTP fake")
	if f.hang {
		_, _ = io.Copy(io.Discard, r)
		return
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.TrimRight(line, "\r\n")
		verb := strings.ToUpper(strings.SplitN(cmd, " ", 2)[0])
		switch verb {
		case "EHLO", "HELO":
			lines := []string{"250-localhost"}
			if f.offerTLS && !isTLS {
				lines = append(lines, "250-STARTTLS")
			}
			lines = append(lines, "250-AUTH PLAIN", "250 8BITMIME")
			reply(strings.Join(lines, "\r\n"))
		case "STARTTLS":
			reply("220 go ahead")
			tc := tls.Server(c, f.tlsConfig)
			if err := tc.Handshake(); err != nil {
				return
			}
			c, isTLS = tc, true
			r, w = bufio.NewReader(tc), bufio.NewWriter(tc)
		case "AUTH":
			fields := strings.Fields(cmd)
			ok := false
			if len(fields) == 3 && fields[1] == "PLAIN" {
				b, _ := base64.StdEncoding.DecodeString(fields[2])
				ok = string(b) == "\x00"+f.user+"\x00"+f.pass
			}
			f.mu.Lock()
			f.authOK, f.sawTLS = ok, isTLS
			f.mu.Unlock()
			if ok {
				reply("235 ok")
			} else {
				reply("535 bad credentials")
			}
		case "MAIL":
			f.mu.Lock()
			f.from = cmd
			f.mu.Unlock()
			reply("250 ok")
		case "RCPT":
			f.mu.Lock()
			f.rcpt = append(f.rcpt, cmd)
			f.mu.Unlock()
			reply("250 ok")
		case "DATA":
			reply("354 go ahead")
			var data bytes.Buffer
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				data.WriteString(strings.TrimPrefix(l, "."))
			}
			f.mu.Lock()
			f.data = data.Bytes()
			f.mu.Unlock()
			reply("250 queued")
		case "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 not implemented")
		}
	}
}

func selfSigned(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

func TestSMTPSend(t *testing.T) {
	for _, implicit := range []bool{true, false} {
		t.Run(fmt.Sprint("implicitTLS=", implicit), func(t *testing.T) {
			srv, pool := newFakeSMTP(t, implicit)
			cfg := Config{Host: "127.0.0.1", Port: srv.port(), Username: "resend", Password: "re_secret",
				From: "Go Practice <go-practice@yornik.eu>", TLSConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
			s, err := NewSMTPSender(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if s.implicitTLS {
				t.Fatal("implicit TLS on a port other than 465")
			}
			// Port 465 decides it in production; the test server has a random port.
			s.implicitTLS = implicit
			s.now = func() time.Time { return when }
			mailer := NewMailer(s, site)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := mailer.Send(ctx, "alice@example.com", PasskeyAdded{Name: "Alice", Passkey: "MacBook", When: when}); err != nil {
				t.Fatal(err)
			}
			srv.mu.Lock()
			defer srv.mu.Unlock()
			if !srv.authOK || !srv.sawTLS {
				t.Errorf("auth ok %v over TLS %v", srv.authOK, srv.sawTLS)
			}
			if srv.from != "MAIL FROM:<go-practice@yornik.eu> BODY=8BITMIME" && srv.from != "MAIL FROM:<go-practice@yornik.eu>" {
				t.Errorf("from = %q", srv.from)
			}
			if len(srv.rcpt) != 1 || srv.rcpt[0] != "RCPT TO:<alice@example.com>" {
				t.Errorf("rcpt = %q", srv.rcpt)
			}
			p := parse(t, srv.data)
			// The footer names the address the mail came from.
			if p.header.Get("Subject") != "A passkey was added to your account" || !strings.Contains(p.text, "“MacBook”") ||
				!strings.Contains(p.html, "<b>“MacBook”</b>") || !strings.Contains(p.text, "automatically from go-practice@yornik.eu.") {
				t.Errorf("delivered message:\n%s", srv.data)
			}
		})
	}
}

func TestSMTPRefusals(t *testing.T) {
	ctx := context.Background()
	t.Run("no STARTTLS", func(t *testing.T) {
		srv, pool := newFakeSMTP(t, false, func(f *fakeSMTP) { f.offerTLS = false })
		s, _ := NewSMTPSender(Config{Host: "127.0.0.1", Port: srv.port(), Username: "resend", Password: "re_secret",
			From: "a@yornik.eu", TLSConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}})
		err := s.Send(ctx, Message{To: "alice@example.com", Subject: "x", Text: "t", HTML: "h"})
		if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
			t.Errorf("Send without STARTTLS: %v", err)
		}
		srv.mu.Lock()
		defer srv.mu.Unlock()
		if srv.authOK || srv.from != "" {
			t.Error("credentials or mail went over a plain connection")
		}
	})
	t.Run("untrusted certificate", func(t *testing.T) {
		srv, _ := newFakeSMTP(t, false)
		s, _ := NewSMTPSender(Config{Host: "127.0.0.1", Port: srv.port(), Username: "resend", Password: "re_secret", From: "a@yornik.eu"})
		if err := s.Send(ctx, Message{To: "alice@example.com", Subject: "x", Text: "t", HTML: "h"}); err == nil {
			t.Error("Send trusted a self-signed certificate")
		}
	})
	t.Run("wrong password", func(t *testing.T) {
		srv, pool := newFakeSMTP(t, false)
		s, _ := NewSMTPSender(Config{Host: "127.0.0.1", Port: srv.port(), Username: "resend", Password: "wrong",
			From: "a@yornik.eu", TLSConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}})
		if err := s.Send(ctx, Message{To: "alice@example.com", Subject: "x", Text: "t", HTML: "h"}); err == nil || !strings.Contains(err.Error(), "auth") {
			t.Errorf("Send with a wrong password: %v", err)
		}
	})
	t.Run("server hangs", func(t *testing.T) {
		srv, pool := newFakeSMTP(t, false, func(f *fakeSMTP) { f.hang = true })
		s, _ := NewSMTPSender(Config{Host: "127.0.0.1", Port: srv.port(), Username: "resend", Password: "re_secret",
			From: "a@yornik.eu", TLSConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, Timeout: time.Minute})
		ctx, cancel := context.WithCancel(ctx)
		time.AfterFunc(100*time.Millisecond, cancel)
		done := make(chan error, 1)
		go func() { done <- s.Send(ctx, Message{To: "alice@example.com", Subject: "x", Text: "t", HTML: "h"}) }()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Send to a hanging server: %v, want context.Canceled", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Send ignored the cancelled context")
		}
	})
	t.Run("bad message", func(t *testing.T) {
		s, _ := NewSMTPSender(Config{Host: "127.0.0.1", Port: 1, From: "a@yornik.eu"})
		if err := s.Send(ctx, Message{To: "x\r\ny", Subject: "x", Text: "t", HTML: "h"}); err == nil {
			t.Error("Send accepted a bad recipient")
		}
	})
}
