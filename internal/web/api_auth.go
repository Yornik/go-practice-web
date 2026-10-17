package web

import (
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/yornik/go-practice-web/internal/auth"
	"github.com/yornik/go-practice-web/internal/store"
)

// maxCredentialJSON bounds a finish request. A passkey response is a few
// KiB; attestation certificates can add a few more.
const maxCredentialJSON = 64 << 10

// userJSON is the public part of a user that the API returns.
type userJSON struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	IsAdmin     bool   `json:"isAdmin"`
}

func toUserJSON(u store.User) userJSON {
	return userJSON{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, IsAdmin: u.IsAdmin}
}

// signedIn starts a session and answers with the user and where to go.
func (s *Server) signedIn(w http.ResponseWriter, r *http.Request, u store.User, next string) {
	if err := s.cfg.Auth.StartSession(r.Context(), w, u.ID, s.client(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(u), "next": safeNext(next)})
}

func (s *Server) registerBegin(w http.ResponseWriter, r *http.Request) {
	var in auth.SignUp
	if err := decode(w, r, maxJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	b, err := s.cfg.Auth.BeginRegistration(r.Context(), in, s.client(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) registerFinish(w http.ResponseWriter, r *http.Request) {
	var in auth.Finish
	if err := decode(w, r, maxCredentialJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	u, err := s.cfg.Auth.FinishRegistration(r.Context(), in, s.client(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.signedIn(w, r, u, "/learn")
}

func (s *Server) loginBegin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Conditional bool `json:"conditional"`
	}
	if err := decode(w, r, maxJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	b, err := s.cfg.Auth.BeginLogin(r.Context(), in.Conditional, s.client(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) loginFinish(w http.ResponseWriter, r *http.Request) {
	var in struct {
		auth.Finish
		Next string `json:"next"`
	}
	if err := decode(w, r, maxCredentialJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	u, err := s.cfg.Auth.FinishLogin(r.Context(), in.Finish, s.client(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.signedIn(w, r, u, in.Next)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Everywhere bool `json:"everywhere"`
	}
	if err := decode(w, r, maxJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	id, _ := identity(r)
	if err := s.cfg.Auth.Logout(r.Context(), w, id, in.Everywhere, s.client(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"next": "/"})
}

func (s *Server) passkeyBegin(w http.ResponseWriter, r *http.Request) {
	id, _ := identity(r)
	b, err := s.cfg.Auth.BeginAddPasskey(r.Context(), id.User, s.client(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) passkeyFinish(w http.ResponseWriter, r *http.Request) {
	var in auth.AddPasskey
	if err := decode(w, r, maxCredentialJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	id, _ := identity(r)
	c, err := s.cfg.Auth.FinishAddPasskey(r.Context(), id.User, in, s.client(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": base64.RawURLEncoding.EncodeToString(c.ID), "name": c.Name})
}

// passkeyID reads the base64url credential ID from the path.
func passkeyID(r *http.Request) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(r.PathValue("id"))
	if err != nil || len(b) == 0 || len(b) > 1023 {
		return nil, errorf(http.StatusNotFound, "not_found", "This passkey does not exist.")
	}
	return b, nil
}

func (s *Server) passkeyRename(w http.ResponseWriter, r *http.Request) {
	cid, err := passkeyID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(w, r, maxJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	id, _ := identity(r)
	if err := s.cfg.Auth.RenamePasskey(r.Context(), id.User, cid, in.Name, s.client(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) passkeyDelete(w http.ResponseWriter, r *http.Request) {
	cid, err := passkeyID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, _ := identity(r)
	if err := s.cfg.Auth.DeletePasskey(r.Context(), id.User, cid, s.client(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// recoverMessage is the same for every address, known or not.
const recoverMessage = "If an account has this verified address, a recovery link is on its way. It works for 15 minutes."

func (s *Server) recoverRequest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if err := decode(w, r, maxJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.cfg.Auth.RequestRecovery(r.Context(), in.Email, s.client(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": recoverMessage})
}

func (s *Server) recoverBegin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if err := decode(w, r, maxJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	b, err := s.cfg.Auth.BeginRecovery(r.Context(), in.Token, s.client(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) recoverFinish(w http.ResponseWriter, r *http.Request) {
	var in auth.Recover
	if err := decode(w, r, maxCredentialJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	u, err := s.cfg.Auth.FinishRecovery(r.Context(), in, s.client(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.signedIn(w, r, u, "/account")
}

func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if err := decode(w, r, maxJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	u, err := s.cfg.Auth.VerifyEmail(r.Context(), in.Token, s.client(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"email": u.Email})
}

func (s *Server) resendVerification(w http.ResponseWriter, r *http.Request) {
	id, _ := identity(r)
	addr, err := s.cfg.Auth.ResendVerification(r.Context(), id.User, s.client(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"email": addr})
}

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	var in auth.Account
	if err := decode(w, r, maxJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	id, _ := identity(r)
	res, err := s.cfg.Auth.UpdateAccount(r.Context(), id.User, in, s.client(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": toUserJSON(res.User), "email": res.User.Email,
		"verificationSent": res.VerificationSent, "pendingEmail": res.Email,
	})
}

// Admin.

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	var in auth.InviteRequest
	if err := decode(w, r, maxJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	id, _ := identity(r)
	res, err := s.cfg.Auth.CreateInvite(r.Context(), id.User, in, s.client(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// The admin page adds the new row to its table from these fields; the
	// table names creators by username, as pages.go does.
	writeJSON(w, http.StatusCreated, struct {
		auth.InviteResult
		Hash      string    `json:"hash"`
		CreatedAt time.Time `json:"createdAt"`
		ExpiresAt time.Time `json:"expiresAt"`
		Email     string    `json:"email"`
		Note      string    `json:"note"`
		CreatedBy string    `json:"createdBy"`
	}{res, hex.EncodeToString(res.Invite.TokenHash), res.Invite.CreatedAt, res.Invite.ExpiresAt, res.Invite.Email, res.Invite.Note, id.User.Username})
}

func (s *Server) revokeInvite(w http.ResponseWriter, r *http.Request) {
	id, _ := identity(r)
	if err := s.cfg.Auth.RevokeInvite(r.Context(), id.User, r.PathValue("hash"), s.client(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	uid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || uid <= 0 {
		s.fail(w, r, errorf(http.StatusNotFound, "not_found", "This user does not exist."))
		return
	}
	var in auth.UserUpdate
	if err := decode(w, r, maxJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	id, _ := identity(r)
	if err := s.cfg.Auth.UpdateUser(r.Context(), id.User, uid, in, s.client(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
