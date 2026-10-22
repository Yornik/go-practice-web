package main

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/yornik/go-practice-web/internal/auth"
	"github.com/yornik/go-practice-web/internal/mail"
)

// envPrefix is the prefix of every web server variable.
const envPrefix = "GPW_"

// minTokenLen matches the runner's rule for GPR_TOKEN: the 2 must be equal.
const minTokenLen = 32

// config is the web server configuration from GPW_* variables; see the Web
// table in docs/DESIGN.md.
type config struct {
	Addr           string
	MetricsAddr    string
	PublicURL      string // scheme and host, no trailing slash
	DBPath         string
	CourseDir      string
	RunnerURL      string
	RunnerToken    string
	SMTP           mail.Config
	TrustedProxies []netip.Prefix
	Dev            bool
	// MovedFrom is the host the site moved away from, such as
	// "go-learn.party"; "" when it never moved. A passkey belongs to the
	// host that made it, so the sign-in page tells its users to recover.
	MovedFrom string
}

// knownVars are the variables loadConfig reads. Any other GPW_ variable is
// an error: a typo would otherwise silently fall back to a default.
var knownVars = []string{
	"GPW_ADDR", "GPW_METRICS_ADDR", "GPW_PUBLIC_URL", "GPW_DB_PATH", "GPW_COURSE_DIR",
	"GPW_RUNNER_URL", "GPW_RUNNER_TOKEN", "GPW_SMTP_HOST", "GPW_SMTP_PORT", "GPW_SMTP_USER",
	"GPW_SMTP_PASSWORD", "GPW_MAIL_FROM", "GPW_TRUSTED_PROXIES", "GPW_DEV", "GPW_MOVED_FROM",
}

// loadConfig reads the configuration from environ (os.Environ format). It
// reports every problem at once, so that 1 failed start shows them all.
func loadConfig(environ []string) (config, error) {
	env := make(map[string]string)
	var errs []error
	for _, kv := range environ {
		key, value, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(key, envPrefix) {
			continue
		}
		if !slices.Contains(knownVars, key) {
			errs = append(errs, fmt.Errorf("%s: unknown variable", key))
			continue
		}
		env[key] = value
	}
	get := func(key, def string) string {
		if v, ok := env[key]; ok {
			return v
		}
		return def
	}

	c := config{
		Addr:        get("GPW_ADDR", ":8080"),
		MetricsAddr: get("GPW_METRICS_ADDR", ":9090"),
		DBPath:      get("GPW_DB_PATH", "/data/go-practice.db"),
		CourseDir:   get("GPW_COURSE_DIR", "/course"),
		RunnerToken: get("GPW_RUNNER_TOKEN", ""),
		SMTP: mail.Config{
			Host:     get("GPW_SMTP_HOST", ""),
			Username: get("GPW_SMTP_USER", ""),
			Password: get("GPW_SMTP_PASSWORD", ""),
			From:     get("GPW_MAIL_FROM", mail.DefaultFrom),
		},
	}
	var err error
	c.Dev, err = strconv.ParseBool(get("GPW_DEV", "false"))
	if err != nil {
		errs = append(errs, fmt.Errorf("GPW_DEV: %q is not true or false", get("GPW_DEV", "")))
	}
	c.PublicURL, err = parsePublicURL(get("GPW_PUBLIC_URL", ""), c.Dev)
	errs = append(errs, err)
	c.RunnerURL, err = parseRunnerURL(get("GPW_RUNNER_URL", ""))
	errs = append(errs, err)
	c.MovedFrom, err = parseMovedFrom(get("GPW_MOVED_FROM", ""), c.PublicURL)
	errs = append(errs, err)
	errs = append(errs,
		checkAddr("GPW_ADDR", c.Addr),
		checkAddr("GPW_METRICS_ADDR", c.MetricsAddr),
		checkAbs("GPW_DB_PATH", c.DBPath),
		checkAbs("GPW_COURSE_DIR", c.CourseDir),
	)
	if c.Addr == c.MetricsAddr {
		errs = append(errs, errors.New("GPW_METRICS_ADDR: must differ from GPW_ADDR"))
	}
	if len(c.RunnerToken) < minTokenLen {
		errs = append(errs, fmt.Errorf("GPW_RUNNER_TOKEN: required, at least %d characters", minTokenLen))
	}
	port := get("GPW_SMTP_PORT", "465")
	if c.SMTP.Port, err = strconv.Atoi(port); err != nil || c.SMTP.Port < 1 || c.SMTP.Port > 65535 {
		errs = append(errs, fmt.Errorf("GPW_SMTP_PORT: %q is not a port", port))
	}
	if _, err := mail.ParseFrom(c.SMTP.From); err != nil {
		errs = append(errs, fmt.Errorf("GPW_MAIL_FROM: %w", err))
	}
	if c.TrustedProxies, err = auth.ParseTrustedProxies(get("GPW_TRUSTED_PROXIES", "10.244.0.0/16")); err != nil {
		errs = append(errs, fmt.Errorf("GPW_TRUSTED_PROXIES: %w", err))
	}

	if err := errors.Join(errs...); err != nil {
		return config{}, fmt.Errorf("invalid configuration:\n%w", err)
	}
	return c, nil
}

// parsePublicURL accepts an origin: https, or http in dev mode, with no
// path, query or user info. Passkeys are bound to it.
func parsePublicURL(s string, dev bool) (string, error) {
	if s == "" {
		return "", errors.New("GPW_PUBLIC_URL: required, for example https://go-learn.party")
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") {
		return "", fmt.Errorf("GPW_PUBLIC_URL: %q is not an origin such as https://go-learn.party", s)
	}
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && dev:
	default:
		return "", fmt.Errorf("GPW_PUBLIC_URL: %q must use https (http only with GPW_DEV=true)", s)
	}
	return u.Scheme + "://" + u.Host, nil
}

// parseMovedFrom accepts the bare host name of the old site: the page
// prints it in sentences, so a scheme, a port or a path would read wrong.
// It must differ from the host of GPW_PUBLIC_URL.
func parseMovedFrom(s, publicURL string) (string, error) {
	if s == "" {
		return "", nil
	}
	// A passkey's relying party is a domain, never an address. A browser
	// reads a host whose last label is a number as an IPv4 address in any
	// of its forms (127.1, 0x7f.0.0.1), so such a value cannot be the host
	// a passkey came from.
	if len(s) > 253 || !hostName.MatchString(s) || endsInNumber(s) {
		return "", fmt.Errorf("GPW_MOVED_FROM: %q is not a host name such as go-learn.party", s)
	}
	if u, err := url.Parse(publicURL); err == nil && strings.EqualFold(u.Hostname(), s) {
		return "", fmt.Errorf("GPW_MOVED_FROM: %q is the host of GPW_PUBLIC_URL", s)
	}
	return strings.ToLower(s), nil
}

// endsInNumber is the WHATWG URL standard's "ends in a number" check: the
// last label is all digits, or 0x and hex digits.
func endsInNumber(host string) bool {
	last := host[strings.LastIndexByte(host, '.')+1:]
	if l := strings.ToLower(last); strings.HasPrefix(l, "0x") {
		return strings.Trim(l[2:], "0123456789abcdef") == ""
	}
	return last != "" && strings.Trim(last, "0123456789") == ""
}

// hostName is a DNS name of at least 2 labels: letters, digits and
// hyphens, no hyphen at either end of a label.
var hostName = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))+$`)

func parseRunnerURL(s string) (string, error) {
	if s == "" {
		return "", errors.New("GPW_RUNNER_URL: required, for example http://runner.go-practice-runner.svc:8081")
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("GPW_RUNNER_URL: %q is not an http(s) URL", s)
	}
	return strings.TrimSuffix(s, "/"), nil
}

func checkAddr(key, addr string) error {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("%s: %q is not a host:port address: %w", key, addr, err)
	}
	return nil
}

// checkAbs requires an absolute path, so the meaning does not depend on
// the working directory of the pod.
func checkAbs(key, path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s: %q is not an absolute path", key, path)
	}
	return nil
}
