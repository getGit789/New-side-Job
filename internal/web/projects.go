package web

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"briefrelay/internal/db"
	"briefrelay/internal/domain"
)

type ProjectRow struct {
	ID, Name, ClientID, ClientName, Status, UpdatedAt string
}

type Member struct {
	ID, Name, Email string
}

type Milestone struct {
	ID, Title, TargetDate, Status, Visibility, LatestUpdate string
}

type DeliverableRow struct {
	ID, Title, LatestState string
	Required               bool
	LatestNumber           int
}

type Invoice struct {
	ID, Number, Currency, DueDate, Status, PaymentURL, Visibility, CreatedAt string
	AmountCents                                                              int64
	FileID                                                                   sql.NullString
}

type AuditRow struct {
	Action, TargetType, TargetID, Actor, Meta, CreatedAt string
}

func (s *Server) projectRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /projects", s.requireStaff(s.projectsList))
	mux.HandleFunc("POST /projects", s.requireStaff(s.projectCreate))
	mux.HandleFunc("GET /projects/{id}", s.requireStaff(s.projectShow))
	mux.HandleFunc("POST /projects/{id}", s.requireStaff(s.projectEdit))
	mux.HandleFunc("POST /projects/{id}/close", s.requireStaff(s.projectClose))
	mux.HandleFunc("POST /projects/{id}/reopen", s.requireOwner(s.projectReopen))
	mux.HandleFunc("POST /projects/{id}/members", s.requireOwner(s.memberAdd))
	mux.HandleFunc("POST /projects/{id}/members/{uid}/remove", s.requireOwner(s.memberRemove))
	mux.HandleFunc("POST /projects/{id}/milestones", s.requireStaff(s.milestoneCreate))
	mux.HandleFunc("POST /milestones/{id}", s.requireStaff(s.milestoneEdit))
	mux.HandleFunc("POST /milestones/{id}/delete", s.requireStaff(s.milestoneDelete))
}

func (s *Server) listProjects(r *http.Request, q, status string, limit, offset int) ([]ProjectRow, error) {
	cond, args := projectScope(s.user(r))
	if status == "active" || status == "closed" {
		cond += " AND p.status = ?"
		args = append(args, status)
	}
	if q != "" {
		cond += " AND (p.name LIKE ? ESCAPE '\\' OR c.name LIKE ? ESCAPE '\\')"
		like := "%" + escapeLike(q) + "%"
		args = append(args, like, like)
	}
	rows, err := s.db.R.QueryContext(r.Context(), `SELECT p.id, p.name, c.id, c.name, p.status, p.updated_at FROM projects p JOIN client_orgs c ON c.id = p.client_org_id
		WHERE `+cond+` ORDER BY p.updated_at DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProjectRow
	for rows.Next() {
		var p ProjectRow
		if err := rows.Scan(&p.ID, &p.Name, &p.ClientID, &p.ClientName, &p.Status, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Server) listProjectsForClient(r *http.Request, clientID string) ([]ProjectRow, error) {
	cond, args := projectScope(s.user(r))
	rows, err := s.db.R.QueryContext(r.Context(), `SELECT p.id, p.name, c.id, c.name, p.status, p.updated_at FROM projects p JOIN client_orgs c ON c.id = p.client_org_id
		WHERE p.client_org_id = ? AND `+cond+` ORDER BY p.updated_at DESC LIMIT 100`, append([]any{clientID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProjectRow
	for rows.Next() {
		var p ProjectRow
		if err := rows.Scan(&p.ID, &p.Name, &p.ClientID, &p.ClientName, &p.Status, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type projectsPage struct {
	Projects []ProjectRow
	Clients  []Client // for the new-project form
	Query    string
	Status   string
	Page     int
	HasNext  bool
}

// orgNames lists active client organizations (name only) so any staff member can start a project (contract §3).
func (s *Server) orgNames(r *http.Request) ([]Client, error) {
	rows, err := s.db.R.QueryContext(r.Context(), `SELECT id, name FROM client_orgs WHERE workspace_id = ? AND archived_at IS NULL ORDER BY name LIMIT 500`, s.user(r).WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Client
	for rows.Next() {
		var c Client
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Server) projectsList(w http.ResponseWriter, r *http.Request) {
	d := projectsPage{Query: strings.TrimSpace(r.URL.Query().Get("q")), Status: r.URL.Query().Get("status")}
	var limit, offset int
	d.Page, limit, offset = pageArgs(r)
	var err error
	if d.Projects, err = s.listProjects(r, d.Query, d.Status, limit+1, offset); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(d.Projects) > limit {
		d.Projects, d.HasNext = d.Projects[:limit], true
	}
	if d.Clients, err = s.orgNames(r); err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "projects", view{Title: "Projects", Data: d})
}

func (s *Server) projectCreate(w http.ResponseWriter, r *http.Request) {
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	if !within(f["name"], 1, 200) || len(f["summary"]) > 10000 {
		s.fail(w, r, invalid("Project name must be 1–200 characters."))
		return
	}
	u, id, now := s.user(r), db.NewID(), db.Now()
	err := s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT count(*) FROM client_orgs WHERE id = ? AND workspace_id = ? AND archived_at IS NULL`, f["client_org_id"], u.WorkspaceID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return invalid("Choose a client.")
		}
		if _, err := tx.Exec(`INSERT INTO projects (id, workspace_id, client_org_id, name, summary, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, u.WorkspaceID, f["client_org_id"], f["name"], f["summary"], u.ID, now, now); err != nil {
			return err
		}
		// The creator is always a member, so staff can see what they created.
		if _, err := tx.Exec(`INSERT INTO project_members (project_id, user_id, created_at) VALUES (?, ?, ?)`, id, u.ID, now); err != nil {
			return err
		}
		return s.audit(r.Context(), tx, r, "project.created", "project", id, map[string]any{"project_id": id})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects/"+id, http.StatusSeeOther)
}

type projectPage struct {
	Project      Project
	Members      []Member
	Candidates   []Member // staff who are not yet members (owner only)
	Milestones   []Milestone
	Deliverables []DeliverableRow
	Invoices     []Invoice
	Activity     []AuditRow
	IntakeStatus string
}

func (s *Server) projectShow(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d, err := s.projectData(r, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "project", view{Title: p.Name, Data: d})
}

func (s *Server) projectData(r *http.Request, p Project) (projectPage, error) {
	d := projectPage{Project: p}
	ctx := r.Context()
	q := func(query string, args []any, scan func(*sql.Rows) error) error {
		rows, err := s.db.R.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := scan(rows); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	err := q(`SELECT u.id, u.name, u.email FROM project_members m JOIN users u ON u.id = m.user_id WHERE m.project_id = ? ORDER BY u.name`, []any{p.ID}, func(rows *sql.Rows) error {
		var m Member
		err := rows.Scan(&m.ID, &m.Name, &m.Email)
		d.Members = append(d.Members, m)
		return err
	})
	if err != nil {
		return d, err
	}
	if s.role(r) == domain.RoleOwner {
		err = q(`SELECT u.id, u.name, u.email FROM memberships ms JOIN users u ON u.id = ms.user_id WHERE ms.workspace_id = ? AND ms.role IN ('owner','staff')
			AND u.id NOT IN (SELECT user_id FROM project_members WHERE project_id = ?) ORDER BY u.name`, []any{p.WorkspaceID, p.ID}, func(rows *sql.Rows) error {
			var m Member
			err := rows.Scan(&m.ID, &m.Name, &m.Email)
			d.Candidates = append(d.Candidates, m)
			return err
		})
		if err != nil {
			return d, err
		}
	}
	err = q(`SELECT id, title, target_date, status, visibility, latest_update FROM milestones WHERE project_id = ? ORDER BY sort_order, target_date, created_at`, []any{p.ID}, func(rows *sql.Rows) error {
		var m Milestone
		err := rows.Scan(&m.ID, &m.Title, &m.TargetDate, &m.Status, &m.Visibility, &m.LatestUpdate)
		d.Milestones = append(d.Milestones, m)
		return err
	})
	if err != nil {
		return d, err
	}
	err = q(`SELECT d.id, d.title, d.required, COALESCE(v.number, 0), COALESCE(v.state, '') FROM deliverables d
		LEFT JOIN deliverable_versions v ON v.deliverable_id = d.id AND v.number = (SELECT max(number) FROM deliverable_versions WHERE deliverable_id = d.id)
		WHERE d.project_id = ? ORDER BY d.sort_order, d.created_at`, []any{p.ID}, func(rows *sql.Rows) error {
		var x DeliverableRow
		err := rows.Scan(&x.ID, &x.Title, &x.Required, &x.LatestNumber, &x.LatestState)
		d.Deliverables = append(d.Deliverables, x)
		return err
	})
	if err != nil {
		return d, err
	}
	err = q(`SELECT id, number, amount_cents, currency, due_date, status, payment_url, visibility, file_id, created_at FROM invoices WHERE project_id = ? ORDER BY created_at DESC`, []any{p.ID}, func(rows *sql.Rows) error {
		var i Invoice
		err := rows.Scan(&i.ID, &i.Number, &i.AmountCents, &i.Currency, &i.DueDate, &i.Status, &i.PaymentURL, &i.Visibility, &i.FileID, &i.CreatedAt)
		d.Invoices = append(d.Invoices, i)
		return err
	})
	if err != nil {
		return d, err
	}
	err = q(`SELECT a.action, a.target_type, a.target_id, COALESCE(u.name, ''), a.meta, a.created_at FROM audit_events a LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.workspace_id = ? AND json_extract(a.meta, '$.project_id') = ? ORDER BY a.id DESC LIMIT 30`, []any{p.WorkspaceID, p.ID}, func(rows *sql.Rows) error {
		var a AuditRow
		err := rows.Scan(&a.Action, &a.TargetType, &a.TargetID, &a.Actor, &a.Meta, &a.CreatedAt)
		d.Activity = append(d.Activity, a)
		return err
	})
	if err != nil {
		return d, err
	}
	d.IntakeStatus = "No intake submitted yet."
	var n int
	if err := s.db.R.QueryRowContext(ctx, `SELECT count(*) FROM intake_responses WHERE project_id = ? AND status = 'submitted'`, p.ID).Scan(&n); err == nil && n > 0 {
		d.IntakeStatus = "Intake submitted."
	}
	return d, nil
}

func (s *Server) projectEdit(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadWritableProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	if !within(f["name"], 1, 200) || len(f["summary"]) > 10000 {
		s.fail(w, r, invalid("Project name must be 1–200 characters."))
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE projects SET name = ?, summary = ?, updated_at = ? WHERE id = ?`, f["name"], f["summary"], db.Now(), p.ID)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects/"+p.ID, http.StatusSeeOther)
}

func (s *Server) projectClose(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadWritableProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE projects SET status = 'closed', closed_at = ?, closed_by = ?, updated_at = ? WHERE id = ?`, db.Now(), s.user(r).ID, db.Now(), p.ID); err != nil {
			return err
		}
		return s.audit(r.Context(), tx, r, "project.closed", "project", p.ID, map[string]any{"project_id": p.ID})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects/"+p.ID, http.StatusSeeOther)
}

func (s *Server) projectReopen(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	if !p.Closed() {
		s.fail(w, r, invalid("Project is not closed."))
		return
	}
	if !within(f["reason"], 3, 1000) {
		s.fail(w, r, invalid("Give a reason for reopening (3–1000 characters)."))
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE projects SET status = 'active', closed_at = NULL, closed_by = NULL, updated_at = ? WHERE id = ?`, db.Now(), p.ID); err != nil {
			return err
		}
		return s.audit(r.Context(), tx, r, "project.reopened", "project", p.ID, map[string]any{"project_id": p.ID, "reason": f["reason"]})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects/"+p.ID, http.StatusSeeOther)
}

func (s *Server) memberAdd(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadWritableProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT count(*) FROM memberships WHERE workspace_id = ? AND user_id = ? AND role IN ('owner','staff')`, p.WorkspaceID, f["user_id"]).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return invalid("That user is not a staff member of this workspace.")
		}
		_, err := tx.Exec(`INSERT OR IGNORE INTO project_members (project_id, user_id, created_at) VALUES (?, ?, ?)`, p.ID, f["user_id"], db.Now())
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects/"+p.ID, http.StatusSeeOther)
}

func (s *Server) memberRemove(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadWritableProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM project_members WHERE project_id = ? AND user_id = ?`, p.ID, r.PathValue("uid"))
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects/"+p.ID, http.StatusSeeOther)
}

func validMilestone(f map[string]string) error {
	switch {
	case !within(f["title"], 1, 200):
		return invalid("Milestone title must be 1–200 characters.")
	case f["target_date"] != "" && len(f["target_date"]) != 10:
		return invalid("Target date must look like 2026-12-31.")
	case f["status"] != "" && f["status"] != "planned" && f["status"] != "in_progress" && f["status"] != "done":
		return invalid("Unknown milestone status.")
	case f["visibility"] != "internal" && f["visibility"] != "client":
		return invalid("Visibility must be internal or client.")
	case len(f["latest_update"]) > 10000:
		return invalid("Update is too long.")
	}
	return nil
}

func (s *Server) milestoneCreate(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadWritableProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	if err := validMilestone(f); err != nil {
		s.fail(w, r, err)
		return
	}
	id, now := db.NewID(), db.Now()
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO milestones (id, project_id, title, target_date, visibility, latest_update, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, p.ID, f["title"], f["target_date"], f["visibility"], f["latest_update"], s.user(r).ID, now, now)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects/"+p.ID, http.StatusSeeOther)
}

func (s *Server) milestoneEdit(w http.ResponseWriter, r *http.Request) {
	p, err := s.projectOf(r, "milestones", r.PathValue("id"), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	if err := validMilestone(f); err != nil {
		s.fail(w, r, err)
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		var oldVis string
		if err := tx.QueryRow(`SELECT visibility FROM milestones WHERE id = ?`, r.PathValue("id")).Scan(&oldVis); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE milestones SET title = ?, target_date = ?, status = ?, visibility = ?, latest_update = ?, updated_at = ? WHERE id = ?`,
			f["title"], f["target_date"], f["status"], f["visibility"], f["latest_update"], db.Now(), r.PathValue("id")); err != nil {
			return err
		}
		if oldVis != f["visibility"] { // contract §3: visibility changes are audited
			return s.audit(r.Context(), tx, r, "milestone.visibility_changed", "milestone", r.PathValue("id"), map[string]any{"project_id": p.ID, "from": oldVis, "to": f["visibility"]})
		}
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects/"+p.ID, http.StatusSeeOther)
}

func (s *Server) milestoneDelete(w http.ResponseWriter, r *http.Request) {
	p, err := s.projectOf(r, "milestones", r.PathValue("id"), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		res, err := tx.Exec(`DELETE FROM milestones WHERE id = ? AND status = 'planned'`, r.PathValue("id"))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return invalid("Only planned milestones can be deleted. Mark others as done instead.")
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errInvalid) {
			s.fail(w, r, err)
			return
		}
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects/"+p.ID, http.StatusSeeOther)
}
