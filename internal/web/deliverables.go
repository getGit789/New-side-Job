package web

import (
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"briefrelay/internal/db"
	"briefrelay/internal/domain"
	"briefrelay/internal/storage"
)

type Deliverable struct {
	ID, ProjectID, Title, Description, CreatedAt string
	Required                                     bool
}

type VersionRow struct {
	ID, DeliverableID, Kind, URL, Note, State, ReopenReason, CreatedAt, CreatedBy string
	Number                                                                        int
	SharedAt, FileID, FileName                                                    sql.NullString
	FileSize                                                                      sql.NullInt64
	Decisions                                                                     []Decision
}

type Decision struct {
	Type, By, Note, CreatedAt string
}

func (s *Server) deliverableRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /projects/{id}/deliverables", s.requireStaff(s.deliverableCreate))
	mux.HandleFunc("GET /deliverables/{id}", s.requireStaff(s.deliverableShow))
	mux.HandleFunc("POST /deliverables/{id}", s.requireStaff(s.deliverableEdit))
	mux.HandleFunc("POST /deliverables/{id}/versions", s.requireStaff(s.versionCreate))
	mux.HandleFunc("POST /versions/{id}/share", s.requireStaff(s.versionShare))
	mux.HandleFunc("POST /versions/{id}/withdraw", s.requireStaff(s.versionWithdraw))
	mux.HandleFunc("POST /versions/{id}/delete", s.requireStaff(s.versionDelete))
	mux.HandleFunc("GET /versions/{id}/download", s.requireStaff(s.versionDownload))
}

func (s *Server) loadDeliverable(r *http.Request, id string, writable bool) (Deliverable, Project, error) {
	p, err := s.projectOf(r, "deliverables", id, writable)
	if err != nil {
		return Deliverable{}, p, err
	}
	var d Deliverable
	err = s.db.R.QueryRowContext(r.Context(), `SELECT id, project_id, title, description, required, created_at FROM deliverables WHERE id = ?`, id).
		Scan(&d.ID, &d.ProjectID, &d.Title, &d.Description, &d.Required, &d.CreatedAt)
	return d, p, err
}

func (s *Server) loadVersion(r *http.Request, id string, writable bool) (VersionRow, Project, error) {
	var v VersionRow
	err := s.db.R.QueryRowContext(r.Context(), `SELECT v.id, v.deliverable_id, v.kind, v.url, v.note, v.state, v.reopen_reason, v.created_at, v.number, v.shared_at, v.file_id, f.name, f.size
		FROM deliverable_versions v LEFT JOIN files f ON f.id = v.file_id WHERE v.id = ?`, id).
		Scan(&v.ID, &v.DeliverableID, &v.Kind, &v.URL, &v.Note, &v.State, &v.ReopenReason, &v.CreatedAt, &v.Number, &v.SharedAt, &v.FileID, &v.FileName, &v.FileSize)
	if errors.Is(err, sql.ErrNoRows) {
		return v, Project{}, errNotFound
	}
	if err != nil {
		return v, Project{}, err
	}
	p, err := s.projectOf(r, "deliverables", v.DeliverableID, writable)
	return v, p, err
}

func validDeliverable(f map[string]string) error {
	if !within(f["title"], 1, 200) || len(f["description"]) > 10000 {
		return invalid("Deliverable title must be 1–200 characters.")
	}
	return nil
}

func (s *Server) deliverableCreate(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadWritableProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	if err := validDeliverable(f); err != nil {
		s.fail(w, r, err)
		return
	}
	id, now := db.NewID(), db.Now()
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO deliverables (id, project_id, title, description, required, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, p.ID, f["title"], f["description"], f["required"] == "1", s.user(r).ID, now, now)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/deliverables/"+id, http.StatusSeeOther)
}

type deliverablePage struct {
	Deliverable Deliverable
	Project     Project
	Versions    []VersionRow
	Latest      string // state of the newest version
	NeedsReason bool
	CanAdd      bool
}

func (s *Server) deliverableShow(w http.ResponseWriter, r *http.Request) {
	d, p, err := s.loadDeliverable(r, r.PathValue("id"), false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pg := deliverablePage{Deliverable: d, Project: p}
	rows, err := s.db.R.QueryContext(r.Context(), `SELECT v.id, v.deliverable_id, v.kind, v.url, v.note, v.state, v.reopen_reason, v.created_at, v.number, v.shared_at, v.file_id, f.name, f.size, COALESCE(u.name, '')
		FROM deliverable_versions v LEFT JOIN files f ON f.id = v.file_id LEFT JOIN users u ON u.id = v.created_by WHERE v.deliverable_id = ? ORDER BY v.number DESC`, d.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for rows.Next() {
		var v VersionRow
		if err := rows.Scan(&v.ID, &v.DeliverableID, &v.Kind, &v.URL, &v.Note, &v.State, &v.ReopenReason, &v.CreatedAt, &v.Number, &v.SharedAt, &v.FileID, &v.FileName, &v.FileSize, &v.CreatedBy); err != nil {
			rows.Close()
			s.fail(w, r, err)
			return
		}
		pg.Versions = append(pg.Versions, v)
	}
	rows.Close()
	for i := range pg.Versions {
		drows, err := s.db.R.QueryContext(r.Context(), `SELECT d.type, COALESCE(u.name, ''), d.note, d.created_at FROM decisions d LEFT JOIN users u ON u.id = d.by_user WHERE d.version_id = ? ORDER BY d.created_at`, pg.Versions[i].ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for drows.Next() {
			var dec Decision
			if err := drows.Scan(&dec.Type, &dec.By, &dec.Note, &dec.CreatedAt); err == nil {
				pg.Versions[i].Decisions = append(pg.Versions[i].Decisions, dec)
			}
		}
		drows.Close()
	}
	if len(pg.Versions) > 0 {
		pg.Latest = pg.Versions[0].State
	}
	needs, err := domain.NewVersion(domain.VersionState(pg.Latest))
	pg.NeedsReason, pg.CanAdd = needs, err == nil && !p.Closed()
	s.render(w, r, http.StatusOK, "deliverable", view{Title: d.Title, Data: pg})
}

func (s *Server) deliverableEdit(w http.ResponseWriter, r *http.Request) {
	d, _, err := s.loadDeliverable(r, r.PathValue("id"), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	if err := validDeliverable(f); err != nil {
		s.fail(w, r, err)
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE deliverables SET title = ?, description = ?, required = ?, updated_at = ? WHERE id = ?`, f["title"], f["description"], f["required"] == "1", db.Now(), d.ID)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/deliverables/"+d.ID, http.StatusSeeOther)
}

// ---- uploads ----

// ponytail: fixed deny-list; a settings-driven allow-list is a v1.1 candidate.
var blockedExt = map[string]bool{".exe": true, ".bat": true, ".cmd": true, ".com": true, ".scr": true, ".msi": true, ".ps1": true, ".sh": true,
	".php": true, ".phtml": true, ".js": true, ".mjs": true, ".html": true, ".htm": true, ".svg": true, ".jar": true, ".vbs": true}

type upload struct {
	ID, Name string
	Info     storage.Info
}

// saveUpload stores the multipart file in field. It returns nil when no file was sent.
func (s *Server) saveUpload(w http.ResponseWriter, r *http.Request, field string) (*upload, error) {
	maxBytes := s.cfg.MaxUploadMB << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		if errors.Is(err, http.ErrNotMultipart) {
			return nil, nil
		}
		return nil, invalid(fmt.Sprintf("Upload failed or exceeds %d MB.", s.cfg.MaxUploadMB))
	}
	fh, hdr, err := r.FormFile(field)
	if errors.Is(err, http.ErrMissingFile) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	name := filepath.Base(strings.ReplaceAll(hdr.Filename, "\\", "/"))
	if name == "" || name == "." || len(name) > 255 {
		return nil, invalid("File name is not usable.")
	}
	if blockedExt[strings.ToLower(filepath.Ext(name))] {
		return nil, invalid("That file type is not allowed.")
	}
	info, err := s.store.Save(fh, maxBytes)
	if errors.Is(err, storage.ErrTooLarge) {
		return nil, invalid(fmt.Sprintf("File exceeds %d MB.", s.cfg.MaxUploadMB))
	}
	if err != nil {
		return nil, err
	}
	if mt := strings.ToLower(info.MediaType); strings.HasPrefix(mt, "text/html") || strings.Contains(mt, "svg") || strings.Contains(mt, "javascript") {
		s.store.Delete(info.Key)
		return nil, invalid("That file content is not allowed.")
	}
	return &upload{ID: db.NewID(), Name: name, Info: info}, nil
}

func insertFile(tx *sql.Tx, up *upload, userID string) error {
	_, err := tx.Exec(`INSERT INTO files (id, storage_key, name, size, sha256, media_type, uploaded_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		up.ID, up.Info.Key, up.Name, up.Info.Size, up.Info.SHA256, up.Info.MediaType, userID, db.Now())
	return err
}

// serveFile streams a stored file as an attachment. Callers must already have authorized the request.
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, fileID string, auditMeta map[string]any) {
	var key, name, mediaType string
	err := s.db.R.QueryRowContext(r.Context(), `SELECT storage_key, name, media_type FROM files WHERE id = ? AND deleted_at IS NULL`, fileID).Scan(&key, &name, &mediaType)
	if err != nil {
		s.fail(w, r, errNotFound)
		return
	}
	f, err := s.store.Open(key)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		return s.audit(r.Context(), tx, r, "file.downloaded", "file", fileID, auditMeta)
	}); err != nil {
		s.fail(w, r, err)
		return
	}
	// Never let a stored file render inline as HTML/script.
	ct := mediaType
	if mt, _, err := mime.ParseMediaType(ct); err != nil || strings.HasPrefix(mt, "text/html") || strings.Contains(mt, "script") || strings.Contains(mt, "svg") {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// ---- versions ----

func (s *Server) versionCreate(w http.ResponseWriter, r *http.Request) {
	d, _, err := s.loadDeliverable(r, r.PathValue("id"), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	up, err := s.saveUpload(w, r, "file")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f := map[string]string{}
	for k := range r.PostForm {
		f[k] = strings.TrimSpace(r.PostForm.Get(k))
	}
	cleanup := func() {
		if up != nil {
			s.store.Delete(up.Info.Key)
		}
	}
	kind := "link"
	if up != nil {
		kind = "file"
	} else if u, err := url.Parse(f["url"]); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(f["url"]) > 2048 {
		s.fail(w, r, invalid("Attach a file or enter an absolute http(s) link."))
		return
	}
	if len(f["note"]) > 10000 || len(f["reason"]) > 1000 {
		cleanup()
		s.fail(w, r, invalid("Note or reason is too long."))
		return
	}
	id, now, uid := db.NewID(), db.Now(), s.user(r).ID
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		var latest sql.NullString
		var number int
		if err := tx.QueryRow(`SELECT state, number FROM deliverable_versions WHERE deliverable_id = ? ORDER BY number DESC LIMIT 1`, d.ID).Scan(&latest, &number); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		needsReason, err := domain.NewVersion(domain.VersionState(latest.String))
		if err != nil {
			return err
		}
		if needsReason && !within(f["reason"], 3, 1000) {
			return invalid("Reopening an approved deliverable needs a reason (3–1000 characters).")
		}
		var fileID any
		if up != nil {
			if err := insertFile(tx, up, uid); err != nil {
				return err
			}
			fileID = up.ID
		}
		if _, err := tx.Exec(`INSERT INTO deliverable_versions (id, deliverable_id, number, kind, file_id, url, note, reopen_reason, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, d.ID, number+1, kind, fileID, f["url"], f["note"], f["reason"], uid, now, now); err != nil {
			return err
		}
		if needsReason { // contract §4: reopening is audited with its reason
			return s.audit(r.Context(), tx, r, "deliverable.reopened", "deliverable", d.ID, map[string]any{"project_id": d.ProjectID, "reason": f["reason"], "new_version": number + 1})
		}
		return nil
	})
	if err != nil {
		cleanup()
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/deliverables/"+d.ID, http.StatusSeeOther)
}

func (s *Server) transition(w http.ResponseWriter, r *http.Request, to domain.VersionState, action string) {
	v, p, err := s.loadVersion(r, r.PathValue("id"), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := domain.Transition(domain.VersionState(v.State), to, s.role(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		// Re-check inside the write transaction so two concurrent clicks cannot both succeed.
		res, err := tx.Exec(`UPDATE deliverable_versions SET state = ?, shared_at = CASE WHEN ? = 'shared' THEN ? ELSE shared_at END, shared_by = CASE WHEN ? = 'shared' THEN ? ELSE shared_by END, updated_at = ? WHERE id = ? AND state = ?`,
			string(to), string(to), db.Now(), string(to), s.user(r).ID, db.Now(), v.ID, v.State)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: version changed meanwhile", domain.ErrTransition)
		}
		// ponytail: client notification + email for "shared" arrives with the client portal in Phase 4.
		return s.audit(r.Context(), tx, r, action, "deliverable_version", v.ID, map[string]any{"project_id": p.ID, "deliverable_id": v.DeliverableID, "number": v.Number})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/deliverables/"+v.DeliverableID, http.StatusSeeOther)
}

func (s *Server) versionShare(w http.ResponseWriter, r *http.Request) {
	s.transition(w, r, domain.Shared, "version.shared")
}

func (s *Server) versionWithdraw(w http.ResponseWriter, r *http.Request) {
	s.transition(w, r, domain.Withdrawn, "version.withdrawn")
}

// versionDelete removes a draft. Anything ever shared is permanent (contract §2).
func (s *Server) versionDelete(w http.ResponseWriter, r *http.Request) {
	v, _, err := s.loadVersion(r, r.PathValue("id"), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		res, err := tx.Exec(`DELETE FROM deliverable_versions WHERE id = ? AND state = 'draft'`, v.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: only drafts can be deleted", domain.ErrTransition)
		}
		if v.FileID.Valid {
			_, err = tx.Exec(`UPDATE files SET deleted_at = ? WHERE id = ?`, db.Now(), v.FileID.String)
		}
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/deliverables/"+v.DeliverableID, http.StatusSeeOther)
}

func (s *Server) versionDownload(w http.ResponseWriter, r *http.Request) {
	v, p, err := s.loadVersion(r, r.PathValue("id"), false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !v.FileID.Valid {
		s.fail(w, r, errNotFound)
		return
	}
	s.serveFile(w, r, v.FileID.String, map[string]any{"project_id": p.ID, "version_id": v.ID})
}
