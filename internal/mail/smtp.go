package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"time"
)

// placeholderPassword is the value the deployment's Secret holds until the
// operator fills in the real API key.
const placeholderPassword = "REPLACE_ME"

// Config configures the SMTP sender.
type Config struct {
	Host     string // smtp.resend.com
	Port     int    // 465 for implicit TLS; any other port uses STARTTLS
	Username string // resend
	Password string // the API key
	From     string // "Go Practice <go-practice@yornik.eu>"

	// TLSConfig overrides the TLS settings, for tests with their own CA.
	// nil uses the system roots. ServerName is always Host.
	TLSConfig *tls.Config
	// Timeout bounds 1 delivery: connect, TLS, SMTP dialogue. 0 means 30 s.
	Timeout time.Duration
}

// Enabled reports whether c can send mail. An empty host or password, or
// the placeholder password, means mail goes to the log instead.
func (c Config) Enabled() bool {
	return c.Host != "" && c.Password != "" && c.Password != placeholderPassword
}

// NewSender returns an SMTP sender when cfg is enabled, else a log sender.
// It fails only on an invalid From or port.
func NewSender(cfg Config, logger *slog.Logger) (Sender, error) {
	if _, err := ParseFrom(cfg.From); err != nil {
		return nil, err
	}
	if !cfg.Enabled() {
		logger.Warn("SMTP is not configured: emails are written to the log, not sent")
		return NewLogSender(logger), nil
	}
	return NewSMTPSender(cfg)
}

// SMTPSender delivers mail over SMTP with TLS and PLAIN authentication.
// It opens 1 connection per message: the site sends a few emails a day.
type SMTPSender struct {
	cfg         Config
	from        *mail.Address
	implicitTLS bool // TLS from the first byte; true for port 465
	now         func() time.Time
}

// NewSMTPSender validates cfg and returns a sender. It does not connect.
func NewSMTPSender(cfg Config) (*SMTPSender, error) {
	from, err := ParseFrom(cfg.From)
	if err != nil {
		return nil, err
	}
	if cfg.Host == "" || cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("mail: bad SMTP address %q port %d", cfg.Host, cfg.Port)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &SMTPSender{cfg: cfg, from: from, implicitTLS: cfg.Port == implicitTLSPort, now: time.Now}, nil
}

// implicitTLSPort is SMTPS: TLS from the first byte (RFC 8314).
const implicitTLSPort = 465

func (s *SMTPSender) tlsConfig() *tls.Config {
	c := &tls.Config{MinVersion: tls.VersionTLS12}
	if s.cfg.TLSConfig != nil {
		c = s.cfg.TLSConfig.Clone()
	}
	c.ServerName = s.cfg.Host
	return c
}

// Send delivers m. It refuses a server that does not offer TLS: the
// password is an API key that must not cross the network in clear text.
func (s *SMTPSender) Send(ctx context.Context, m Message) error {
	to, err := m.recipient()
	if err != nil {
		return err
	}
	raw, err := m.Encode(s.from, s.now())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	conn, err := s.dial(ctx, addr)
	if err != nil {
		return fmt.Errorf("mail: connect to %s: %w", addr, err)
	}
	// net/smtp has no context support: the deadline bounds every read and
	// write, and closing the connection on cancel unblocks the rest.
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	if err := s.deliver(conn, to, raw); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = errors.Join(ctxErr, err)
		}
		return fmt.Errorf("mail: send to %s via %s: %w", to, addr, err)
	}
	return nil
}

func (s *SMTPSender) dial(ctx context.Context, addr string) (net.Conn, error) {
	d := &net.Dialer{}
	if s.implicitTLS {
		return (&tls.Dialer{NetDialer: d, Config: s.tlsConfig()}).DialContext(ctx, "tcp", addr)
	}
	return d.DialContext(ctx, "tcp", addr)
}

func (s *SMTPSender) deliver(conn net.Conn, to string, raw []byte) error {
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = c.Close() }() // after Quit this only closes the closed connection
	if _, isTLS := conn.(*tls.Conn); !isTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("the server does not offer STARTTLS")
		}
		if err := c.StartTLS(s.tlsConfig()); err != nil {
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}
	if s.cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	return c.Quit()
}

// LogSender writes messages to the log instead of sending them. It logs
// the text part, links included: in development and in the E2E tests that
// is how a recovery link reaches the tester.
type LogSender struct {
	logger *slog.Logger
}

// NewLogSender returns a sender that logs at INFO.
func NewLogSender(logger *slog.Logger) *LogSender {
	return &LogSender{logger: logger}
}

// Send logs m.
func (s *LogSender) Send(ctx context.Context, m Message) error {
	to, err := m.recipient()
	if err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "mail not sent: SMTP is not configured", "to", to, "subject", m.Subject, "text", m.Text)
	return nil
}
