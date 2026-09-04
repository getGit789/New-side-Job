package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"briefrelay/internal/domain"
)

// The overview answers one question for whoever is logged in: what needs me next?
// Every pointer links to the place where the action happens.

type pointer struct {
	Title, Detail, URL string
	Urgent             bool
}

type step struct {
	Label, URL string
	Done       bool
}

type homePage struct {
	Greeting                string
	ActiveProjects, Clients int
	PendingReviews          int
	Recent                  []ProjectRow
	Pointers                []pointer
	Steps                   []step // first-run guide; empty once every step is done
	AllDone                 bool
}

func (s *Server) greeting(name string) string {
	if first, _, ok := strings.Cut(strings.TrimSpace(name), " "); ok && first != "" {
		name = first
	}
	h := time.Now().In(s.prefs.Load().loc).Hour()
	switch {
	case h < 5:
		return "Still up, " + name
	case h < 12:
		return "Good morning, " + name
	case h < 18:
		return "Good afternoon, " + name
	default:
		return "Good evening, " + name
	}
}

// query runs a read and hands each row to scan. It keeps the pointer queries below short.
func (s *Server) query(r *http.Request, sqlText string, args []any, scan func(*sql.Rows) error) error {
	rows, err := s.db.R.QueryContext(r.Context(), sqlText, args...)
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

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	u := s.user(r)
	if u == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	d := homePage{Greeting: s.greeting(u.Name)}
	var err error
	if domain.Role(u.Role).IsStaff() {
		err = s.staffHome(r, &d)
	} else {
		err = s.clientHome(r, &d)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "home", view{Title: "Overview", Data: &d})
}

const latestVersion = `(SELECT state FROM deliverable_versions WHERE deliverable_id = d.id ORDER BY number DESC LIMIT 1)`

func (s *Server) staffHome(r *http.Request, d *homePage) error {
	u := s.user(r)
	cond, args := projectScope(u)
	today := time.Now().In(s.prefs.Load().loc).Format("2006-01-02")
	if err := s.db.R.QueryRowContext(r.Context(), `SELECT
		(SELECT count(*) FROM projects p WHERE p.status = 'active' AND `+cond+`),
		(SELECT count(*) FROM client_orgs WHERE workspace_id = ? AND archived_at IS NULL),
		(SELECT count(*) FROM deliverable_versions v JOIN deliverables d ON d.id = v.deliverable_id JOIN projects p ON p.id = d.project_id WHERE v.state = 'shared' AND `+cond+`)`,
		append(append(append([]any{}, args...), u.WorkspaceID), args...)...).Scan(&d.ActiveProjects, &d.Clients, &d.PendingReviews); err != nil {
		return err
	}
	add := func(title, detail, url string, urgent bool) {
		d.Pointers = append(d.Pointers, pointer{Title: title, Detail: detail, URL: url, Urgent: urgent})
	}
	// Revision requested and no newer version yet: the ball is in our court.
	err := s.query(r, `SELECT d.id, d.title, p.name FROM deliverables d JOIN projects p ON p.id = d.project_id
		WHERE p.status = 'active' AND `+cond+` AND `+latestVersion+` = 'revision_requested' ORDER BY d.updated_at LIMIT 10`, args, func(rows *sql.Rows) error {
		var id, title, project string
		if err := rows.Scan(&id, &title, &project); err != nil {
			return err
		}
		add(title+": the client asked for a revision", project+". Add the next version.", "/deliverables/"+id, true)
		return nil
	})
	if err != nil {
		return err
	}
	// Overdue invoices.
	err = s.query(r, `SELECT i.number, i.due_date, p.id, p.name FROM invoices i JOIN projects p ON p.id = i.project_id
		WHERE i.status = 'sent' AND i.due_date != '' AND i.due_date < ? AND `+cond+` ORDER BY i.due_date LIMIT 10`, append([]any{today}, args...), func(rows *sql.Rows) error {
		var number, due, pid, project string
		if err := rows.Scan(&number, &due, &pid, &project); err != nil {
			return err
		}
		add("Invoice "+number+" is overdue", project+", due "+due+". Follow up or mark it paid.", "/projects/"+pid, true)
		return nil
	})
	if err != nil {
		return err
	}
	// Ready for sign-off: every required deliverable approved or waived, project still open.
	err = s.query(r, `SELECT p.id, p.name FROM projects p WHERE p.status = 'active' AND `+cond+`
		AND EXISTS (SELECT 1 FROM deliverables d WHERE d.project_id = p.id AND d.required = 1)
		AND NOT EXISTS (SELECT 1 FROM deliverables d WHERE d.project_id = p.id AND d.required = 1
			AND NOT EXISTS (SELECT 1 FROM waivers w WHERE w.deliverable_id = d.id) AND COALESCE(`+latestVersion+`, '') != 'approved')
		ORDER BY p.updated_at LIMIT 10`, args, func(rows *sql.Rows) error {
		var pid, project string
		if err := rows.Scan(&pid, &project); err != nil {
			return err
		}
		add(project+" is ready for sign-off", "Everything required is approved. Ask the client to sign off.", "/projects/"+pid, false)
		return nil
	})
	if err != nil {
		return err
	}
	// Drafts nobody has shared yet.
	err = s.query(r, `SELECT d.id, d.title, p.name FROM deliverables d JOIN projects p ON p.id = d.project_id
		WHERE p.status = 'active' AND `+cond+` AND `+latestVersion+` = 'draft' ORDER BY d.updated_at LIMIT 10`, args, func(rows *sql.Rows) error {
		var id, title, project string
		if err := rows.Scan(&id, &title, &project); err != nil {
			return err
		}
		add(title+" has a draft the client cannot see", project+". Share it when it is ready.", "/deliverables/"+id, false)
		return nil
	})
	if err != nil {
		return err
	}
	// Waiting on the client for more than a week.
	err = s.query(r, `SELECT d.id, d.title, p.name, v.number, v.shared_at FROM deliverable_versions v JOIN deliverables d ON d.id = v.deliverable_id JOIN projects p ON p.id = d.project_id
		WHERE v.state = 'shared' AND v.shared_at < ? AND `+cond+` ORDER BY v.shared_at LIMIT 10`, append([]any{time.Now().Add(-7 * 24 * time.Hour).UTC().Format(time.RFC3339)}, args...), func(rows *sql.Rows) error {
		var id, title, project, sharedAt string
		var n int
		if err := rows.Scan(&id, &title, &project, &n, &sharedAt); err != nil {
			return err
		}
		add(fmt.Sprintf("%s v%d has waited over a week", title, n), project+". A short nudge to the client helps.", "/deliverables/"+id, false)
		return nil
	})
	if err != nil {
		return err
	}
	// Contacts without portal access.
	ccond, cargs := clientScope(&userLike{u.ID, u.Email, u.Name, u.Role, u.WorkspaceID})
	err = s.query(r, `SELECT ct.name, c.id, c.name FROM client_contacts ct JOIN client_orgs c ON c.id = ct.client_org_id
		WHERE ct.user_id IS NULL AND ct.removed_at IS NULL AND c.archived_at IS NULL AND `+ccond+` ORDER BY ct.created_at LIMIT 10`, cargs, func(rows *sql.Rows) error {
		var name, cid, org string
		if err := rows.Scan(&name, &cid, &org); err != nil {
			return err
		}
		add(name+" at "+org+" has no portal access yet", "Send the invitation so they can review and approve.", "/clients/"+cid, false)
		return nil
	})
	if err != nil {
		return err
	}
	// Briefs not started.
	err = s.query(r, `SELECT p.id, p.name FROM projects p WHERE p.status = 'active' AND `+cond+`
		AND NOT EXISTS (SELECT 1 FROM intake_responses i WHERE i.project_id = p.id) ORDER BY p.created_at LIMIT 10`, args, func(rows *sql.Rows) error {
		var pid, project string
		if err := rows.Scan(&pid, &project); err != nil {
			return err
		}
		add(project+": the brief is not started", "The client fills it in on their project page. Remind them, or skip it if you already know the scope.", "/projects/"+pid, false)
		return nil
	})
	if err != nil {
		return err
	}

	// First-run guide (owner and staff alike; hidden once complete).
	var clients, invited, projects, shared int
	if err := s.db.R.QueryRowContext(r.Context(), `SELECT
		(SELECT count(*) FROM client_orgs WHERE workspace_id = ?),
		(SELECT count(*) FROM invitations WHERE workspace_id = ? AND role = 'client') + (SELECT count(*) FROM client_contacts c JOIN client_orgs o ON o.id = c.client_org_id WHERE o.workspace_id = ? AND c.user_id IS NOT NULL),
		(SELECT count(*) FROM projects WHERE workspace_id = ?),
		(SELECT count(*) FROM deliverable_versions v JOIN deliverables d ON d.id = v.deliverable_id JOIN projects p ON p.id = d.project_id WHERE p.workspace_id = ? AND v.state != 'draft')`,
		u.WorkspaceID, u.WorkspaceID, u.WorkspaceID, u.WorkspaceID, u.WorkspaceID).Scan(&clients, &invited, &projects, &shared); err != nil {
		return err
	}
	d.Steps = []step{
		{"Add a client and a contact", "/clients", clients > 0},
		{"Send the contact a portal invitation", "/clients", invited > 0},
		{"Create a project for that client", "/projects", projects > 0},
		{"Add a deliverable and share the first version", "/projects", shared > 0},
	}
	d.AllDone = clients > 0 && invited > 0 && projects > 0 && shared > 0
	d.Recent, err = s.listProjects(r, "", "active", 8, 0)
	return err
}

func (s *Server) clientHome(r *http.Request, d *homePage) error {
	u := s.user(r)
	cond, args := projectScope(u)
	add := func(title, detail, url string, urgent bool) {
		d.Pointers = append(d.Pointers, pointer{Title: title, Detail: detail, URL: url, Urgent: urgent})
	}
	err := s.query(r, `SELECT d.id, d.title, p.name, v.number FROM deliverable_versions v JOIN deliverables d ON d.id = v.deliverable_id JOIN projects p ON p.id = d.project_id
		WHERE v.state = 'shared' AND p.status = 'active' AND `+cond+` ORDER BY v.shared_at LIMIT 10`, args, func(rows *sql.Rows) error {
		var id, title, project string
		var n int
		if err := rows.Scan(&id, &title, &project, &n); err != nil {
			return err
		}
		add(fmt.Sprintf("%s v%d is waiting for your decision", title, n), project+". Approve it or ask for a revision.", "/portal/deliverables/"+id, true)
		return nil
	})
	if err != nil {
		return err
	}
	err = s.query(r, `SELECT p.id, p.name FROM projects p WHERE p.status = 'active' AND `+cond+`
		AND NOT EXISTS (SELECT 1 FROM intake_responses i WHERE i.project_id = p.id AND i.status = 'submitted') ORDER BY p.created_at LIMIT 10`, args, func(rows *sql.Rows) error {
		var pid, project string
		if err := rows.Scan(&pid, &project); err != nil {
			return err
		}
		add(project+": tell us what you need", "The brief takes a few minutes and helps the work start right.", "/portal/projects/"+pid+"/intake", false)
		return nil
	})
	if err != nil {
		return err
	}
	err = s.query(r, `SELECT i.number, i.due_date, p.id, p.name FROM invoices i JOIN projects p ON p.id = i.project_id
		WHERE i.status = 'sent' AND i.visibility = 'client' AND `+cond+` ORDER BY i.due_date LIMIT 10`, args, func(rows *sql.Rows) error {
		var number, due, pid, project string
		if err := rows.Scan(&number, &due, &pid, &project); err != nil {
			return err
		}
		detail := project + "."
		if due != "" {
			detail = project + ", due " + due + "."
		}
		add("Invoice "+number+" is open", detail, "/portal/projects/"+pid, false)
		return nil
	})
	if err != nil {
		return err
	}
	err = s.query(r, `SELECT p.id, p.name FROM projects p WHERE p.status = 'active' AND `+cond+`
		AND EXISTS (SELECT 1 FROM deliverables d WHERE d.project_id = p.id AND d.required = 1)
		AND NOT EXISTS (SELECT 1 FROM deliverables d WHERE d.project_id = p.id AND d.required = 1
			AND NOT EXISTS (SELECT 1 FROM waivers w WHERE w.deliverable_id = d.id) AND COALESCE(`+latestVersion+`, '') != 'approved')
		ORDER BY p.updated_at LIMIT 10`, args, func(rows *sql.Rows) error {
		var pid, project string
		if err := rows.Scan(&pid, &project); err != nil {
			return err
		}
		add(project+" is ready for your sign-off", "Everything required is approved. Signing off completes the project.", "/portal/projects/"+pid, true)
		return nil
	})
	if err != nil {
		return err
	}
	d.Recent, err = s.listProjects(r, "", "", 100, 0)
	return err
}
