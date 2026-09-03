package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"briefrelay/internal/db"
	"briefrelay/internal/domain"
)

// Client portal. Every loader goes through projectScope, which limits clients to their own organization.
// Content filters (drafts, internal comments, internal milestones and invoices) live in the queries here.

type Comment struct {
	ID, Author, Body, Visibility, CreatedAt string
}

type Signoff struct {
	By, Snapshot, CreatedAt string
}

type handoffRow struct {
	ID, Title, LatestState, Waiver string
	LatestNumber                   int
	Satisfied                      bool
}

type handoffInfo struct {
	Rows     []handoffRow
	Ready    bool // every required deliverable approved or waived, and at least one exists
	Signoffs []Signoff
}

type intakeField struct{ Key, Label string }

// Fixed intake structure (contract: no form builder in v1).
var intakeFields = []intakeField{
	{"goals", "What should this project achieve?"},
	{"audience", "Who is it for?"},
	{"deliverables", "What do you expect to receive?"},
	{"deadline", "Deadline or key dates"},
	{"budget", "Budget range"},
	{"references", "Examples or references you like"},
	{"notes", "Anything else we should know"},
}

type Intake struct {
	ID, Status, SubmittedAt, SubmittedBy string
	Version                              int
	Answers                              map[string]string
}

func (s *Server) portalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /portal/projects/{id}", s.requireClient(s.portalProject))
	mux.HandleFunc("GET /portal/deliverables/{id}", s.requireClient(s.portalDeliverable))
	mux.HandleFunc("POST /portal/versions/{id}/comment", s.requireClient(s.portalComment))
	mux.HandleFunc("POST /portal/versions/{id}/decide", s.requireClient(s.portalDecide))
	mux.HandleFunc("GET /portal/versions/{id}/download", s.requireClient(s.portalDownload))
	mux.HandleFunc("GET /portal/invoices/{id}/document", s.requireClient(s.portalInvoiceDocument))
	mux.HandleFunc("GET /portal/projects/{id}/intake", s.requireClient(s.portalIntake))
	mux.HandleFunc("POST /portal/projects/{id}/intake", s.requireClient(s.portalIntakeSave))
	mux.HandleFunc("POST /portal/projects/{id}/intake/comment", s.requireClient(s.portalIntakeComment))
	mux.HandleFunc("POST /portal/projects/{id}/signoff", s.requireClient(s.portalSignoff))
}

// ---- shared loaders (used by staff pages too) ----

func (s *Server) loadComments(r *http.Request, targetType, targetID string, includeInternal bool) ([]Comment, error) {
	cond := ""
	if !includeInternal {
		cond = " AND c.visibility = 'client'"
	}
	rows, err := s.db.R.QueryContext(r.Context(), `SELECT c.id, COALESCE(u.name, ''), CASE WHEN c.deleted_at IS NULL THEN c.body ELSE '(deleted)' END, c.visibility, c.created_at
		FROM comments c LEFT JOIN users u ON u.id = c.author_id WHERE c.target_type = ? AND c.target_id = ?`+cond+` ORDER BY c.created_at`, targetType, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.Author, &c.Body, &c.Visibility, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Server) addComment(tx *sql.Tx, r *http.Request, p Project, targetType, targetID, visibility, body string) (string, error) {
	if !within(body, 1, 10000) {
		return "", invalid("Comment must be 1–10,000 characters.")
	}
	if visibility != "internal" && visibility != "client" {
		return "", invalid("Comment visibility must be internal or client.")
	}
	id := db.NewID()
	_, err := tx.Exec(`INSERT INTO comments (id, target_type, target_id, project_id, author_id, body, visibility, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, targetType, targetID, p.ID, s.user(r).ID, body, visibility, db.Now())
	return id, err
}

func (s *Server) loadHandoff(r *http.Request, p Project) (handoffInfo, error) {
	var h handoffInfo
	rows, err := s.db.R.QueryContext(r.Context(), `SELECT d.id, d.title, COALESCE(v.number, 0), COALESCE(v.state, ''), COALESCE((SELECT reason FROM waivers WHERE deliverable_id = d.id ORDER BY created_at DESC LIMIT 1), '')
		FROM deliverables d LEFT JOIN deliverable_versions v ON v.deliverable_id = d.id AND v.number = (SELECT max(number) FROM deliverable_versions WHERE deliverable_id = d.id)
		WHERE d.project_id = ? AND d.required = 1 ORDER BY d.sort_order, d.created_at`, p.ID)
	if err != nil {
		return h, err
	}
	defer rows.Close()
	h.Ready = true
	for rows.Next() {
		var x handoffRow
		if err := rows.Scan(&x.ID, &x.Title, &x.LatestNumber, &x.LatestState, &x.Waiver); err != nil {
			return h, err
		}
		x.Satisfied = x.LatestState == string(domain.Approved) || x.Waiver != ""
		h.Ready = h.Ready && x.Satisfied
		h.Rows = append(h.Rows, x)
	}
	if len(h.Rows) == 0 {
		h.Ready = false
	}
	srows, err := s.db.R.QueryContext(r.Context(), `SELECT COALESCE(u.name, ''), s.snapshot, s.created_at FROM signoffs s LEFT JOIN users u ON u.id = s.by_user WHERE s.project_id = ? ORDER BY s.created_at DESC`, p.ID)
	if err != nil {
		return h, err
	}
	defer srows.Close()
	for srows.Next() {
		var so Signoff
		if err := srows.Scan(&so.By, &so.Snapshot, &so.CreatedAt); err == nil {
			h.Signoffs = append(h.Signoffs, so)
		}
	}
	return h, nil
}

// loadIntake returns the newest intake row for a project (draft or submitted), or nil.
func (s *Server) loadIntake(r *http.Request, projectID string) (*Intake, error) {
	var in Intake
	var raw string
	var subAt, subBy sql.NullString
	err := s.db.R.QueryRowContext(r.Context(), `SELECT i.id, i.status, i.version, i.answers, i.submitted_at, COALESCE(u.name, '') FROM intake_responses i LEFT JOIN users u ON u.id = i.submitted_by
		WHERE i.project_id = ? ORDER BY i.version DESC LIMIT 1`, projectID).Scan(&in.ID, &in.Status, &in.Version, &raw, &subAt, &subBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	in.SubmittedAt, in.SubmittedBy = subAt.String, subBy.String
	in.Answers = map[string]string{}
	json.Unmarshal([]byte(raw), &in.Answers)
	return &in, nil
}

// ---- portal pages ----

type portalProjectPage struct {
	Project      Project
	Milestones   []Milestone
	Deliverables []DeliverableRow
	Invoices     []Invoice
	Intake       *Intake
	Handoff      handoffInfo
}

func (s *Server) portalProject(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d := portalProjectPage{Project: p}
	ctx := r.Context()
	rows, err := s.db.R.QueryContext(ctx, `SELECT id, title, target_date, status, visibility, latest_update FROM milestones WHERE project_id = ? AND visibility = 'client' ORDER BY sort_order, target_date, created_at`, p.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.ID, &m.Title, &m.TargetDate, &m.Status, &m.Visibility, &m.LatestUpdate); err == nil {
			d.Milestones = append(d.Milestones, m)
		}
	}
	rows.Close()
	// Newest non-draft version only: drafts do not exist for clients.
	rows, err = s.db.R.QueryContext(ctx, `SELECT d.id, d.title, d.required, COALESCE(v.number, 0), COALESCE(v.state, '') FROM deliverables d
		LEFT JOIN deliverable_versions v ON v.deliverable_id = d.id AND v.number = (SELECT max(number) FROM deliverable_versions WHERE deliverable_id = d.id AND state != 'draft')
		WHERE d.project_id = ? AND v.id IS NOT NULL ORDER BY d.sort_order, d.created_at`, p.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for rows.Next() {
		var x DeliverableRow
		if err := rows.Scan(&x.ID, &x.Title, &x.Required, &x.LatestNumber, &x.LatestState); err == nil {
			d.Deliverables = append(d.Deliverables, x)
		}
	}
	rows.Close()
	rows, err = s.db.R.QueryContext(ctx, `SELECT id, number, amount_cents, currency, due_date, status, payment_url, visibility, file_id, created_at FROM invoices WHERE project_id = ? AND visibility = 'client' AND status != 'draft' ORDER BY created_at DESC`, p.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for rows.Next() {
		var i Invoice
		if err := rows.Scan(&i.ID, &i.Number, &i.AmountCents, &i.Currency, &i.DueDate, &i.Status, &i.PaymentURL, &i.Visibility, &i.FileID, &i.CreatedAt); err == nil {
			d.Invoices = append(d.Invoices, i)
		}
	}
	rows.Close()
	if d.Intake, err = s.loadIntake(r, p.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	if d.Handoff, err = s.loadHandoff(r, p); err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "portal_project", view{Title: p.Name, Data: d})
}

type portalDeliverablePage struct {
	Deliverable Deliverable
	Project     Project
	Versions    []VersionRow
}

func (s *Server) portalDeliverable(w http.ResponseWriter, r *http.Request) {
	d, p, err := s.loadDeliverable(r, r.PathValue("id"), false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pg := portalDeliverablePage{Deliverable: d, Project: p}
	rows, err := s.db.R.QueryContext(r.Context(), `SELECT v.id, v.deliverable_id, v.kind, v.url, v.note, v.state, v.created_at, v.number, v.shared_at, v.file_id, f.name, f.size
		FROM deliverable_versions v LEFT JOIN files f ON f.id = v.file_id WHERE v.deliverable_id = ? AND v.state != 'draft' ORDER BY v.number DESC`, d.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for rows.Next() {
		var v VersionRow
		if err := rows.Scan(&v.ID, &v.DeliverableID, &v.Kind, &v.URL, &v.Note, &v.State, &v.CreatedAt, &v.Number, &v.SharedAt, &v.FileID, &v.FileName, &v.FileSize); err != nil {
			rows.Close()
			s.fail(w, r, err)
			return
		}
		pg.Versions = append(pg.Versions, v)
	}
	rows.Close()
	if len(pg.Versions) == 0 {
		s.fail(w, r, errNotFound) // nothing shared yet: the deliverable does not exist for the client
		return
	}
	for i := range pg.Versions {
		if pg.Versions[i].Decisions, err = s.loadDecisions(r, pg.Versions[i].ID); err != nil {
			s.fail(w, r, err)
			return
		}
		if pg.Versions[i].Comments, err = s.loadComments(r, "version", pg.Versions[i].ID, false); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.render(w, r, http.StatusOK, "portal_deliverable", view{Title: d.Title, Data: pg})
}

// clientVersion loads a version the client may act on: non-draft, in their organization.
func (s *Server) clientVersion(r *http.Request, id string, writable bool) (VersionRow, Project, error) {
	v, p, err := s.loadVersion(r, id, writable)
	if err == nil && v.State == string(domain.Draft) {
		return v, p, errNotFound
	}
	return v, p, err
}

func (s *Server) portalComment(w http.ResponseWriter, r *http.Request) {
	v, p, err := s.clientVersion(r, r.PathValue("id"), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		id, err := s.addComment(tx, r, p, "version", v.ID, "client", f["body"])
		if err != nil {
			return err
		}
		staff, err := staffOf(tx, p)
		if err != nil {
			return err
		}
		return s.notify(tx, r, staff, "comment.client", id, fmt.Sprintf("%s commented on %s v%d", s.user(r).Name, p.Name, v.Number), "/deliverables/"+v.DeliverableID)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/portal/deliverables/"+v.DeliverableID, http.StatusSeeOther)
}

func (s *Server) portalDecide(w http.ResponseWriter, r *http.Request) {
	v, p, err := s.clientVersion(r, r.PathValue("id"), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	var to domain.VersionState
	switch f["decision"] {
	case "approve":
		to = domain.Approved
	case "revision":
		to = domain.RevisionRequested
	default:
		s.fail(w, r, invalid("Choose approve or request revision."))
		return
	}
	if to == domain.RevisionRequested && !within(f["note"], 1, 10000) {
		s.fail(w, r, invalid("Say what should change when you request a revision."))
		return
	}
	if len(f["note"]) > 10000 {
		s.fail(w, r, invalid("Note is too long."))
		return
	}
	if err := domain.Transition(domain.VersionState(v.State), to, s.role(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	u := s.user(r)
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		res, err := tx.Exec(`UPDATE deliverable_versions SET state = ?, updated_at = ? WHERE id = ? AND state = 'shared'`, string(to), db.Now(), v.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: version changed meanwhile", domain.ErrTransition)
		}
		decID := db.NewID()
		if _, err := tx.Exec(`INSERT INTO decisions (id, version_id, type, by_user, note, created_at) VALUES (?, ?, ?, ?, ?, ?)`, decID, v.ID, string(to), u.ID, f["note"], db.Now()); err != nil {
			return err
		}
		if err := s.audit(r.Context(), tx, r, "decision."+string(to), "deliverable_version", v.ID, map[string]any{"project_id": p.ID, "deliverable_id": v.DeliverableID, "number": v.Number}); err != nil {
			return err
		}
		staff, err := staffOf(tx, p)
		if err != nil {
			return err
		}
		verb := "approved"
		if to == domain.RevisionRequested {
			verb = "requested a revision of"
		}
		return s.notify(tx, r, staff, "decision", decID, fmt.Sprintf("%s %s %s v%d", u.Name, verb, p.Name, v.Number), "/deliverables/"+v.DeliverableID)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/portal/deliverables/"+v.DeliverableID, http.StatusSeeOther)
}

func (s *Server) portalDownload(w http.ResponseWriter, r *http.Request) {
	v, p, err := s.clientVersion(r, r.PathValue("id"), false)
	if err != nil || !v.FileID.Valid {
		s.fail(w, r, errNotFound)
		return
	}
	s.serveFile(w, r, v.FileID.String, map[string]any{"project_id": p.ID, "version_id": v.ID})
}

func (s *Server) portalInvoiceDocument(w http.ResponseWriter, r *http.Request) {
	p, err := s.projectOf(r, "invoices", r.PathValue("id"), false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var fileID sql.NullString
	err = s.db.R.QueryRowContext(r.Context(), `SELECT file_id FROM invoices WHERE id = ? AND visibility = 'client' AND status != 'draft'`, r.PathValue("id")).Scan(&fileID)
	if err != nil || !fileID.Valid {
		s.fail(w, r, errNotFound)
		return
	}
	s.serveFile(w, r, fileID.String, map[string]any{"project_id": p.ID, "invoice_id": r.PathValue("id")})
}

// ---- intake ----

type portalIntakePage struct {
	Project  Project
	Intake   *Intake
	Fields   []intakeField
	Comments []Comment
	Editable bool
}

func (s *Server) portalIntake(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pg := portalIntakePage{Project: p, Fields: intakeFields, Editable: !p.Closed()}
	if pg.Intake, err = s.loadIntake(r, p.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	if pg.Intake == nil {
		pg.Intake = &Intake{Answers: map[string]string{}}
	}
	if pg.Comments, err = s.loadComments(r, "intake", p.ID, false); err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "portal_intake", view{Title: "Project brief", Data: pg})
}

// portalIntakeSave keeps one draft row; submit freezes it as the next version (contract §2).
func (s *Server) portalIntakeSave(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadWritableProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	answers := map[string]string{}
	for _, fld := range intakeFields {
		if len(f[fld.Key]) > 10000 {
			s.fail(w, r, invalid("An answer is too long."))
			return
		}
		answers[fld.Key] = f[fld.Key]
	}
	submit := f["action"] == "submit"
	if submit && answers["goals"] == "" {
		s.fail(w, r, invalid("Tell us what the project should achieve before submitting."))
		return
	}
	raw, _ := json.Marshal(answers)
	u, now := s.user(r), db.Now()
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		var draftID sql.NullString
		var maxVersion int
		if err := tx.QueryRow(`SELECT (SELECT id FROM intake_responses WHERE project_id = ? AND status = 'draft'), COALESCE(max(version), 0) FROM intake_responses WHERE project_id = ?`, p.ID, p.ID).Scan(&draftID, &maxVersion); err != nil {
			return err
		}
		status := "draft"
		var subBy, subAt any
		if submit {
			status, subBy, subAt = "submitted", u.ID, now
		}
		if draftID.Valid {
			if _, err := tx.Exec(`UPDATE intake_responses SET answers = ?, status = ?, submitted_by = ?, submitted_at = ?, updated_at = ? WHERE id = ?`, string(raw), status, subBy, subAt, now, draftID.String); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(`INSERT INTO intake_responses (id, project_id, version, status, answers, submitted_by, submitted_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				db.NewID(), p.ID, maxVersion+1, status, string(raw), subBy, subAt, now, now); err != nil {
				return err
			}
		}
		if !submit {
			return nil
		}
		if err := s.audit(r.Context(), tx, r, "intake.submitted", "project", p.ID, map[string]any{"project_id": p.ID}); err != nil {
			return err
		}
		staff, err := staffOf(tx, p)
		if err != nil {
			return err
		}
		return s.notify(tx, r, staff, "intake.submitted", fmt.Sprintf("%s:%d", p.ID, maxVersion+1), fmt.Sprintf("%s submitted the brief for %s", u.Name, p.Name), "/projects/"+p.ID)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/portal/projects/"+p.ID+"/intake", http.StatusSeeOther)
}

func (s *Server) portalIntakeComment(w http.ResponseWriter, r *http.Request) {
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
		id, err := s.addComment(tx, r, p, "intake", p.ID, "client", f["body"])
		if err != nil {
			return err
		}
		staff, err := staffOf(tx, p)
		if err != nil {
			return err
		}
		return s.notify(tx, r, staff, "comment.client", id, fmt.Sprintf("%s replied on the brief for %s", s.user(r).Name, p.Name), "/projects/"+p.ID)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/portal/projects/"+p.ID+"/intake", http.StatusSeeOther)
}

// ---- sign-off (contract §5) ----

func (s *Server) portalSignoff(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadWritableProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	h, err := s.loadHandoff(r, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !h.Ready {
		s.fail(w, r, fmt.Errorf("%w: sign-off needs every required deliverable approved or waived", domain.ErrTransition))
		return
	}
	snapshot, _ := json.Marshal(map[string]any{"deliverables": h.Rows})
	u, id := s.user(r), db.NewID()
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO signoffs (id, project_id, by_user, snapshot, ip, created_at) VALUES (?, ?, ?, ?, ?, ?)`, id, p.ID, u.ID, string(snapshot), s.clientIP(r), db.Now()); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE projects SET status = 'closed', closed_at = ?, closed_by = ?, updated_at = ? WHERE id = ? AND status = 'active'`, db.Now(), u.ID, db.Now(), p.ID); err != nil {
			return err
		}
		if err := s.audit(r.Context(), tx, r, "signoff.recorded", "project", p.ID, map[string]any{"project_id": p.ID, "signoff_id": id}); err != nil {
			return err
		}
		staff, err := staffOf(tx, p)
		if err != nil {
			return err
		}
		return s.notify(tx, r, staff, "signoff", id, fmt.Sprintf("%s signed off %s", u.Name, p.Name), "/projects/"+p.ID)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/portal/projects/"+p.ID, http.StatusSeeOther)
}
