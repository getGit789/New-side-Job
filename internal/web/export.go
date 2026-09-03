package web

import (
	"archive/zip"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"briefrelay/internal/db"
)

// Export, import and permanent deletion (contract §3: owner only; plan §5.4 and Epic 6).
func (s *Server) exportRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /clients/export.csv", s.requireOwner(s.clientsExport))
	mux.HandleFunc("POST /clients/import", s.requireOwner(s.clientsImport))
	mux.HandleFunc("POST /clients/{id}/delete", s.requireOwner(s.clientDelete))
	mux.HandleFunc("GET /projects/{id}/export.zip", s.requireOwner(s.projectExport))
	mux.HandleFunc("POST /projects/{id}/delete", s.requireOwner(s.projectDelete))
}

var csvHeader = []string{"name", "email", "phone", "archived", "contact_name", "contact_email", "contact_title"}

// clientsExport writes one row per contact (orgs without contacts get one row with blank contact fields).
func (s *Server) clientsExport(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.R.QueryContext(r.Context(), `SELECT o.name, o.email, o.phone, o.archived_at IS NOT NULL, COALESCE(c.name, ''), COALESCE(c.email, ''), COALESCE(c.title, '')
		FROM client_orgs o LEFT JOIN client_contacts c ON c.client_org_id = o.id AND c.removed_at IS NULL
		WHERE o.workspace_id = ? ORDER BY o.name, c.name`, s.user(r).WorkspaceID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="clients.csv"`)
	cw := csv.NewWriter(w)
	cw.Write(csvHeader)
	for rows.Next() {
		var name, email, phone, cn, ce, ct string
		var archived bool
		if err := rows.Scan(&name, &email, &phone, &archived, &cn, &ce, &ct); err != nil {
			s.fail(w, r, err)
			return
		}
		a := ""
		if archived {
			a = "yes"
		}
		cw.Write([]string{csvSafe(name), csvSafe(email), csvSafe(phone), a, csvSafe(cn), csvSafe(ce), csvSafe(ct)})
	}
	cw.Flush()
	s.logErr("audit", s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		return s.audit(r.Context(), tx, r, "clients.exported", "workspace", s.user(r).WorkspaceID, nil)
	}))
}

// csvSafe stops spreadsheet formula injection: a leading =, +, -, @ is prefixed with a quote.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// clientsImport reads the same columns clientsExport writes. Existing orgs (same name) get the contact
// added; existing contacts (same email in that org) are left alone. Nothing is ever overwritten.
func (s *Server) clientsImport(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.parseForm(w, r); !ok {
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		s.fail(w, r, invalid("Choose a CSV file to import."))
		return
	}
	defer f.Close()
	cr := csv.NewReader(io.LimitReader(f, 4<<20))
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err != nil {
		s.fail(w, r, invalid("The file is empty or not CSV."))
		return
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimPrefix(strings.TrimSpace(h), "\ufeff"))] = i
	}
	if _, ok := col["name"]; !ok {
		s.fail(w, r, invalid("The first row must name the columns and include \"name\" (see the export for the format)."))
		return
	}
	get := func(rec []string, k string) string {
		if i, ok := col[k]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	u := s.user(r)
	var orgs, contacts int
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		for line := 2; ; line++ {
			rec, err := cr.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return invalid(fmt.Sprintf("Line %d: %v", line, err))
			}
			if line > 5001 {
				return invalid("Import at most 5,000 rows at a time.")
			}
			name := get(rec, "name")
			if !within(name, 1, 200) {
				return invalid(fmt.Sprintf("Line %d: name is required (max 200 characters).", line))
			}
			var oid string
			err = tx.QueryRow(`SELECT id FROM client_orgs WHERE workspace_id = ? AND lower(name) = lower(?) LIMIT 1`, u.WorkspaceID, name).Scan(&oid)
			if err == sql.ErrNoRows {
				oid, err = db.NewID(), nil
				if _, err = tx.Exec(`INSERT INTO client_orgs (id, workspace_id, name, email, phone, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
					oid, u.WorkspaceID, name, get(rec, "email"), get(rec, "phone"), u.ID, db.Now(), db.Now()); err != nil {
					return err
				}
				orgs++
			}
			if err != nil {
				return err
			}
			ce := strings.ToLower(get(rec, "contact_email"))
			if ce == "" {
				continue
			}
			if !strings.Contains(ce, "@") || len(ce) > 254 {
				return invalid(fmt.Sprintf("Line %d: contact_email is not valid.", line))
			}
			var n int
			if err := tx.QueryRow(`SELECT count(*) FROM client_contacts WHERE client_org_id = ? AND email = ?`, oid, ce).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				continue
			}
			cn := get(rec, "contact_name")
			if cn == "" {
				cn = ce
			}
			if _, err := tx.Exec(`INSERT INTO client_contacts (id, client_org_id, name, email, title, created_at) VALUES (?, ?, ?, ?, ?, ?)`, db.NewID(), oid, cn, ce, get(rec, "contact_title"), db.Now()); err != nil {
				return err
			}
			contacts++
		}
		return s.audit(r.Context(), tx, r, "clients.imported", "workspace", u.WorkspaceID, map[string]any{"orgs": orgs, "contacts": contacts})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/clients?imported=%d+clients,+%d+contacts", orgs, contacts), http.StatusSeeOther)
}

// projectExport is the durable hand-over record (AT-11): client-visible content, decisions, sign-off,
// audit trail and the files, as a zip. Internal comments, notes and invoices are never included.
func (s *Server) projectExport(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	rowsOf := func(q string, args ...any) ([]map[string]any, error) {
		rows, err := s.db.R.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		cols, _ := rows.Columns()
		var out []map[string]any
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return nil, err
			}
			m := map[string]any{}
			for i, c := range cols {
				if b, ok := vals[i].([]byte); ok {
					vals[i] = string(b)
				}
				m[c] = vals[i]
			}
			out = append(out, m)
		}
		return out, rows.Err()
	}
	doc := map[string]any{"format": "briefrelay-project-export/1", "exported_at": db.Now(), "project": map[string]any{
		"id": p.ID, "name": p.Name, "summary": p.Summary, "status": p.Status, "client": p.ClientName, "created_at": p.CreatedAt, "closed_at": p.ClosedAt.String}}
	type section struct {
		key, query string
	}
	for _, sec := range []section{
		{"milestones", `SELECT title, target_date, status, latest_update FROM milestones WHERE project_id = ? AND visibility = 'client' ORDER BY sort_order`},
		{"intake", `SELECT version, status, answers, submitted_at FROM intake_responses WHERE project_id = ? AND status = 'submitted' ORDER BY version`},
		{"deliverables", `SELECT id, title, description, required FROM deliverables WHERE project_id = ? ORDER BY sort_order, created_at`},
		{"versions", `SELECT v.id, v.deliverable_id, v.number, v.kind, v.url, v.note, v.state, v.shared_at, f.name AS file_name, f.sha256 AS file_sha256
			FROM deliverable_versions v JOIN deliverables d ON d.id = v.deliverable_id LEFT JOIN files f ON f.id = v.file_id WHERE d.project_id = ? AND v.state != 'draft' ORDER BY d.sort_order, v.number`},
		{"decisions", `SELECT x.version_id, x.type, COALESCE(u.name, '') AS by, x.note, x.created_at FROM decisions x JOIN deliverable_versions v ON v.id = x.version_id JOIN deliverables d ON d.id = v.deliverable_id LEFT JOIN users u ON u.id = x.by_user WHERE d.project_id = ? ORDER BY x.created_at`},
		{"comments", `SELECT c.target_type, c.target_id, COALESCE(u.name, '') AS author, c.body, c.created_at FROM comments c LEFT JOIN users u ON u.id = c.author_id WHERE c.project_id = ? AND c.visibility = 'client' AND c.deleted_at IS NULL ORDER BY c.created_at`},
		{"waivers", `SELECT w.deliverable_id, COALESCE(u.name, '') AS by, w.reason, w.created_at FROM waivers w LEFT JOIN users u ON u.id = w.by_user JOIN deliverables d ON d.id = w.deliverable_id WHERE d.project_id = ? ORDER BY w.created_at`},
		{"signoffs", `SELECT COALESCE(u.name, '') AS by, s.snapshot, s.ip, s.created_at FROM signoffs s LEFT JOIN users u ON u.id = s.by_user WHERE s.project_id = ? ORDER BY s.created_at`},
		{"invoices", `SELECT i.number, i.amount_cents, i.currency, i.due_date, i.status, i.payment_url, f.name AS file_name FROM invoices i LEFT JOIN files f ON f.id = i.file_id WHERE i.project_id = ? AND i.visibility = 'client' ORDER BY i.created_at`},
		{"audit", `SELECT a.action, a.target_type, a.target_id, COALESCE(u.name, '') AS actor, a.created_at FROM audit_events a LEFT JOIN users u ON u.id = a.actor_id WHERE json_extract(a.meta, '$.project_id') = ? ORDER BY a.id`},
	} {
		rows, err := rowsOf(sec.query, p.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if rows == nil {
			rows = []map[string]any{}
		}
		doc[sec.key] = rows
	}
	files, err := rowsOf(`SELECT f.storage_key, f.name, 'versions/' || d.title || '-v' || v.number AS prefix FROM deliverable_versions v JOIN deliverables d ON d.id = v.deliverable_id JOIN files f ON f.id = v.file_id WHERE d.project_id = ? AND v.state != 'draft'
		UNION ALL SELECT f.storage_key, f.name, 'invoices/' || i.number FROM invoices i JOIN files f ON f.id = i.file_id WHERE i.project_id = ? AND i.visibility = 'client'`, p.ID, p.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		return s.audit(ctx, tx, r, "project.exported", "project", p.ID, map[string]any{"project_id": p.ID})
	}); err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="project-%s.zip"`, p.ID))
	zw := zip.NewWriter(w)
	if f, err := zw.Create("project.json"); err == nil {
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		enc.Encode(doc)
	}
	for _, f := range files {
		src, err := s.store.Open(f["storage_key"].(string))
		if err != nil {
			continue // a missing blob must not break the record itself
		}
		name := path.Clean(fmt.Sprintf("files/%s-%s", f["prefix"], path.Base(f["name"].(string))))
		if dst, err := zw.Create(name); err == nil {
			io.Copy(dst, src)
		}
		src.Close()
	}
	zw.Close()
}

// deleteProjects removes projects and everything under them. Files are soft-deleted so the hourly job purges the blobs.
func deleteProjects(tx *sql.Tx, where string, args ...any) error {
	for _, q := range []string{
		`UPDATE files SET deleted_at = ? WHERE id IN (SELECT v.file_id FROM deliverable_versions v JOIN deliverables d ON d.id = v.deliverable_id WHERE d.project_id IN (` + where + `))
			OR id IN (SELECT file_id FROM invoices WHERE project_id IN (` + where + `))`,
		`DELETE FROM decisions WHERE version_id IN (SELECT v.id FROM deliverable_versions v JOIN deliverables d ON d.id = v.deliverable_id WHERE d.project_id IN (` + where + `))`,
		`DELETE FROM projects WHERE id IN (` + where + `)`,
	} {
		a := args
		if strings.HasPrefix(q, "UPDATE") {
			a = append([]any{db.Now()}, append(append([]any{}, args...), args...)...)
		}
		if _, err := tx.Exec(q, a...); err != nil {
			return err
		}
	}
	return nil
}

// projectDelete is allowed for closed projects only (contract §3). Export first; this is final.
func (s *Server) projectDelete(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !p.Closed() {
		s.fail(w, r, invalid("Close the project before deleting it. Export it first if you need the record."))
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		if err := deleteProjects(tx, "?", p.ID); err != nil {
			return err
		}
		return s.audit(r.Context(), tx, r, "project.deleted", "project", p.ID, map[string]any{"name": p.Name, "client": p.ClientName})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

// clientDelete permanently removes an archived organization, its projects, contacts and their portal accounts.
func (s *Server) clientDelete(w http.ResponseWriter, r *http.Request) {
	c, err := s.loadClient(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !c.ArchivedAt.Valid {
		s.fail(w, r, invalid("Archive the client first. Deletion is permanent."))
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		if err := deleteProjects(tx, `SELECT id FROM projects WHERE client_org_id = ?`, c.ID); err != nil {
			return err
		}
		// Portal accounts belong to this org only (one contact per user), so they go too.
		rows, err := tx.Query(`SELECT user_id FROM client_contacts WHERE client_org_id = ? AND user_id IS NOT NULL`, c.ID)
		if err != nil {
			return err
		}
		var users []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				users = append(users, id)
			}
		}
		rows.Close()
		for _, q := range []string{
			`DELETE FROM invitations WHERE contact_id IN (SELECT id FROM client_contacts WHERE client_org_id = ?)`,
			`DELETE FROM client_orgs WHERE id = ?`, // cascades contacts
		} {
			if _, err := tx.Exec(q, c.ID); err != nil {
				return err
			}
		}
		for _, uid := range users {
			if _, err := tx.Exec(`DELETE FROM users WHERE id = ?`, uid); err != nil {
				return err
			}
		}
		return s.audit(r.Context(), tx, r, "client_org.deleted", "client_org", c.ID, map[string]any{"name": c.Name})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}
