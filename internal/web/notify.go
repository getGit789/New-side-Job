package web

import (
	"database/sql"
	"fmt"
	"net/http"

	"briefrelay/internal/db"
	"briefrelay/internal/jobs"
	"briefrelay/internal/mail"
)

// Notification recipients (contract §7).

// staffOf returns the assigned staff plus the owner of a project.
func staffOf(tx *sql.Tx, p Project) ([]string, error) {
	return ids(tx, `SELECT user_id FROM project_members WHERE project_id = ?
		UNION SELECT user_id FROM memberships WHERE workspace_id = ? AND role = 'owner'`, p.ID, p.WorkspaceID)
}

// contactsOf returns the client users of a project's organization who have accepted their invitation.
func contactsOf(tx *sql.Tx, p Project) ([]string, error) {
	return ids(tx, `SELECT user_id FROM client_contacts WHERE client_org_id = ? AND user_id IS NOT NULL AND removed_at IS NULL`, p.ClientOrgID)
}

func ids(tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// notify writes one in-app notification per user and queues one email per user.
// The dedupe key makes every (event, record, user) email happen at most once, ever.
func (s *Server) notify(tx *sql.Tx, r *http.Request, users []string, kind, recordID, title, path string) error {
	actor := s.user(r)
	for _, uid := range users {
		if actor != nil && uid == actor.ID {
			continue // nobody needs a notification about their own action
		}
		if _, err := tx.Exec(`INSERT INTO notifications (id, user_id, kind, title, url, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			db.NewID(), uid, kind, title, path, db.Now()); err != nil {
			return err
		}
		var email, name string
		if err := tx.QueryRow(`SELECT email, name FROM users WHERE id = ?`, uid).Scan(&email, &name); err != nil {
			return err
		}
		body := fmt.Sprintf("Hi %s,\n\n%s\n\nOpen it here:\n%s%s\n", name, title, s.cfg.BaseURL.String(), path)
		if err := jobs.Enqueue(r.Context(), tx, "mail.send", mail.Job{To: email, Subject: title, Body: body},
			jobs.Options{DedupeKey: fmt.Sprintf("mail:%s:%s:%s", kind, recordID, uid)}); err != nil {
			return err
		}
	}
	return nil
}

type Notification struct {
	ID, Kind, Title, URL, CreatedAt string
	Read                            bool
}

func (s *Server) unreadCount(r *http.Request) int {
	u := s.user(r)
	if u == nil {
		return 0
	}
	var n int
	s.db.R.QueryRowContext(r.Context(), `SELECT count(*) FROM notifications WHERE user_id = ? AND read_at IS NULL`, u.ID).Scan(&n)
	return n
}

func (s *Server) notificationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /notifications", s.requireUser(s.notifications))
	mux.HandleFunc("POST /notifications/read", s.requireUser(s.notificationsRead))
}

func (s *Server) notifications(w http.ResponseWriter, r *http.Request) {
	u := s.user(r)
	rows, err := s.db.R.QueryContext(r.Context(), `SELECT id, kind, title, url, created_at, read_at IS NOT NULL FROM notifications WHERE user_id = ? ORDER BY created_at DESC LIMIT 100`, u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer rows.Close()
	var list []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Kind, &n.Title, &n.URL, &n.CreatedAt, &n.Read); err == nil {
			list = append(list, n)
		}
	}
	s.render(w, r, http.StatusOK, "notifications", view{Title: "Notifications", Data: list})
}

func (s *Server) notificationsRead(w http.ResponseWriter, r *http.Request) {
	u := s.user(r)
	s.logErr("mark read", s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL`, db.Now(), u.ID)
		return err
	}))
	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}
