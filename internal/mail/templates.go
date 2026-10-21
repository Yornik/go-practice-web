package mail

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	htmltemplate "html/template"
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
	Exercises int // the number of exercises in the course; 0 leaves it out
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
	text *texttemplate.Template // defines "subject" and "layout"
	html *htmltemplate.Template // defines "layout"
}

var funcs = map[string]any{
	// when formats a moment the way the emails write it, in UTC because
	// the site does not know the reader's time zone.
	"when": func(t time.Time) string { return t.UTC().Format("2 January 2006 at 15:04 UTC") },
	"date": func(t time.Time) string { return t.UTC().Format("2 January 2006") },
	"duration": func(d time.Duration) string {
		switch {
		case d >= 48*time.Hour && d%(24*time.Hour) == 0:
			return fmt.Sprintf("%d days", d/(24*time.Hour))
		case d >= 2*time.Hour && d%time.Hour == 0:
			return fmt.Sprintf("%d hours", d/time.Hour)
		case d == time.Hour:
			return "1 hour"
		default:
			return fmt.Sprintf("%d minutes", d/time.Minute)
		}
	},
	"plural": func(n int, one, many string) string {
		if n == 1 {
			return one
		}
		return many
	},
}

// templates are parsed once: they are embedded, so a parse error is a bug
// that the tests catch, not a runtime condition.
var templates = func() map[string]templateSet {
	sets := map[string]templateSet{}
	for _, name := range templateNames {
		sets[name] = templateSet{
			text: texttemplate.Must(texttemplate.New(name).Funcs(funcs).
				ParseFS(templateFS, "templates/layout.txt", "templates/"+name+".txt")),
			html: htmltemplate.Must(htmltemplate.New(name).Funcs(funcs).
				ParseFS(templateFS, "templates/layout.html", "templates/"+name+".html")),
		}
	}
	return sets
}()

// view is what every template sees.
type view struct {
	SiteURL string // without a trailing slash
	To      string
	E       Email
}

// Compose renders e for the recipient to. siteURL is the public URL of the
// site, for the links to the account page and the footer.
func Compose(siteURL, to string, e Email) (Message, error) {
	set, ok := templates[e.templateName()]
	if !ok {
		return Message{}, fmt.Errorf("mail: no template %q", e.templateName())
	}
	v := view{SiteURL: strings.TrimSuffix(siteURL, "/"), To: to, E: e}
	var subject, text, html bytes.Buffer
	if err := set.text.ExecuteTemplate(&subject, "subject", v); err != nil {
		return Message{}, fmt.Errorf("mail: render %s subject: %w", e.templateName(), err)
	}
	if err := set.text.ExecuteTemplate(&text, "layout", v); err != nil {
		return Message{}, fmt.Errorf("mail: render %s text: %w", e.templateName(), err)
	}
	if err := set.html.ExecuteTemplate(&html, "layout", v); err != nil {
		return Message{}, fmt.Errorf("mail: render %s HTML: %w", e.templateName(), err)
	}
	m := Message{
		To:      to,
		Subject: strings.Join(strings.Fields(subject.String()), " "),
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

// NewMailer returns a Mailer that sends with s. siteURL is GPW_PUBLIC_URL.
func NewMailer(s Sender, siteURL string) *Mailer {
	return &Mailer{sender: s, siteURL: siteURL}
}

// Delivers reports whether the Mailer's sender hands messages to a mail
// server; see the function Delivers.
func (m *Mailer) Delivers() bool { return Delivers(m.sender) }

// Send renders e for to and sends it.
func (m *Mailer) Send(ctx context.Context, to string, e Email) error {
	msg, err := Compose(m.siteURL, to, e)
	if err != nil {
		return err
	}
	return m.sender.Send(ctx, msg)
}
