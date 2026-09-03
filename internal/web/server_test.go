package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"briefrelay/internal/config"
	"briefrelay/internal/db"
	"briefrelay/internal/jobs"
	"briefrelay/internal/mail"
	"briefrelay/internal/storage"
)

func newTestServer(t *testing.T) (*httptest.Server, *http.Client) {
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
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	jar := &cookieJar{}
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return ts, client
}

type cookieJar struct{ cookies []*http.Cookie }

func (j *cookieJar) SetCookies(_ *url.URL, cs []*http.Cookie) { j.cookies = cs }
func (j *cookieJar) Cookies(*url.URL) []*http.Cookie          { return j.cookies }

func TestInstallLoginLogoutJourney(t *testing.T) {
	ts, c := newTestServer(t)
	get := func(path string) *http.Response {
		r, err := c.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r
	}
	post := func(path string, form url.Values) *http.Response {
		r, err := c.PostForm(ts.URL+path, form)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r
	}
	if r := get("/"); r.StatusCode != 303 || r.Header.Get("Location") != "/setup" {
		t.Fatalf("fresh install must redirect to /setup, got %d %s", r.StatusCode, r.Header.Get("Location"))
	}
	if r := get("/healthz"); r.StatusCode != 503 { // worker is not running in this test, so jobs check fails
		t.Fatalf("healthz before worker: %d", r.StatusCode)
	}
	if r := post("/setup", url.Values{"workspace": {"Acme"}, "name": {"Ann"}, "email": {"ann@acme.test"}, "password": {"short"}}); r.StatusCode != 422 {
		t.Fatalf("weak password accepted: %d", r.StatusCode)
	}
	if r := post("/setup", url.Values{"workspace": {"Acme"}, "name": {"Ann"}, "email": {"Ann@Acme.test"}, "password": {"a-long-enough-password"}}); r.StatusCode != 303 {
		t.Fatalf("setup: %d", r.StatusCode)
	}
	if r := get("/setup"); r.StatusCode != 404 {
		t.Fatalf("setup must lock after install: %d", r.StatusCode)
	}
	r, _ := c.Get(ts.URL + "/")
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 200 || !strings.Contains(string(body), "ann@acme.test") || r.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("home after setup: %d headers=%v", r.StatusCode, r.Header)
	}
	if r := post("/logout", nil); r.StatusCode != 303 {
		t.Fatalf("logout: %d", r.StatusCode)
	}
	if r := get("/"); r.StatusCode != 303 || r.Header.Get("Location") != "/login" {
		t.Fatalf("after logout must redirect to /login: %d", r.StatusCode)
	}
	if r := post("/login", url.Values{"email": {"ann@acme.test"}, "password": {"wrong-password-here"}}); r.StatusCode != 401 {
		t.Fatalf("wrong password: %d", r.StatusCode)
	}
	if r := post("/login", url.Values{"email": {"ann@acme.test"}, "password": {"a-long-enough-password"}}); r.StatusCode != 303 {
		t.Fatalf("login: %d", r.StatusCode)
	}
	if r := get("/"); r.StatusCode != 200 {
		t.Fatalf("home after login: %d", r.StatusCode)
	}
	// Cross-site form post is refused by the stdlib CSRF protection.
	req, _ := http.NewRequest("POST", ts.URL+"/logout", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	if r, _ := c.Do(req); r.StatusCode != 403 {
		t.Fatalf("cross-site POST must be blocked: %d", r.StatusCode)
	}
}
