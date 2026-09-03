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
	mux.HandleFunc("GET /password/forgot", s.forgotForm)
	mux.HandleFunc("POST /password/forgot", s.forgotSubmit)
	mux.HandleFunc("GET /password/reset/{token}", s.resetForm)
	mux.HandleFunc("POST /password/reset/{token}", s.resetSubmit)
}

func (s *Server) accountShow(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "account", view{Title: "Account"})
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
	var hash string
	if err := s.db.R.QueryRowContext(r.Context(), `SELECT password_hash FROM users WHERE id = ?`, u.ID).Scan(&hash); err != nil {
		s.fail(w, r, err)
		return
	}
	valid, _ := auth.VerifyPassword(hash, f["current"])
	switch {
	case !valid:
		s.render(w, r, http.StatusUnauthorized, "account", view{Title: "Account", Error: "Current password is incorrect."})
		return
	case len(f["password"]) < 12:
		s.render(w, r, http.StatusUnprocessableEntity, "account", view{Title: "Account", Error: "New password must be at least 12 characters."})
		return
	}
	c, _ := r.Cookie(sessionCookie)
	err := s.db.Tx(r.Context(), func(tx *sql.Tx) error {
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
	s.render(w, r, http.StatusOK, "account", view{Title: "Account", Message: "Password changed. Other devices were logged out."})
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
	if len(f["password"]) < 12 {
		s.render(w, r, http.StatusUnprocessableEntity, "reset", view{Title: "Choose a new password", Error: "Password must be at least 12 characters."})
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
