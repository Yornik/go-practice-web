package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// tokenBytes is the entropy of every secret this package hands out:
// session cookies, invite, setup and email links.
const tokenBytes = 32

// tokenLen is the length of a token as text: unpadded base64url of 32
// bytes.
var tokenLen = base64.RawURLEncoding.EncodedLen(tokenBytes)

// newToken returns a random token and the hash that the store keeps. Only
// the hash is stored, so a copy of the database grants nothing.
func newToken() (token string, hash []byte) {
	b := make([]byte, tokenBytes)
	// crypto/rand.Read never fails on supported platforms.
	_, _ = rand.Read(b)
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashString(token)
}

func hashString(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// hashToken checks the shape of a token from a link or cookie and returns
// its hash. A malformed token is rejected before it reaches the database.
func hashToken(token string) ([]byte, bool) {
	if len(token) != tokenLen {
		return nil, false
	}
	if _, err := base64.RawURLEncoding.DecodeString(token); err != nil {
		return nil, false
	}
	return hashString(token), true
}

// errBadToken is the answer to a malformed link: the same as for a used
// one, so a guess learns nothing.
var errBadToken = newError(http.StatusGone, "token_invalid", "This link was already used or has expired.", nil)

var usernameRE = regexp.MustCompile(`^[a-z0-9_-]{3,32}$`)

// cleanUsername lowercases and checks a username against the rule the
// schema enforces, so the user gets a message instead of a constraint
// error.
func cleanUsername(s string) (string, error) {
	u := strings.ToLower(strings.TrimSpace(s))
	if !usernameRE.MatchString(u) {
		return "", badInput("bad_username", "Use 3 to 32 characters: lowercase letters, digits, - and _.")
	}
	return u, nil
}

const maxDisplayNameRunes = 64

// cleanDisplayName trims and collapses spaces and rejects control and
// formatting characters, which could make a name look like another.
func cleanDisplayName(s string) (string, error) {
	return cleanLabel(s, maxDisplayNameRunes, "bad_display_name", "Use 1 to 64 printable characters for your name.")
}

func cleanLabel(s string, maxRunes int, code, msg string) (string, error) {
	if !utf8.ValidString(s) {
		return "", badInput(code, msg)
	}
	name := strings.Join(strings.Fields(s), " ")
	n := utf8.RuneCountInString(name)
	if n == 0 || n > maxRunes {
		return "", badInput(code, msg)
	}
	for _, r := range name {
		if !unicode.IsPrint(r) && r != ' ' {
			return "", badInput(code, msg)
		}
	}
	return name, nil
}

const maxEmailLen = 254

// errEmailRequired is the answer to an account without an address: the
// address is the only way back in after a lost passkey.
var errEmailRequired = badInput("email_required", "Enter your email address. It is your way back in if you lose your passkey.")

// requiredEmail checks the address of an account: cleanEmail, and it must
// not be empty. Sign-up and the account page use it; an invite's address
// stays optional.
func requiredEmail(s string) (string, error) {
	e, err := cleanEmail(s)
	if err != nil {
		return "", err
	}
	if e == "" {
		return "", errEmailRequired
	}
	return e, nil
}

// cleanEmail checks an optional email address. It accepts a bare address
// only, so that "Name <a@b>" cannot sneak a display name into a header.
func cleanEmail(s string) (string, error) {
	e := strings.TrimSpace(s)
	if e == "" {
		return "", nil
	}
	a, err := mail.ParseAddress(e)
	if err != nil || a.Address != e || len(e) > maxEmailLen || !strings.Contains(e[strings.LastIndex(e, "@")+1:], ".") {
		return "", badInput("bad_email", "Enter an email address like name@example.com.")
	}
	return e, nil
}
