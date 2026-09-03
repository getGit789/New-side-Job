package web

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"briefrelay/internal/config"
	"briefrelay/internal/db"
	"briefrelay/internal/jobs"
	"briefrelay/internal/mail"
	"briefrelay/internal/storage"
)

type env struct {
	t  *testing.T
	ts *httptest.Server
	db *db.DB
}

func newEnv(t *testing.T) *env {
	t.Setenv("BRIEFRELAY_ENV", "development")
	t.Setenv("BRIEFRELAY_DATA_DIR", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if _, err := d.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, _ := storage.NewLocal(cfg.FilesDir)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := jobs.New(d, log)
	s, err := New(cfg, d, q, mail.New(cfg.SMTP, log), st, log)
	if err != nil {
		t.Fatal(err)
	}
	s.limiter = newIPLimiter(1000, 1000) // the journey test fires many requests from one IP
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return &env{t: t, ts: ts, db: d}
}

// browser is one logged-in actor with its own cookie jar.
type browser struct {
	e *env
	c *http.Client
}

type jar struct{ cookies []*http.Cookie }

func (j *jar) SetCookies(_ *url.URL, cs []*http.Cookie) {
	for _, c := range cs {
		if c.MaxAge < 0 {
			j.cookies = nil
		} else {
			j.cookies = []*http.Cookie{c}
		}
	}
}
func (j *jar) Cookies(*url.URL) []*http.Cookie { return j.cookies }

func (e *env) browser() *browser {
	return &browser{e: e, c: &http.Client{Jar: &jar{}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (b *browser) get(path string) (*http.Response, string) {
	r, err := b.c.Get(b.e.ts.URL + path)
	if err != nil {
		b.e.t.Fatal(err)
	}
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	return r, string(body)
}

func (b *browser) post(path string, form url.Values) *http.Response {
	r, err := b.c.PostForm(b.e.ts.URL+path, form)
	if err != nil {
		b.e.t.Fatal(err)
	}
	r.Body.Close()
	return r
}

func (b *browser) upload(path string, fields map[string]string, fileField, fileName, content string) *http.Response {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		w.WriteField(k, v)
	}
	if fileName != "" {
		fw, _ := w.CreateFormFile(fileField, fileName)
		fw.Write([]byte(content))
	}
	w.Close()
	req, _ := http.NewRequest("POST", b.e.ts.URL+path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	r, err := b.c.Do(req)
	if err != nil {
		b.e.t.Fatal(err)
	}
	r.Body.Close()
	return r
}

// want fails the test unless the response has the status; it returns the Location header.
func (b *browser) want(r *http.Response, status int, what string) string {
	b.e.t.Helper()
	if r.StatusCode != status {
		b.e.t.Fatalf("%s: got %d, want %d", what, r.StatusCode, status)
	}
	return r.Header.Get("Location")
}

func lastSegment(s string) string { return s[strings.LastIndex(s, "/")+1:] }

func (e *env) scalar(query string, args ...any) string {
	var out string
	if err := e.db.R.QueryRow(query, args...).Scan(&out); err != nil {
		e.t.Fatalf("query %q: %v", query, err)
	}
	return out
}

func (e *env) count(query string, args ...any) int {
	var n int
	if err := e.db.R.QueryRow(query, args...).Scan(&n); err != nil {
		e.t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func setupOwner(e *env) *browser {
	o := e.browser()
	o.want(o.post("/setup", url.Values{"workspace": {"Acme"}, "name": {"Ann"}, "email": {"ann@acme.test"}, "password": {"a-long-enough-password"}}), 303, "setup")
	return o
}

func TestInstallLoginLogoutJourney(t *testing.T) {
	e := newEnv(t)
	b := e.browser()
	if r, _ := b.get("/"); r.StatusCode != 303 || r.Header.Get("Location") != "/setup" {
		t.Fatalf("fresh install must redirect to /setup, got %d %s", r.StatusCode, r.Header.Get("Location"))
	}
	if r, _ := b.get("/healthz"); r.StatusCode != 503 { // worker is not running here, so jobs check fails
		t.Fatalf("healthz before worker: %d", r.StatusCode)
	}
	b.want(b.post("/setup", url.Values{"workspace": {"Acme"}, "name": {"Ann"}, "email": {"ann@acme.test"}, "password": {"short"}}), 422, "weak password")
	b.want(b.post("/setup", url.Values{"workspace": {"Acme"}, "name": {"Ann"}, "email": {"Ann@Acme.test"}, "password": {"a-long-enough-password"}}), 303, "setup")
	if r, _ := b.get("/setup"); r.StatusCode != 404 {
		t.Fatalf("setup must lock after install: %d", r.StatusCode)
	}
	r, body := b.get("/")
	if r.StatusCode != 200 || !strings.Contains(body, "Acme") || r.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("home after setup: %d", r.StatusCode)
	}
	b.want(b.post("/logout", nil), 303, "logout")
	if r, _ := b.get("/"); r.StatusCode != 303 || r.Header.Get("Location") != "/login" {
		t.Fatalf("after logout must redirect to /login: %d", r.StatusCode)
	}
	b.want(b.post("/login", url.Values{"email": {"ann@acme.test"}, "password": {"wrong-password-here"}}), 401, "wrong password")
	b.want(b.post("/login", url.Values{"email": {"ann@acme.test"}, "password": {"a-long-enough-password"}}), 303, "login")
	if r, _ := b.get("/"); r.StatusCode != 200 {
		t.Fatalf("home after login: %d", r.StatusCode)
	}
	req, _ := http.NewRequest("POST", e.ts.URL+"/logout", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	if r, _ := b.c.Do(req); r.StatusCode != 403 {
		t.Fatalf("cross-site POST must be blocked: %d", r.StatusCode)
	}
}

func TestOwnerJourneyAndStaffIsolation(t *testing.T) {
	e := newEnv(t)
	o := setupOwner(e)

	// Client and contact.
	cid := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Blue Bakery"}, "email": {"hi@blue.test"}}), 303, "create client"))
	o.want(o.post("/clients/"+cid+"/contacts", url.Values{"name": {"Bo"}, "email": {"bo@blue.test"}}), 303, "add contact")
	o.want(o.post("/clients/"+cid+"/archive", nil), 303, "archive with no projects")
	o.want(o.post("/clients/"+cid+"/unarchive", nil), 303, "unarchive")

	// Project, milestone, visibility audit.
	pid := lastSegment(o.want(o.post("/projects", url.Values{"client_org_id": {cid}, "name": {"Website"}}), 303, "create project"))
	o.want(o.post("/clients/"+cid+"/archive", nil), 422, "archive with an active project must fail")
	o.want(o.post("/projects/"+pid+"/milestones", url.Values{"title": {"Kickoff"}, "visibility": {"internal"}}), 303, "milestone")
	mid := e.scalar(`SELECT id FROM milestones WHERE project_id = ?`, pid)
	o.want(o.post("/milestones/"+mid, url.Values{"title": {"Kickoff"}, "status": {"done"}, "visibility": {"client"}}), 303, "milestone edit")
	if e.count(`SELECT count(*) FROM audit_events WHERE action = 'milestone.visibility_changed'`) != 1 {
		t.Fatal("visibility change must be audited")
	}
	o.want(o.post("/milestones/"+mid+"/delete", nil), 422, "done milestone cannot be deleted")

	// Deliverable: upload, download, share, no second draft while shared.
	did := lastSegment(o.want(o.post("/projects/"+pid+"/deliverables", url.Values{"title": {"Logo"}, "required": {"1"}}), 303, "deliverable"))
	o.want(o.upload("/deliverables/"+did+"/versions", map[string]string{"note": "first cut"}, "file", "logo.pdf", "%PDF-1.4 hello"), 303, "upload v1")
	o.want(o.upload("/deliverables/"+did+"/versions", nil, "file", "evil.html", "<script>"), 422, "html upload must be rejected")
	v1 := e.scalar(`SELECT id FROM deliverable_versions WHERE deliverable_id = ? AND number = 1`, did)
	r, body := o.get("/versions/" + v1 + "/download")
	if r.StatusCode != 200 || body != "%PDF-1.4 hello" || !strings.HasPrefix(r.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("download: %d %q %q", r.StatusCode, body, r.Header.Get("Content-Disposition"))
	}
	o.want(o.upload("/deliverables/"+did+"/versions", map[string]string{"url": "https://example.com/v2"}, "", "", ""), 409, "second draft must be rejected")
	o.want(o.post("/versions/"+v1+"/share", nil), 303, "share v1")
	o.want(o.post("/versions/"+v1+"/share", nil), 409, "share twice must fail")
	o.want(o.post("/versions/"+v1+"/delete", nil), 409, "shared version cannot be deleted")
	o.want(o.upload("/deliverables/"+did+"/versions", map[string]string{"url": "https://example.com/v2"}, "", "", ""), 409, "no new version while awaiting decision")

	// Client approves (the client flow itself is Phase 4; we simulate the decision rows).
	e.db.W.Exec(`UPDATE deliverable_versions SET state = 'approved' WHERE id = ?`, v1)
	o.want(o.upload("/deliverables/"+did+"/versions", map[string]string{"url": "https://example.com/v2"}, "", "", ""), 422, "reopen without reason must fail")
	o.want(o.upload("/deliverables/"+did+"/versions", map[string]string{"url": "https://example.com/v2", "reason": "client changed the brief"}, "", "", ""), 303, "reopen with reason")
	if e.scalar(`SELECT state FROM deliverable_versions WHERE id = ?`, v1) != "approved" {
		t.Fatal("approved version must stay approved after reopen")
	}
	if e.count(`SELECT count(*) FROM audit_events WHERE action = 'deliverable.reopened'`) != 1 {
		t.Fatal("reopen must be audited")
	}
	if e.count(`SELECT count(*) FROM deliverable_versions WHERE deliverable_id = ? AND number = 2 AND state = 'draft'`, did) != 1 {
		t.Fatal("v2 draft missing")
	}

	// Invoice state machine.
	o.want(o.upload("/projects/"+pid+"/invoices", map[string]string{"number": "INV-1", "amount": "1250.50", "currency": "usd", "visibility": "client"}, "", "", ""), 303, "invoice")
	iid := e.scalar(`SELECT id FROM invoices WHERE project_id = ?`, pid)
	if e.scalar(`SELECT amount_cents || ' ' || currency FROM invoices WHERE id = ?`, iid) != "125050 USD" {
		t.Fatal("money parsing broke")
	}
	o.want(o.post("/invoices/"+iid+"/status", url.Values{"to": {"paid"}}), 409, "draft→paid must fail")
	o.want(o.post("/invoices/"+iid+"/status", url.Values{"to": {"sent"}}), 303, "draft→sent")
	o.want(o.post("/invoices/"+iid+"/status", url.Values{"to": {"paid"}}), 303, "sent→paid")

	// Staff invitation via the queued email.
	o.want(o.post("/team/invite", url.Values{"email": {"sam@acme.test"}}), 303, "invite")
	payload := e.scalar(`SELECT payload FROM jobs WHERE kind = 'mail.send'`)
	token := regexp.MustCompile(`/invite/([A-Za-z0-9_-]+)`).FindStringSubmatch(payload)[1]
	s := e.browser()
	if r, _ := s.get("/invite/" + token); r.StatusCode != 200 {
		t.Fatalf("invite page: %d", r.StatusCode)
	}
	s.want(s.post("/invite/"+token, url.Values{"name": {"Sam"}, "password": {"another-long-password"}}), 303, "accept invite")
	if r, _ := s.get("/invite/" + token); r.StatusCode != 410 {
		t.Fatalf("invite must be single use: %d", r.StatusCode)
	}
	sid := e.scalar(`SELECT id FROM users WHERE email = 'sam@acme.test'`)

	// Staff isolation: nothing until assigned, then only the assigned project.
	if r, _ := s.get("/projects/" + pid); r.StatusCode != 404 {
		t.Fatalf("unassigned staff must get 404, got %d", r.StatusCode)
	}
	if r, _ := s.get("/clients/" + cid); r.StatusCode != 404 {
		t.Fatalf("unassigned staff must not see the client, got %d", r.StatusCode)
	}
	if r, _ := s.get("/team"); r.StatusCode != 403 {
		t.Fatalf("staff must not open team, got %d", r.StatusCode)
	}
	if r, _ := s.get("/versions/" + v1 + "/download"); r.StatusCode != 404 {
		t.Fatalf("unassigned staff must not download, got %d", r.StatusCode)
	}
	s.want(s.post("/projects/"+pid+"/members", url.Values{"user_id": {sid}}), 403, "staff cannot assign members")
	o.want(o.post("/projects/"+pid+"/members", url.Values{"user_id": {sid}}), 303, "owner assigns staff")
	if r, _ := s.get("/projects/" + pid); r.StatusCode != 200 {
		t.Fatalf("assigned staff must see the project, got %d", r.StatusCode)
	}
	if r, _ := s.get("/clients/" + cid); r.StatusCode != 200 {
		t.Fatalf("assigned staff must see the client, got %d", r.StatusCode)
	}
	cid2 := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Other Co"}}), 303, "client B"))
	pid2 := lastSegment(o.want(o.post("/projects", url.Values{"client_org_id": {cid2}, "name": {"Secret"}}), 303, "project B"))
	if r, _ := s.get("/projects/" + pid2); r.StatusCode != 404 {
		t.Fatalf("staff must not see another project, got %d", r.StatusCode)
	}
	if _, body := s.get("/projects?q=Secret"); strings.Contains(body, "/projects/"+pid2) {
		t.Fatal("search must not leak unassigned projects")
	}

	// Close makes the project read-only; owner reopens with a reason.
	o.want(o.post("/projects/"+pid+"/close", nil), 303, "close")
	o.want(o.post("/projects/"+pid+"/milestones", url.Values{"title": {"Late"}, "visibility": {"internal"}}), 409, "closed project rejects writes")
	s.want(s.post("/projects/"+pid+"/reopen", url.Values{"reason": {"more work"}}), 403, "staff cannot reopen")
	o.want(o.post("/projects/"+pid+"/reopen", url.Values{"reason": {""}}), 422, "reopen needs a reason")
	o.want(o.post("/projects/"+pid+"/reopen", url.Values{"reason": {"more work"}}), 303, "reopen")
	o.want(o.post("/projects/"+pid+"/milestones", url.Values{"title": {"Late"}, "visibility": {"internal"}}), 303, "writes work again")

	// Removing staff ends their session immediately.
	o.want(o.post("/team/"+sid+"/remove", nil), 303, "remove staff")
	if r, _ := s.get("/projects/" + pid); r.StatusCode != 303 {
		t.Fatalf("removed staff must be logged out, got %d", r.StatusCode)
	}
	if r, _ := o.get("/activity"); r.StatusCode != 200 {
		t.Fatalf("activity page: %d", r.StatusCode)
	}
}
