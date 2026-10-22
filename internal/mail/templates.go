package mail

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	htmltemplate "html/template"
	"net/url"
	"strings"
	texttemplate "text/template"
	"time"
)

// Email is 1 of the emails the site sends. The types in this file
// implement it; each has a text and an HTML template.
type Email interface {
	templateName() string
}

// VerifyEmail asks the user to confirm an address: after sign-up and after
// an address change.
type VerifyEmail struct {
	Name     string // the display name
	Link     string // the full verification URL
	ValidFor time.Duration
}

// Recovery carries a recovery link.
type Recovery struct {
	Name     string
	Link     string
	ValidFor time.Duration
}

// Invite invites someone to sign up.
type Invite struct {
	InvitedBy string // the admin's display name
	Note      string // the admin's note; may be empty
	Link      string
	Expires   time.Time
	ValidFor  time.Duration // how long the invite works; 0 leaves it out
	Exercises int           // the number of exercises in the course; 0 leaves it out
}

// PasskeyAdded is the security notice for a new passkey.
type PasskeyAdded struct {
	Name    string
	Passkey string // the passkey's label
	When    time.Time
}

// PasskeyRemoved is the security notice for a removed passkey.
type PasskeyRemoved struct {
	Name    string
	Passkey string
	When    time.Time
}

// EmailChange describes a change of address for the 2 notices about it.
type EmailChange struct {
	Name     string
	Old, New string
	When     time.Time
	// Pending is true while the new address is not verified: the change
	// takes effect only when it is.
	Pending bool
}

// EmailChangedOld goes to the old address.
type EmailChangedOld struct{ EmailChange }

// EmailChangedNew goes to the new address.
type EmailChangedNew struct{ EmailChange }

// RecoveryUsed is the security notice after a recovery added a passkey.
type RecoveryUsed struct {
	Name          string
	Passkey       string // the new passkey's label
	When          time.Time
	RemovedOthers int // the number of old passkeys the user removed
}

func (VerifyEmail) templateName() string     { return "verify" }
func (Recovery) templateName() string        { return "recovery" }
func (Invite) templateName() string          { return "invite" }
func (PasskeyAdded) templateName() string    { return "passkey-added" }
func (PasskeyRemoved) templateName() string  { return "passkey-removed" }
func (EmailChangedOld) templateName() string { return "email-changed-old" }
func (EmailChangedNew) templateName() string { return "email-changed-new" }
func (RecoveryUsed) templateName() string    { return "recovery-used" }

//go:embed templates
var templateFS embed.FS

// templateNames lists every email; the tests render each one.
var templateNames = []string{
	"verify", "recovery", "invite", "passkey-added", "passkey-removed",
	"email-changed-old", "email-changed-new", "recovery-used",
}

type templateSet struct {
	text *texttemplate.Template // defines "subject", "hush", "rest" and "layout"
	html *htmltemplate.Template // defines "layout"
}

// Lifetime writes how long a link works, for a sentence: "15 minutes",
// "1 hour", "48 hours", "7 days". The check-email page says the same with
// it, so the page and the mail never disagree. Days start at 3, so the
// verification link reads "48 hours" in both.
func Lifetime(d time.Duration) string {
	n, unit := int(d/time.Minute), "minute"
	switch {
	case d >= 72*time.Hour && d%(24*time.Hour) == 0:
		n, unit = int(d/(24*time.Hour)), "day"
	case d >= time.Hour && d%time.Hour == 0:
		n, unit = int(d/time.Hour), "hour"
	}
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// action is the 1 button of a mail and the full URL under it.
type action struct {
	Label string // names the action: "Verify your address"
	URL   string
}

var funcs = map[string]any{
	// when formats a moment the way the emails write it, in UTC because
	// the site does not know the reader's time zone.
	"when":     func(t time.Time) string { return t.UTC().Format("2 January 2006 at 15:04 UTC") },
	"date":     func(t time.Time) string { return t.UTC().Format("2 January 2006") },
	"lifetime": Lifetime,
	"plural": func(n int, one, many string) string {
		if n == 1 {
			return one
		}
		return many
	},
	"action": func(label, url string) action { return action{label, url} },
}

// htmlFuncs add Outlook's conditional comments, which hold the card at
// 560 px there because Outlook on Windows ignores max-width.
// html/template drops comments from template text, so they come in as
// trusted constants.
var htmlFuncs = map[string]any{
	"msoOpen": func() htmltemplate.HTML {
		return `<!--[if mso]><table role="presentation" width="560" align="center" cellpadding="0" cellspacing="0" border="0"><tr><td><![endif]-->`
	},
	"msoClose": func() htmltemplate.HTML { return `<!--[if mso]></td></tr></table><![endif]-->` },
}

// templates are parsed once: they are embedded, so a parse error is a bug
// that the tests catch, not a runtime condition.
var templates = func() map[string]templateSet {
	sets := map[string]templateSet{}
	for _, name := range templateNames {
		sets[name] = templateSet{
			text: texttemplate.Must(texttemplate.New(name).Funcs(funcs).
				ParseFS(templateFS, "templates/layout.txt", "templates/"+name+".txt")),
			html: htmltemplate.Must(htmltemplate.New(name).Funcs(funcs).Funcs(htmlFuncs).
				ParseFS(templateFS, "templates/layout.html", "templates/"+name+".html")),
		}
	}
	return sets
}()

// view is what every template sees.
type view struct {
	SiteURL string // without a trailing slash
	Host    string // the site's address for people: "vesper-labs.org"
	From    string // the sender's address, "hush@vesper-labs.org"; "" leaves it out
	To      string
	E       Email
	// Rendered once from the text templates, so both parts say the same:
	Subject string // the subject, and the heading of both parts
	Hush    string // HUSH's paragraph
	Rest    bool   // HUSH ends with "Please rest."
}

// Compose renders e for the recipient to. siteURL is the public URL of the
// site, for the links to the account page and the footer.
func Compose(siteURL, to string, e Email) (Message, error) {
	return compose(siteURL, "", to, e)
}

// compose renders e; from is the sender's address for the footer.
func compose(siteURL, from, to string, e Email) (Message, error) {
	set, ok := templates[e.templateName()]
	if !ok {
		return Message{}, fmt.Errorf("mail: no template %q", e.templateName())
	}
	site := strings.TrimSuffix(siteURL, "/")
	v := view{SiteURL: site, Host: site, From: from, To: to, E: e}
	if u, err := url.Parse(site); err == nil && u.Host != "" {
		v.Host = u.Host
	}
	line := func(name string) (string, error) {
		var b bytes.Buffer
		if err := set.text.ExecuteTemplate(&b, name, v); err != nil {
			return "", fmt.Errorf("mail: render %s %s: %w", e.templateName(), name, err)
		}
		return strings.Join(strings.Fields(b.String()), " "), nil
	}
	var err error
	if v.Subject, err = line("subject"); err != nil {
		return Message{}, err
	}
	if v.Hush, err = line("hush"); err != nil {
		return Message{}, err
	}
	rest, err := line("rest")
	if err != nil {
		return Message{}, err
	}
	v.Rest = rest != ""
	var text, html bytes.Buffer
	if err := set.text.ExecuteTemplate(&text, "layout", v); err != nil {
		return Message{}, fmt.Errorf("mail: render %s text: %w", e.templateName(), err)
	}
	if err := set.html.ExecuteTemplate(&html, "layout", v); err != nil {
		return Message{}, fmt.Errorf("mail: render %s HTML: %w", e.templateName(), err)
	}
	m := Message{
		To:      to,
		Subject: v.Subject,
		Text:    tidyText(text.String()),
		HTML:    html.String(),
	}
	if _, err := m.recipient(); err != nil {
		return Message{}, err
	}
	return m, nil
}

// tidyText trims the blank lines that template actions leave behind.
func tidyText(s string) string {
	var out []string
	blank := true // drops leading blank lines
	for line := range strings.Lines(s) {
		line = strings.TrimRight(line, " \t\r\n")
		if line == "" {
			if blank {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}

// Mailer renders and sends the site's emails.
type Mailer struct {
	sender  Sender
	siteURL string
}

// addresser is a sender that knows its From address, for the footer.
type addresser interface{ Address() string }

// NewMailer returns a Mailer that sends with s. siteURL is GPW_PUBLIC_URL.
func NewMailer(s Sender, siteURL string) *Mailer {
	return &Mailer{sender: s, siteURL: siteURL}
}

// Delivers reports whether the Mailer's sender hands messages to a mail
// server; see the function Delivers.
func (m *Mailer) Delivers() bool { return Delivers(m.sender) }

// Send renders e for to and sends it.
func (m *Mailer) Send(ctx context.Context, to string, e Email) error {
	from := ""
	if a, ok := m.sender.(addresser); ok {
		from = a.Address()
	}
	msg, err := compose(m.siteURL, from, to, e)
	if err != nil {
		return err
	}
	return m.sender.Send(ctx, msg)
}
