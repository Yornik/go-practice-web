package auth

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/yornik/go-practice-web/internal/mail"
	"github.com/yornik/go-practice-web/internal/store"
)

var errNotAdmin = newError(http.StatusForbidden, "forbidden", "Only an admin can do this.", nil)

// InviteRequest is the admin's invite form.
type InviteRequest struct {
	Email string `json:"email"` // optional: the invite is mailed there
	Note  string `json:"note"`  // optional: shown in the email and the invite list
	Days  int    `json:"days"`  // 0 means 7
}

// InviteResult is a created invite. The link holds the only copy of the
// token; the store keeps its hash.
type InviteResult struct {
	// Link is the invite link for the admin to pass on. It is "" when an
	// SMTP server took the mail: then the mail holds the only copy, so
	// opening the link proves the address (flow 2 in docs/DESIGN.md).
	Link   string       `json:"url,omitempty"`
	Invite store.Invite `json:"-"`
	// Mailed is true when the invite went out by email. MailError holds
	// the reason when sending failed; the link still works.
	Mailed    bool   `json:"mailed"`
	MailError string `json:"mailError,omitempty"`
}

// CreateInvite makes a single-use sign-up link. With an email address it
// is sent at once, so the admin learns about a failure.
func (s *Service) CreateInvite(ctx context.Context, admin store.User, in InviteRequest, c Client) (InviteResult, error) {
	if !admin.IsAdmin {
		return InviteResult{}, errNotAdmin
	}
	email, err := cleanEmail(in.Email)
	if err != nil {
		return InviteResult{}, err
	}
	days := in.Days
	if days == 0 {
		days = int(DefaultInvite / (24 * time.Hour))
	}
	if days < 1 || days > MaxInviteDays {
		return InviteResult{}, badInput("bad_days", fmt.Sprintf("An invite lasts 1 to %d days.", MaxInviteDays))
	}
	note := ""
	if in.Note != "" {
		if note, err = cleanLabel(in.Note, maxNoteRunes, "bad_note", "Keep the note under 500 characters."); err != nil {
			return InviteResult{}, err
		}
	}
	token, hash := newToken()
	inv, err := s.store.CreateInvite(ctx, store.Invite{
		TokenHash: hash, CreatedBy: admin.ID, Email: email, Note: note,
		ExpiresAt: s.now().Add(time.Duration(days) * 24 * time.Hour),
	})
	if err != nil {
		return InviteResult{}, storeError(err, "create invite")
	}
	link := s.link("/join", url.Values{"invite": {token}})
	res := InviteResult{Link: link, Invite: inv}
	s.audit(ctx, admin.ID, "invite.created", map[string]any{"email": email, "days": days}, c)
	if email != "" {
		mctx, cancel := context.WithTimeout(ctx, mailTimeout)
		defer cancel()
		err := s.mailer.Send(mctx, email, mail.Invite{
			InvitedBy: admin.DisplayName, Note: note, Link: link, Expires: inv.ExpiresAt, Exercises: s.exercises,
		})
		switch {
		case err != nil:
			s.log.Error("send invite", "err", err)
			res.MailError = "The email could not be sent. Copy the link and send it yourself."
		case s.mailer.Delivers():
			res.Mailed = true
			// Only a mail that left the server proves the address, so only
			// it is recorded: anyone with the log can read the log sender's
			// copy.
			if err := s.store.MarkInviteMailed(ctx, hash); err != nil {
				// The invite works; a sign-up with it gets a verification
				// link, as with an invite without an address, so the admin
				// may have the link to pass on.
				s.log.Error("mark invite mailed", "err", err)
				break
			}
			// The mail must hold the only copy of the link: a copy the
			// admin passes on would prove nothing about the address, for
			// example when it has a typo and the mail bounced later.
			res.Link = ""
		default:
			res.Mailed = true
		}
	}
	return res, nil
}

// RevokeInvite deletes an unused invite by the hex hash the admin page
// lists.
func (s *Service) RevokeInvite(ctx context.Context, admin store.User, hashHex string, c Client) error {
	if !admin.IsAdmin {
		return errNotAdmin
	}
	hash, err := hex.DecodeString(hashHex)
	if err != nil || len(hash) != 32 {
		return newError(http.StatusNotFound, "not_found", "This invite does not exist or was used.", err)
	}
	if err := s.store.DeleteInvite(ctx, hash); err != nil {
		e := storeError(err, "revoke invite")
		if ae, ok := AsError(e); ok && ae.Status == http.StatusNotFound {
			ae.Message = "This invite does not exist or was used."
		}
		return e
	}
	s.audit(ctx, admin.ID, "invite.revoked", map[string]string{"hash": hashHex}, c)
	return nil
}

// UserUpdate is the admin's change to an account. Nil fields stay.
type UserUpdate struct {
	Disabled *bool `json:"disabled"`
	Admin    *bool `json:"admin"`
}

// UpdateUser disables, enables, promotes or demotes an account. Admins
// cannot lock themselves out: they cannot disable themselves or drop their
// own admin flag.
func (s *Service) UpdateUser(ctx context.Context, admin store.User, id int64, in UserUpdate, c Client) error {
	if !admin.IsAdmin {
		return errNotAdmin
	}
	if id == admin.ID && (in.Disabled != nil && *in.Disabled || in.Admin != nil && !*in.Admin) {
		return badInput("self", "You cannot disable your own account or remove your own admin rights.")
	}
	if in.Disabled != nil {
		if err := s.store.SetDisabled(ctx, id, *in.Disabled); err != nil {
			return storeError(err, "disable user")
		}
		s.audit(ctx, admin.ID, "user.disabled", map[string]any{"user": id, "disabled": *in.Disabled}, c)
	}
	if in.Admin != nil {
		if err := s.store.SetAdmin(ctx, id, *in.Admin); err != nil {
			return storeError(err, "set admin")
		}
		s.audit(ctx, admin.ID, "user.admin", map[string]any{"user": id, "admin": *in.Admin}, c)
	}
	return nil
}
