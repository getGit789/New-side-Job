package web

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"briefrelay/internal/db"
	"briefrelay/internal/domain"
)

type Client struct {
	ID, Name, Email, Phone, Notes, CreatedAt string
	ArchivedAt                               sql.NullString
	ProjectCount                             int
}

type Contact struct {
	ID, Name, Email, Title, CreatedAt string
	UserID                            sql.NullString
}

func (s *Server) clientRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /clients", s.requireStaff(s.clientsList))
	mux.HandleFunc("POST /clients", s.requireOwner(s.clientCreate))
	mux.HandleFunc("GET /clients/{id}", s.requireStaff(s.clientShow))
	mux.HandleFunc("POST /clients/{id}", s.requireOwner(s.clientEdit))
	mux.HandleFunc("POST /clients/{id}/contacts", s.requireStaff(s.contactCreate))
	mux.HandleFunc("POST /clients/{id}/contacts/{cid}/remove", s.requireStaff(s.contactRemove))
	mux.HandleFunc("POST /clients/{id}/contacts/{cid}/invite", s.requireStaff(s.contactInvite))
	mux.HandleFunc("POST /clients/{id}/archive", s.requireOwner(s.clientArchive))
	mux.HandleFunc("POST /clients/{id}/unarchive", s.requireOwner(s.clientUnarchive))
}

// clientScope: owner sees every org; staff only orgs that have a project they are assigned to (contract §3).
func clientScope(u *userLike) (string, []any) {
	if u.Role == string(domain.RoleOwner) {
		return "c.workspace_id = ?", []any{u.WorkspaceID}
	}
	return "c.workspace_id = ? AND c.id IN (SELECT p.client_org_id FROM projects p JOIN project_members m ON m.project_id = p.id WHERE m.user_id = ?)", []any{u.WorkspaceID, u.ID}
}

type userLike = struct {
	ID, Email, Name, Role, WorkspaceID string
}

func (s *Server) loadClient(r *http.Request, id string) (Client, error) {
	u := s.user(r)
	cond, args := clientScope(&userLike{u.ID, u.Email, u.Name, u.Role, u.WorkspaceID})
	var c Client
	err := s.db.R.QueryRowContext(r.Context(), `SELECT c.id, c.name, c.email, c.phone, c.notes, c.created_at, c.archived_at,
		(SELECT count(*) FROM projects p WHERE p.client_org_id = c.id) FROM client_orgs c WHERE c.id = ? AND `+cond, append([]any{id}, args...)...).
		Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Notes, &c.CreatedAt, &c.ArchivedAt, &c.ProjectCount)
	if errors.Is(err, sql.ErrNoRows) {
		return Client{}, errNotFound
	}
	return c, err
}

type clientsPage struct {
	Clients      []Client
	Query        string
	ShowArchived bool
	Page         int
	HasNext      bool
}

func (s *Server) clientsList(w http.ResponseWriter, r *http.Request) {
	u := s.user(r)
	cond, args := clientScope(&userLike{u.ID, u.Email, u.Name, u.Role, u.WorkspaceID})
	d := clientsPage{Query: strings.TrimSpace(r.URL.Query().Get("q")), ShowArchived: r.URL.Query().Get("archived") == "1"}
	var limit, offset int
	d.Page, limit, offset = pageArgs(r)
	if !d.ShowArchived {
		cond += " AND c.archived_at IS NULL"
	}
	if d.Query != "" {
		cond += " AND c.name LIKE ? ESCAPE '\\'"
		args = append(args, "%"+escapeLike(d.Query)+"%")
	}
	rows, err := s.db.R.QueryContext(r.Context(), `SELECT c.id, c.name, c.email, c.phone, c.created_at, c.archived_at,
		(SELECT count(*) FROM projects p WHERE p.client_org_id = c.id) FROM client_orgs c WHERE `+cond+` ORDER BY c.name LIMIT ? OFFSET ?`,
		append(args, limit+1, offset)...)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var c Client
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.CreatedAt, &c.ArchivedAt, &c.ProjectCount); err != nil {
			s.fail(w, r, err)
			return
		}
		d.Clients = append(d.Clients, c)
	}
	if len(d.Clients) > limit {
		d.Clients, d.HasNext = d.Clients[:limit], true
	}
	s.render(w, r, http.StatusOK, "clients", view{Title: "Clients", Data: d})
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func validClient(f map[string]string) error {
	switch {
	case !within(f["name"], 1, 200):
		return invalid("Client name must be 1–200 characters.")
	case f["email"] != "" && (!strings.Contains(f["email"], "@") || len(f["email"]) > 254):
		return invalid("Enter a valid email or leave it empty.")
	case len(f["phone"]) > 60 || len(f["notes"]) > 10000:
		return invalid("Phone or notes are too long.")
	}
	return nil
}

func (s *Server) clientCreate(w http.ResponseWriter, r *http.Request) {
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	if err := validClient(f); err != nil {
		s.fail(w, r, err)
		return
	}
	u, id, now := s.user(r), db.NewID(), db.Now()
	err := s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO client_orgs (id, workspace_id, name, email, phone, notes, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, u.WorkspaceID, f["name"], strings.ToLower(f["email"]), f["phone"], f["notes"], u.ID, now, now); err != nil {
			return err
		}
		return s.audit(r.Context(), tx, r, "client_org.created", "client_org", id, nil)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/clients/"+id, http.StatusSeeOther)
}

type clientPage struct {
	Client   Client
	Contacts []Contact
	Projects []ProjectRow
}

func (s *Server) clientShow(w http.ResponseWriter, r *http.Request) {
	c, err := s.loadClient(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d := clientPage{Client: c}
	rows, err := s.db.R.QueryContext(r.Context(), `SELECT id, name, email, title, created_at, user_id FROM client_contacts WHERE client_org_id = ? AND removed_at IS NULL ORDER BY created_at`, c.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for rows.Next() {
		var ct Contact
		if err := rows.Scan(&ct.ID, &ct.Name, &ct.Email, &ct.Title, &ct.CreatedAt, &ct.UserID); err != nil {
			rows.Close()
			s.fail(w, r, err)
			return
		}
		d.Contacts = append(d.Contacts, ct)
	}
	rows.Close()
	if d.Projects, err = s.listProjectsForClient(r, c.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "client", view{Title: c.Name, Data: d})
}

func (s *Server) clientEdit(w http.ResponseWriter, r *http.Request) {
	c, err := s.loadClient(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	if err := validClient(f); err != nil {
		s.fail(w, r, err)
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE client_orgs SET name = ?, email = ?, phone = ?, notes = ?, updated_at = ? WHERE id = ?`,
			f["name"], strings.ToLower(f["email"]), f["phone"], f["notes"], db.Now(), c.ID)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/clients/"+c.ID, http.StatusSeeOther)
}

func (s *Server) contactCreate(w http.ResponseWriter, r *http.Request) {
	c, err := s.loadClient(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	email := strings.ToLower(f["email"])
	if !within(f["name"], 1, 200) || !strings.Contains(email, "@") || len(email) > 254 || len(f["title"]) > 200 {
		s.fail(w, r, invalid("Contact needs a name and a valid email."))
		return
	}
	id := db.NewID()
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO client_contacts (id, client_org_id, name, email, title, created_at) VALUES (?, ?, ?, ?, ?, ?)`, id, c.ID, f["name"], email, f["title"], db.Now())
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/clients/"+c.ID, http.StatusSeeOther)
}

// contactInvite sends a single-use portal invitation to a contact (contract §3: O and assigned staff).
func (s *Server) contactInvite(w http.ResponseWriter, r *http.Request) {
	c, err := s.loadClient(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	u := s.user(r)
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		var email string
		var userID sql.NullString
		if err := tx.QueryRow(`SELECT email, user_id FROM client_contacts WHERE id = ? AND client_org_id = ? AND removed_at IS NULL`, r.PathValue("cid"), c.ID).Scan(&email, &userID); err != nil {
			return errNotFound
		}
		if userID.Valid {
			return invalid("This contact already has portal access.")
		}
		var n int
		if err := tx.QueryRow(`SELECT count(*) FROM users WHERE email = ?`, email).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return invalid("A user with that email already exists in this workspace.")
		}
		if _, err := tx.Exec(`UPDATE invitations SET revoked_at = ? WHERE contact_id = ? AND accepted_at IS NULL AND revoked_at IS NULL`, db.Now(), r.PathValue("cid")); err != nil {
			return err
		}
		var ws string
		if err := tx.QueryRow(`SELECT name FROM workspaces WHERE id = ?`, u.WorkspaceID).Scan(&ws); err != nil {
			return err
		}
		_, err := s.createInvitation(r, tx, email, "client", r.PathValue("cid"), u.Name, ws)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/clients/"+c.ID, http.StatusSeeOther)
}

func (s *Server) contactRemove(w http.ResponseWriter, r *http.Request) {
	c, err := s.loadClient(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		res, err := tx.Exec(`UPDATE client_contacts SET removed_at = ? WHERE id = ? AND client_org_id = ? AND removed_at IS NULL`, db.Now(), r.PathValue("cid"), c.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errNotFound
		}
		// Contract §2: removing a contact ends their access immediately.
		if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = (SELECT user_id FROM client_contacts WHERE id = ?)`, r.PathValue("cid")); err != nil {
			return err
		}
		return s.audit(r.Context(), tx, r, "member.removed", "client_contact", r.PathValue("cid"), map[string]any{"client_org_id": c.ID})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/clients/"+c.ID, http.StatusSeeOther)
}

func (s *Server) clientArchive(w http.ResponseWriter, r *http.Request) {
	c, err := s.loadClient(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		var open int
		if err := tx.QueryRow(`SELECT count(*) FROM projects WHERE client_org_id = ? AND status = 'active'`, c.ID).Scan(&open); err != nil {
			return err
		}
		if open > 0 {
			return invalid("Close every project of this client before archiving it.")
		}
		if _, err := tx.Exec(`UPDATE client_orgs SET archived_at = ?, updated_at = ? WHERE id = ?`, db.Now(), db.Now(), c.ID); err != nil {
			return err
		}
		return s.audit(r.Context(), tx, r, "client_org.archived", "client_org", c.ID, nil)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/clients/"+c.ID, http.StatusSeeOther)
}

func (s *Server) clientUnarchive(w http.ResponseWriter, r *http.Request) {
	c, err := s.loadClient(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE client_orgs SET archived_at = NULL, updated_at = ? WHERE id = ?`, db.Now(), c.ID)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/clients/"+c.ID, http.StatusSeeOther)
}
