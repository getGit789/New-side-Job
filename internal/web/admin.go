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

const invitationTTL = 7 * 24 * time.Hour

type TeamMember struct {
	ID, Name, Email, Role, CreatedAt string
}

type Invitation struct {
	ID, Email, Role, CreatedAt, ExpiresAt string
}

func (s *Server) adminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /team", s.requireOwner(s.teamShow))
	mux.HandleFunc("POST /team/invite", s.requireOwner(s.teamInvite))
	mux.HandleFunc("POST /invitations/{id}/revoke", s.requireOwner(s.invitationRevoke))
	mux.HandleFunc("POST /team/{uid}/remove", s.requireOwner(s.teamRemove))
	mux.HandleFunc("GET /invite/{token}", s.inviteForm)
	mux.HandleFunc("POST /invite/{token}", s.inviteAccept)
	mux.HandleFunc("GET /activity", s.requireOwner(s.activity))
}

type teamPage struct {
	Members     []TeamMember
	Invitations []Invitation
}

func (s *Server) teamShow(w http.ResponseWriter, r *http.Request) {
	ws := s.user(r).WorkspaceID
	var d teamPage
	rows, err := s.db.R.QueryContext(r.Context(), `SELECT u.id, u.name, u.email, m.role, m.created_at FROM memberships m JOIN users u ON u.id = m.user_id WHERE m.workspace_id = ? AND m.role IN ('owner','staff') ORDER BY m.role, u.name`, ws)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for rows.Next() {
		var m TeamMember
		if err := rows.Scan(&m.ID, &m.Name, &m.Email, &m.Role, &m.CreatedAt); err == nil {
			d.Members = append(d.Members, m)
		}
	}
	rows.Close()
	rows, err = s.db.R.QueryContext(r.Context(), `SELECT id, email, role, created_at, expires_at FROM invitations WHERE workspace_id = ? AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ? ORDER BY created_at DESC`, ws, db.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for rows.Next() {
		var i Invitation
		if err := rows.Scan(&i.ID, &i.Email, &i.Role, &i.CreatedAt, &i.ExpiresAt); err == nil {
			d.Invitations = append(d.Invitations, i)
		}
	}
	rows.Close()
	s.render(w, r, http.StatusOK, "team", view{Title: "Team", Data: d})
}

// createInvitation writes the invitation and queues the email inside tx. Used for staff now and client contacts in Phase 4.
func (s *Server) createInvitation(r *http.Request, tx *sql.Tx, email, role, contactID, inviterName, workspaceName string) (id string, err error) {
	u := s.user(r)
	id, token, now := db.NewID(), auth.NewToken(), time.Now()
	var contact any
	if contactID != "" {
		contact = contactID
	}
	if _, err := tx.Exec(`INSERT INTO invitations (id, workspace_id, email, role, contact_id, token_hash, created_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, u.WorkspaceID, email, role, contact, auth.HashToken(token), u.ID, db.Time(now), db.Time(now.Add(invitationTTL))); err != nil {
		return "", err
	}
	link := s.cfg.BaseURL.String() + "/invite/" + token
	body := fmt.Sprintf("%s invited you to the %s workspace on BriefRelay.\n\nAccept the invitation here (valid for 7 days, single use):\n%s\n\nIf you did not expect this, ignore this message.", inviterName, workspaceName, link)
	if err := jobs.Enqueue(r.Context(), tx, "mail.send", mail.Job{To: email, Subject: "You are invited to " + workspaceName, Body: body}, jobs.Options{DedupeKey: "mail:invitation:" + id}); err != nil {
		return "", err
	}
	return id, s.audit(r.Context(), tx, r, "invitation.sent", "invitation", id, map[string]any{"role": role})
}

func (s *Server) teamInvite(w http.ResponseWriter, r *http.Request) {
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	email := strings.ToLower(f["email"])
	if !strings.Contains(email, "@") || len(email) > 254 {
		s.fail(w, r, invalid("Enter a valid email address."))
		return
	}
	u := s.user(r)
	err := s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT count(*) FROM users WHERE email = ?`, email).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return invalid("A user with that email already exists.")
		}
		var ws string
		if err := tx.QueryRow(`SELECT name FROM workspaces WHERE id = ?`, u.WorkspaceID).Scan(&ws); err != nil {
			return err
		}
		_, err := s.createInvitation(r, tx, email, "staff", "", u.Name, ws)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/team", http.StatusSeeOther)
}

func (s *Server) invitationRevoke(w http.ResponseWriter, r *http.Request) {
	err := s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		res, err := tx.Exec(`UPDATE invitations SET revoked_at = ? WHERE id = ? AND workspace_id = ? AND accepted_at IS NULL AND revoked_at IS NULL`, db.Now(), r.PathValue("id"), s.user(r).WorkspaceID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errNotFound
		}
		return s.audit(r.Context(), tx, r, "invitation.revoked", "invitation", r.PathValue("id"), nil)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/team", http.StatusSeeOther)
}

func (s *Server) teamRemove(w http.ResponseWriter, r *http.Request) {
	u := s.user(r)
	uid := r.PathValue("uid")
	if uid == u.ID {
		s.fail(w, r, invalid("The owner cannot remove themselves."))
		return
	}
	err := s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		res, err := tx.Exec(`DELETE FROM memberships WHERE workspace_id = ? AND user_id = ? AND role = 'staff'`, u.WorkspaceID, uid)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errNotFound
		}
		if _, err := tx.Exec(`DELETE FROM project_members WHERE user_id = ?`, uid); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, uid); err != nil {
			return err
		}
		return s.audit(r.Context(), tx, r, "member.removed", "user", uid, nil)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/team", http.StatusSeeOther)
}

type invitePage struct {
	Token, Email, Workspace string
}

func (s *Server) loadInvitation(r *http.Request, token string) (id, ws, email, role string, err error) {
	var expires string
	err = s.db.R.QueryRowContext(r.Context(), `SELECT i.id, w.name, i.email, i.role, i.expires_at FROM invitations i JOIN workspaces w ON w.id = i.workspace_id
		WHERE i.token_hash = ? AND i.accepted_at IS NULL AND i.revoked_at IS NULL`, auth.HashToken(token)).Scan(&id, &ws, &email, &role, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", "", errNotFound
	}
	if err != nil {
		return
	}
	if exp, perr := db.ParseTime(expires); perr != nil || time.Now().After(exp) {
		return "", "", "", "", errNotFound
	}
	return
}

func (s *Server) inviteForm(w http.ResponseWriter, r *http.Request) {
	_, ws, email, _, err := s.loadInvitation(r, r.PathValue("token"))
	if err != nil {
		s.errorPage(w, r, http.StatusGone, "This invitation is not valid any more. Ask for a new one.")
		return
	}
	s.render(w, r, http.StatusOK, "invite", view{Title: "Accept invitation", Data: invitePage{Token: r.PathValue("token"), Email: email, Workspace: ws}})
}

func (s *Server) inviteAccept(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	invID, ws, email, role, err := s.loadInvitation(r, token)
	if err != nil {
		s.errorPage(w, r, http.StatusGone, "This invitation is not valid any more. Ask for a new one.")
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	v := view{Title: "Accept invitation", Form: f, Data: invitePage{Token: token, Email: email, Workspace: ws}}
	switch {
	case !within(f["name"], 1, 200):
		v.Error = "Enter your name."
	case len(f["password"]) < 12 || len(f["password"]) > 200:
		v.Error = "Password must be at least 12 characters."
	}
	if v.Error != "" {
		s.render(w, r, http.StatusUnprocessableEntity, "invite", v)
		return
	}
	var session string
	userID, now, ip := db.NewID(), db.Now(), s.clientIP(r)
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		// Single use: the UPDATE only succeeds once.
		res, err := tx.Exec(`UPDATE invitations SET accepted_at = ? WHERE id = ? AND accepted_at IS NULL AND revoked_at IS NULL`, now, invID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errNotFound
		}
		var wsID string
		var contactID sql.NullString
		if err := tx.QueryRow(`SELECT workspace_id, contact_id FROM invitations WHERE id = ?`, invID).Scan(&wsID, &contactID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO users (id, email, name, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, userID, email, f["name"], auth.HashPassword(f["password"]), now, now); err != nil {
			return invalid("An account with this email already exists. Log in instead.")
		}
		if _, err := tx.Exec(`INSERT INTO memberships (workspace_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`, wsID, userID, role, now); err != nil {
			return err
		}
		if role == "client" {
			res, err := tx.Exec(`UPDATE client_contacts SET user_id = ? WHERE id = ? AND user_id IS NULL AND removed_at IS NULL`, userID, contactID.String)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return errNotFound // contact was removed after the invitation went out
			}
		}
		if err := db.Audit(r.Context(), tx, wsID, userID, "invitation.accepted", "invitation", invID, ip, fmt.Sprintf(`{"role":%q}`, role)); err != nil {
			return err
		}
		session, err = auth.CreateSession(r.Context(), tx, userID, ip, r.UserAgent())
		return err
	})
	if errors.Is(err, errNotFound) {
		s.errorPage(w, r, http.StatusGone, "This invitation was already used.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.setSessionCookie(w, session, int(auth.SessionTTL.Seconds()))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

type activityPage struct {
	Events  []AuditRow
	Page    int
	HasNext bool
}

func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	d := activityPage{}
	var limit, offset int
	d.Page, limit, offset = pageArgs(r)
	rows, err := s.db.R.QueryContext(r.Context(), `SELECT a.action, a.target_type, a.target_id, COALESCE(u.name, a.actor_id, ''), a.meta, a.created_at FROM audit_events a LEFT JOIN users u ON u.id = a.actor_id
		WHERE +a.workspace_id = ? OR a.workspace_id IS NULL ORDER BY a.id DESC LIMIT ? OFFSET ?`, s.user(r).WorkspaceID, limit+1, offset)
	// The unary + stops SQLite from using the workspace index for the OR, which made it collect and
	// sort every event; a newest-first rowid walk stops after one page (430ms → 1ms at 100k events).
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var a AuditRow
		if err := rows.Scan(&a.Action, &a.TargetType, &a.TargetID, &a.Actor, &a.Meta, &a.CreatedAt); err != nil {
			s.fail(w, r, err)
			return
		}
		d.Events = append(d.Events, a)
	}
	if len(d.Events) > limit {
		d.Events, d.HasNext = d.Events[:limit], true
	}
	s.render(w, r, http.StatusOK, "activity", view{Title: "Activity", Data: d})
}
