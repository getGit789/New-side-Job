package web

import (
	"context"
	"database/sql"
	"errors"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"briefrelay/internal/db"
)

// Workspace settings (plan §5.4). One workspace per install, so the values are cached in memory and
// refreshed by the only writers: setup and the settings page. Templates read them through view.Workspace
// and the date function.

type prefs struct {
	ID, Name, Contact, TimeZone, DateFormat, Currency, AllowedExt string
	LogoFileID                                                    sql.NullString
	loc                                                           *time.Location
	ext                                                           map[string]bool // lower-case ".pdf"; nil = no allow-list
}

func (p *prefs) HasLogo() bool { return p.LogoFileID.Valid }

var dateFormats = []struct{ Layout, Label string }{
	{"2006-01-02", "2026-12-31 (ISO)"}, {"02.01.2006", "31.12.2026"}, {"02/01/2006", "31/12/2026"}, {"01/02/2006", "12/31/2026"}, {"2 Jan 2006", "31 Dec 2026"},
}

func validDateFormat(layout string) bool {
	for _, f := range dateFormats {
		if f.Layout == layout {
			return true
		}
	}
	return false
}

// parseExt turns "pdf, .PNG,zip" into {".pdf",".png",".zip"}; empty input means no allow-list.
func parseExt(s string) map[string]bool {
	out := map[string]bool{}
	for _, e := range strings.Split(s, ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			out["."+strings.TrimPrefix(e, ".")] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Server) loadPrefs(ctx context.Context) error {
	p := &prefs{Name: "BriefRelay", TimeZone: "UTC", DateFormat: "2006-01-02", Currency: "USD", loc: time.UTC}
	err := s.db.R.QueryRowContext(ctx, `SELECT id, name, contact, time_zone, date_format, currency, allowed_ext, logo_file_id FROM workspaces ORDER BY created_at LIMIT 1`).
		Scan(&p.ID, &p.Name, &p.Contact, &p.TimeZone, &p.DateFormat, &p.Currency, &p.AllowedExt, &p.LogoFileID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if loc, err := time.LoadLocation(p.TimeZone); err == nil {
		p.loc = loc
	}
	p.ext = parseExt(p.AllowedExt)
	s.prefs.Store(p)
	return nil
}

func (s *Server) settingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /settings", s.requireOwner(s.settingsShow))
	mux.HandleFunc("POST /settings", s.requireOwner(s.settingsSave))
	mux.HandleFunc("GET /logo", s.logo)
	mux.HandleFunc("GET /search", s.requireStaff(s.search))
}

type settingsPage struct {
	Prefs   *prefs
	Formats []struct{ Layout, Label string }
	Blocked string
}

func (s *Server) settingsShow(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "settings", view{Title: "Settings", Data: settingsPage{Prefs: s.prefs.Load(), Formats: dateFormats, Blocked: blockedList()}})
}

func blockedList() string {
	var out []string
	for e := range blockedExt {
		out = append(out, strings.TrimPrefix(e, "."))
	}
	return strings.Join(sortStrings(out), ", ")
}

func (s *Server) settingsSave(w http.ResponseWriter, r *http.Request) {
	if s.cfg.IsDemo() {
		s.fail(w, r, errForbidden)
		return
	}
	up, err := s.saveUpload(w, r, "logo")
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
	cur := strings.ToUpper(f["currency"])
	var vErr error
	switch {
	case !within(f["name"], 1, 200):
		vErr = invalid("Workspace name must be 1–200 characters.")
	case len(f["contact"]) > 10000 || len(f["allowed_ext"]) > 1000:
		vErr = invalid("Contact details or extension list is too long.")
	case len(cur) != 3:
		vErr = invalid("Default currency must be a 3-letter code such as USD.")
	case !validDateFormat(f["date_format"]):
		vErr = invalid("Choose one of the listed date formats.")
	case up != nil && !strings.HasPrefix(up.Info.MediaType, "image/"):
		vErr = invalid("The logo must be a PNG, JPEG, GIF or WebP image.")
	}
	if vErr == nil {
		if _, err := time.LoadLocation(f["time_zone"]); err != nil || len(f["time_zone"]) > 64 {
			vErr = invalid("Time zone must be an IANA name such as Europe/Berlin or America/New_York.")
		}
	}
	if vErr != nil {
		cleanup()
		s.fail(w, r, vErr)
		return
	}
	ws := s.user(r).WorkspaceID
	old := s.prefs.Load()
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		if up != nil {
			if err := insertFile(tx, up, s.user(r).ID); err != nil {
				return err
			}
			if old.LogoFileID.Valid {
				if _, err := tx.Exec(`UPDATE files SET deleted_at = ? WHERE id = ?`, db.Now(), old.LogoFileID.String); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(`UPDATE workspaces SET logo_file_id = ? WHERE id = ?`, up.ID, ws); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE workspaces SET name = ?, contact = ?, time_zone = ?, date_format = ?, currency = ?, allowed_ext = ?, updated_at = ? WHERE id = ?`,
			f["name"], f["contact"], f["time_zone"], f["date_format"], cur, f["allowed_ext"], db.Now(), ws); err != nil {
			return err
		}
		return s.audit(r.Context(), tx, r, "settings.changed", "workspace", ws, map[string]any{"time_zone": f["time_zone"], "date_format": f["date_format"], "currency": cur, "allowed_ext": f["allowed_ext"], "logo_changed": up != nil})
	})
	if err != nil {
		cleanup()
		s.fail(w, r, err)
		return
	}
	if err := s.loadPrefs(r.Context()); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

// logo serves the workspace logo inline. It is the one file reachable without a session: it is the
// owner's own branding, restricted to image types at upload, and never a client upload.
func (s *Server) logo(w http.ResponseWriter, r *http.Request) {
	p := s.prefs.Load()
	if !p.LogoFileID.Valid {
		s.errorPage(w, r, http.StatusNotFound, "No logo has been uploaded.")
		return
	}
	var key, mediaType string
	if err := s.db.R.QueryRowContext(r.Context(), `SELECT storage_key, media_type FROM files WHERE id = ? AND deleted_at IS NULL`, p.LogoFileID.String).Scan(&key, &mediaType); err != nil {
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
	if mt, _, err := mime.ParseMediaType(mediaType); err != nil || !strings.HasPrefix(mt, "image/") || strings.Contains(mt, "svg") {
		s.fail(w, r, errNotFound)
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Disposition", "inline; filename="+filepath.Base("logo"+extFor(mediaType)))
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

func extFor(mediaType string) string {
	if exts, _ := mime.ExtensionsByType(mediaType); len(exts) > 0 {
		return exts[0]
	}
	return ""
}

// ---- search (plan §5.4: clients, projects, deliverables; every list scoped and capped) ----

type searchPage struct {
	Query        string
	Clients      []Client
	Projects     []ProjectRow
	Deliverables []searchHit
}

type searchHit struct {
	ID, Title, ProjectID, ProjectName, ClientName, LatestState string
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	d := searchPage{Query: strings.TrimSpace(r.URL.Query().Get("q"))}
	if len(d.Query) > 200 {
		d.Query = d.Query[:200]
	}
	if d.Query == "" {
		s.render(w, r, http.StatusOK, "search", view{Title: "Search", Data: d})
		return
	}
	u := s.user(r)
	like := "%" + escapeLike(d.Query) + "%"
	cond, args := clientScope(&userLike{u.ID, u.Email, u.Name, u.Role, u.WorkspaceID})
	rows, err := s.db.R.QueryContext(r.Context(), `SELECT c.id, c.name, c.email, c.archived_at FROM client_orgs c WHERE `+cond+` AND c.name LIKE ? ESCAPE '\' ORDER BY c.name LIMIT ?`, append(args, like, pageSize)...)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for rows.Next() {
		var c Client
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.ArchivedAt); err == nil {
			d.Clients = append(d.Clients, c)
		}
	}
	rows.Close()
	if d.Projects, err = s.listProjects(r, d.Query, "", pageSize, 0); err != nil {
		s.fail(w, r, err)
		return
	}
	cond, args = projectScope(u)
	rows, err = s.db.R.QueryContext(r.Context(), `SELECT d.id, d.title, p.id, p.name, c.name, COALESCE((SELECT state FROM deliverable_versions WHERE deliverable_id = d.id ORDER BY number DESC LIMIT 1), '')
		FROM deliverables d JOIN projects p ON p.id = d.project_id JOIN client_orgs c ON c.id = p.client_org_id
		WHERE `+cond+` AND d.title LIKE ? ESCAPE '\' ORDER BY d.updated_at DESC LIMIT ?`, append(args, like, pageSize)...)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var h searchHit
		if err := rows.Scan(&h.ID, &h.Title, &h.ProjectID, &h.ProjectName, &h.ClientName, &h.LatestState); err == nil {
			d.Deliverables = append(d.Deliverables, h)
		}
	}
	s.render(w, r, http.StatusOK, "search", view{Title: "Search", Data: d})
}
