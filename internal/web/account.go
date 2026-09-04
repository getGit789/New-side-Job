package web

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"briefrelay/internal/auth"
	"briefrelay/internal/db"
	"briefrelay/internal/jobs"
	"briefrelay/internal/mail"
)

const resetTTL = time.Hour

func (s *Server) accountRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /account", s.requireUser(s.accountShow))
	mux.HandleFunc("POST /account/password", s.requireUser(s.passwordChange))
	mux.HandleFunc("POST /account/email", s.requireUser(s.emailChange))
	mux.HandleFunc("POST /account/notifications", s.requireUser(s.notificationPref))
	mux.HandleFunc("GET /password/forgot", s.forgotForm)
	mux.HandleFunc("POST /password/forgot", s.forgotSubmit)
	mux.HandleFunc("GET /password/reset/{token}", s.resetForm)
	mux.HandleFunc("POST /password/reset/{token}", s.resetSubmit)
}

type accountPage struct{ EmailNotifications bool }

func (s *Server) accountData(r *http.Request) accountPage {
	var d accountPage
	s.db.R.QueryRowContext(r.Context(), `SELECT email_notifications FROM users WHERE id = ?`, s.user(r).ID).Scan(&d.EmailNotifications)
	return d
}

func (s *Server) accountShow(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "account", view{Title: "Account", Data: s.accountData(r)})
}

// reauth checks the current password for security-sensitive account changes (plan §6.3).
func (s *Server) reauth(r *http.Request, password string) (bool, error) {
	var hash string
	if err := s.db.R.QueryRowContext(r.Context(), `SELECT password_hash FROM users WHERE id = ?`, s.user(r).ID).Scan(&hash); err != nil {
		return false, err
	}
	ok, _ := auth.VerifyPassword(hash, password)
	return ok, nil
}

// emailChange needs the current password. The contact record follows the user so invitations and exports stay consistent.
func (s *Server) emailChange(w http.ResponseWriter, r *http.Request) {
	if s.cfg.IsDemo() {
		s.errorPage(w, r, http.StatusForbidden, "Account details cannot be changed in the demo.")
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	u := s.user(r)
	email := strings.ToLower(f["email"])
	valid, err := s.reauth(r, f["current"])
	if err != nil {
		s.fail(w, r, err)
		return
	}
	page := view{Title: "Account", Data: s.accountData(r)}
	switch {
	case !valid:
		page.Error = "Current password is incorrect."
		s.render(w, r, http.StatusUnauthorized, "account", page)
		return
	case !strings.Contains(email, "@") || len(email) > 254 || strings.ContainsAny(email, " \t\n"):
		page.Error = "Enter a valid email address."
		s.render(w, r, http.StatusUnprocessableEntity, "account", page)
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT count(*) FROM users WHERE email = ? AND id != ?`, email, u.ID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return invalid("That email is already used by another account.")
		}
		if _, err := tx.Exec(`UPDATE users SET email = ?, updated_at = ? WHERE id = ?`, email, db.Now(), u.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE client_contacts SET email = ? WHERE user_id = ?`, email, u.ID); err != nil {
			return err
		}
		return s.audit(r.Context(), tx, r, "auth.email_changed", "user", u.ID, map[string]any{"from": u.Email, "to": email})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	page.Message = "Email changed to " + email + "."
	s.render(w, r, http.StatusOK, "account", page)
}

// notificationPref: in-app notifications are always on; email is per user, default on (contract §7).
func (s *Server) notificationPref(w http.ResponseWriter, r *http.Request) {
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	u := s.user(r)
	err := s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE users SET email_notifications = ?, updated_at = ? WHERE id = ?`, f["email_notifications"] == "1", db.Now(), u.ID)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "account", view{Title: "Account", Message: "Notification preference saved.", Data: s.accountData(r)})
}

// passwordChange requires the current password (re-authentication, contract §8) and ends every other session.
func (s *Server) passwordChange(w http.ResponseWriter, r *http.Request) {
	if s.cfg.IsDemo() {
		s.errorPage(w, r, http.StatusForbidden, "Passwords cannot be changed in the demo.")
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	u := s.user(r)
	valid, err := s.reauth(r, f["current"])
	if err != nil {
		s.fail(w, r, err)
		return
	}
	switch {
	case !valid:
		s.render(w, r, http.StatusUnauthorized, "account", view{Title: "Account", Error: "Current password is incorrect.", Data: s.accountData(r)})
		return
	case auth.CheckPassword(f["password"]) != nil:
		s.render(w, r, http.StatusUnprocessableEntity, "account", view{Title: "Account", Error: passwordRule, Data: s.accountData(r)})
		return
	}
	c, _ := r.Cookie(sessionCookie)
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		if err := setPassword(tx, u.ID, f["password"]); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ? AND token_hash != ?`, u.ID, auth.HashToken(c.Value)); err != nil {
			return err
		}
		return s.audit(r.Context(), tx, r, "user.password_changed", "user", u.ID, nil)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "account", view{Title: "Account", Message: "Password changed. Other devices were logged out.", Data: s.accountData(r)})
}

func setPassword(tx *sql.Tx, userID, password string) error {
	_, err := tx.Exec(`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, auth.HashPassword(password), db.Now(), userID)
	return err
}

func (s *Server) forgotForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "forgot", view{Title: "Reset password"})
}

// forgotSubmit answers the same way whether or not the address exists, so it cannot be used to list users.
func (s *Server) forgotSubmit(w http.ResponseWriter, r *http.Request) {
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	email := strings.ToLower(f["email"])
	done := view{Title: "Reset password", Message: "If that email belongs to an account, a reset link is on its way. It works once and expires in one hour."}
	if s.cfg.IsDemo() {
		s.render(w, r, http.StatusOK, "forgot", done)
		return
	}
	var userID, name string
	err := s.db.R.QueryRowContext(r.Context(), `SELECT id, name FROM users WHERE email = ?`, email).Scan(&userID, &name)
	if errors.Is(err, sql.ErrNoRows) {
		s.render(w, r, http.StatusOK, "forgot", done)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	token := auth.NewToken()
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO password_resets (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
			auth.HashToken(token), userID, db.Now(), db.Time(time.Now().Add(resetTTL))); err != nil {
			return err
		}
		body := fmt.Sprintf("Hello %s,\n\nSomeone asked to reset the password for this BriefRelay account. If it was you, open this link within one hour:\n%s/password/reset/%s\n\nIf it was not you, ignore this message; nothing changes.", name, s.cfg.BaseURL, token)
		if err := jobs.Enqueue(r.Context(), tx, "mail.send", mail.Job{To: email, Subject: "Reset your BriefRelay password", Body: body}, jobs.Options{}); err != nil {
			return err
		}
		return db.Audit(r.Context(), tx, "", userID, "auth.reset_requested", "user", userID, s.clientIP(r), "")
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "forgot", done)
}

func (s *Server) loadReset(r *http.Request, token string) (userID string, err error) {
	var expires string
	err = s.db.R.QueryRowContext(r.Context(), `SELECT user_id, expires_at FROM password_resets WHERE token_hash = ? AND used_at IS NULL`, auth.HashToken(token)).Scan(&userID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errNotFound
	}
	if exp, perr := db.ParseTime(expires); err == nil && (perr != nil || time.Now().After(exp)) {
		return "", errNotFound
	}
	return
}

func (s *Server) resetForm(w http.ResponseWriter, r *http.Request) {
	if _, err := s.loadReset(r, r.PathValue("token")); err != nil {
		s.errorPage(w, r, http.StatusGone, "This reset link is not valid any more. Request a new one.")
		return
	}
	s.render(w, r, http.StatusOK, "reset", view{Title: "Choose a new password"})
}

func (s *Server) resetSubmit(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	userID, err := s.loadReset(r, token)
	if err != nil {
		s.errorPage(w, r, http.StatusGone, "This reset link is not valid any more. Request a new one.")
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	if auth.CheckPassword(f["password"]) != nil {
		s.render(w, r, http.StatusUnprocessableEntity, "reset", view{Title: "Choose a new password", Error: passwordRule})
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		res, err := tx.Exec(`UPDATE password_resets SET used_at = ? WHERE token_hash = ? AND used_at IS NULL`, db.Now(), auth.HashToken(token))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errNotFound
		}
		if err := setPassword(tx, userID, f["password"]); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
			return err
		}
		return db.Audit(r.Context(), tx, "", userID, "auth.reset_completed", "user", userID, s.clientIP(r), "")
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
