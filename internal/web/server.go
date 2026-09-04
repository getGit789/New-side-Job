// Package web is the HTTP layer: routing, middleware, setup wizard, login, health.
package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"briefrelay/internal/auth"
	"briefrelay/internal/config"
	"briefrelay/internal/db"
	"briefrelay/internal/domain"
	"briefrelay/internal/jobs"
	"briefrelay/internal/mail"
	"briefrelay/internal/storage"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

const sessionCookie = "br_session"

// Version is set at build time with -ldflags "-X briefrelay/internal/web.Version=1.0.0".
var Version = "dev"

type Server struct {
	cfg       config.Config
	db        *db.DB
	q         *jobs.Queue
	mail      *mail.Mailer
	store     *storage.Local
	log       *slog.Logger
	pages     map[string]*template.Template
	limiter   *ipLimiter
	authLimit *ipLimiter
	installed atomic.Bool
	prefs     atomic.Pointer[prefs] // workspace settings; see settings.go
	assets    string                // hash of the static files, so a changed stylesheet gets a new URL
	dummyHash string                // keeps failed logins for unknown users as slow as real ones
}

// funcs are the template helpers. date renders in the workspace time zone and format (plan §5.4).
func (s *Server) funcs() template.FuncMap {
	return template.FuncMap{
		"date": func(v string) string {
			t, err := db.ParseTime(v)
			if err != nil {
				return v
			}
			p := s.prefs.Load()
			return t.In(p.loc).Format(p.DateFormat + " 15:04")
		},
		"money": func(cents int64, cur string) string {
			return fmt.Sprintf("%s %d.%02d", cur, cents/100, cents%100)
		},
		"amount": func(cents int64) string { return fmt.Sprintf("%d.%02d", cents/100, cents%100) },
		"human":  func(s string) string { return strings.ReplaceAll(s, "_", " ") },
		"add":    func(a, b int) int { return a + b },
	}
}

func sortStrings(v []string) []string { sort.Strings(v); return v }

func New(cfg config.Config, d *db.DB, q *jobs.Queue, m *mail.Mailer, st *storage.Local, log *slog.Logger) (*Server, error) {
	s := &Server{cfg: cfg, db: d, q: q, mail: m, store: st, log: log, pages: map[string]*template.Template{},
		limiter: newIPLimiter(20, 40), authLimit: newIPLimiter(0.1, 5), dummyHash: auth.HashPassword("dummy")}
	if err := s.loadPrefs(context.Background()); err != nil {
		return nil, err
	}
	h := sha256.New()
	for _, name := range []string{"static/app.css", "static/app.js"} {
		b, err := staticFS.ReadFile(name)
		if err != nil {
			return nil, err
		}
		h.Write(b)
	}
	s.assets = hex.EncodeToString(h.Sum(nil))[:12]
	for _, p := range []string{"setup", "login", "home", "error", "clients", "client", "projects", "project", "deliverable", "team", "invite", "activity", "notifications", "portal_project", "portal_deliverable", "portal_intake", "account", "forgot", "reset", "settings", "search"} {
		t, err := template.New("").Funcs(s.funcs()).ParseFS(templateFS, "templates/layout.html", "templates/"+p+".html")
		if err != nil {
			return nil, err
		}
		s.pages[p] = t
	}
	_, ok, err := d.Setting(context.Background(), "installed")
	if err != nil {
		return nil, err
	}
	s.installed.Store(ok)
	return s, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /setup", s.setupForm)
	mux.HandleFunc("POST /setup", s.setupSubmit)
	mux.HandleFunc("GET /login", s.loginForm)
	mux.HandleFunc("POST /login", s.loginSubmit)
	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("GET /{$}", s.home)
	mux.Handle("GET /static/", s.static())
	s.clientRoutes(mux)
	s.projectRoutes(mux)
	s.deliverableRoutes(mux)
	s.invoiceRoutes(mux)
	s.adminRoutes(mux)
	s.notificationRoutes(mux)
	s.portalRoutes(mux)
	s.accountRoutes(mux)
	s.exportRoutes(mux)
	s.settingsRoutes(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.errorPage(w, r, http.StatusNotFound, "That page does not exist.")
	})

	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.errorPage(w, r, http.StatusForbidden, "Cross-site request blocked.")
	}))
	var h http.Handler = cop.Handler(mux)
	h = s.withUser(h)
	h = s.setupGate(h)
	h = s.rateLimit(h)
	h = s.securityHeaders(h)
	h = s.logging(h)
	h = s.recoverer(h)
	return h
}

// static serves the stylesheet, script and font with long caching; templates append ?v=<content hash>
// so any change to the files changes the URL and no browser keeps a stale copy.
func (s *Server) static() http.Handler {
	fs := http.FileServerFS(staticFS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			s.errorPage(w, r, http.StatusNotFound, "That page does not exist.")
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fs.ServeHTTP(w, r)
	})
}

// ---- middleware ----

type ctxKey int

const userKey ctxKey = 1

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil && p != http.ErrAbortHandler {
				s.log.Error("panic", "err", p, "path", r.URL.Path)
				s.errorPage(w, r, http.StatusInternalServerError, "Something went wrong. The error has been logged.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(c int) { w.status = c; w.ResponseWriter.WriteHeader(c) }

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b [8]byte
		rand.Read(b[:])
		id := hex.EncodeToString(b[:])
		w.Header().Set("X-Request-Id", id)
		sw := &statusWriter{ResponseWriter: w, status: 200}
		start := time.Now()
		next.ServeHTTP(sw, r)
		s.log.Info("http", "id", id, "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds(), "ip", s.clientIP(r))
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; frame-ancestors 'none'; form-action 'self'; base-uri 'self'; object-src 'none'")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if s.cfg.BaseURL.Scheme == "https" {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		ip := s.clientIP(r)
		authPath := r.Method == http.MethodPost && (r.URL.Path == "/login" || r.URL.Path == "/setup" || strings.HasPrefix(r.URL.Path, "/invite/") || strings.HasPrefix(r.URL.Path, "/password/"))
		if !s.limiter.allow(ip) || (authPath && !s.authLimit.allow(ip)) {
			w.Header().Set("Retry-After", "60")
			s.errorPage(w, r, http.StatusTooManyRequests, "Too many requests. Please wait a minute and try again.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// setupGate sends everything to /setup until the owner exists, then makes /setup disappear.
func (s *Server) setupGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isSetup := r.URL.Path == "/setup"
		switch {
		case !s.installed.Load() && !isSetup && r.URL.Path != "/healthz" && !strings.HasPrefix(r.URL.Path, "/static/"):
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
		case s.installed.Load() && isSetup:
			s.errorPage(w, r, http.StatusNotFound, "Setup is complete and locked.")
		default:
			next.ServeHTTP(w, r)
		}
	})
}

// withUser resolves the session cookie once per request. It hits the database every time, so logout is immediate.
func (s *Server) withUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil {
			u, ok, err := auth.UserBySession(r.Context(), s.db, c.Value)
			if err != nil {
				s.log.Error("session lookup", "err", err)
			}
			if ok {
				r = r.WithContext(context.WithValue(r.Context(), userKey, &u))
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: s.cfg.BaseURL.Scheme == "https", SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

// ---- rendering ----

type view struct {
	User      *auth.User
	Role      domain.Role
	IsOwner   bool
	Unread    int
	Demo      bool
	Workspace *prefs
	Version   string
	Title     string
	Error     string
	Form      map[string]string
	Status    string
	Message   string
	Data      any
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, page string, v view) {
	v.User = s.user(r)
	v.Demo = s.cfg.IsDemo()
	v.Workspace = s.prefs.Load()
	v.Version = s.assets
	if v.User != nil {
		v.Role = domain.Role(v.User.Role)
		v.IsOwner = v.Role == domain.RoleOwner
		v.Unread = s.unreadCount(r)
	}
	if v.Form == nil {
		v.Form = map[string]string{}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := s.pages[page].ExecuteTemplate(w, "layout", v); err != nil {
		s.log.Error("render", "page", page, "err", err)
	}
}

func (s *Server) errorPage(w http.ResponseWriter, r *http.Request, status int, msg string) {
	s.render(w, r, status, "error", view{Title: http.StatusText(status), Status: fmt.Sprintf("%d %s", status, http.StatusText(status)), Message: msg})
}

func (s *Server) parseForm(w http.ResponseWriter, r *http.Request) (map[string]string, bool) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	}
	if err := r.ParseMultipartForm(4 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		s.errorPage(w, r, http.StatusBadRequest, "Could not read the form.")
		return nil, false
	}
	out := map[string]string{}
	for k := range r.PostForm {
		out[k] = strings.TrimSpace(r.PostForm.Get(k))
	}
	return out, true
}

// ---- handlers ----

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	checks := map[string]string{"application": "ok", "database": "ok", "storage": "ok", "jobs": "ok", "mail": "ok"}
	healthy := true
	fail := func(k string, err error) { checks[k] = "fail: " + err.Error(); healthy = false }
	if err := s.db.R.PingContext(ctx); err != nil {
		fail("database", err)
	}
	if err := s.store.Writable(); err != nil {
		fail("storage", err)
	}
	if !s.q.Healthy() {
		fail("jobs", errors.New("worker has not ticked recently"))
	}
	if !s.mail.Configured() {
		checks["mail"] = "not configured (mail is logged, not sent)"
	}
	stats, _ := s.q.Stats(ctx)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	status := "ok"
	if !healthy {
		status = "degraded"
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(map[string]any{"status": status, "version": Version, "installed": s.installed.Load(), "checks": checks, "jobs": stats})
}

func (s *Server) setupForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "setup", view{Title: "Set up"})
}

var errAlreadyInstalled = errors.New("already installed")

func (s *Server) setupSubmit(w http.ResponseWriter, r *http.Request) {
	form, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	v := view{Title: "Set up", Form: form}
	email := strings.ToLower(form["email"])
	switch {
	case form["workspace"] == "" || form["name"] == "":
		v.Error = "Workspace name and your name are required."
	case !strings.Contains(email, "@"):
		v.Error = "Enter a valid email address."
	case auth.CheckPassword(form["password"]) != nil:
		v.Error = passwordRule
	}
	if v.Error != "" {
		s.render(w, r, http.StatusUnprocessableEntity, "setup", v)
		return
	}
	var token string
	ip := s.clientIP(r)
	err := s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT count(*) FROM settings WHERE key = 'installed'`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return errAlreadyInstalled
		}
		now, wsID, userID := db.Now(), db.NewID(), db.NewID()
		if _, err := tx.Exec(`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, wsID, form["workspace"], now, now); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO users (id, email, name, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, userID, email, form["name"], auth.HashPassword(form["password"]), now, now); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO memberships (workspace_id, user_id, role, created_at) VALUES (?, ?, 'owner', ?)`, wsID, userID, now); err != nil {
			return err
		}
		for k, val := range map[string]string{"installed": "1", "installed_at": now, "installed_version": Version} {
			if err := db.SetSetting(r.Context(), tx, k, val); err != nil {
				return err
			}
		}
		if err := db.Audit(r.Context(), tx, wsID, userID, "setup.completed", "workspace", wsID, ip, ""); err != nil {
			return err
		}
		var err error
		token, err = auth.CreateSession(r.Context(), tx, userID, ip, r.UserAgent())
		return err
	})
	if errors.Is(err, errAlreadyInstalled) {
		s.installed.Store(true)
		s.errorPage(w, r, http.StatusNotFound, "Setup is complete and locked.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.installed.Store(true)
	s.logErr("load prefs", s.loadPrefs(r.Context()))
	s.setSessionCookie(w, token, int(auth.SessionTTL.Seconds()))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// passwordRule is the one user-facing sentence for contract §8's password rule.
const passwordRule = "Password must be 12 to 200 characters and not a commonly used password."

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	if s.user(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, r, http.StatusOK, "login", view{Title: "Log in"})
}

func (s *Server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	form, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	email := strings.ToLower(form["email"])
	ip := s.clientIP(r)
	var userID, hash string
	err := s.db.R.QueryRowContext(r.Context(), `SELECT id, password_hash FROM users WHERE email = ?`, email).Scan(&userID, &hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.fail(w, r, err)
		return
	}
	if hash == "" {
		hash = s.dummyHash
	}
	valid, _ := auth.VerifyPassword(hash, form["password"])
	if !valid || userID == "" {
		s.logErr("audit", s.db.Tx(r.Context(), func(tx *sql.Tx) error {
			return db.Audit(r.Context(), tx, "", "", "auth.login_failed", "user", email, ip, "")
		}))
		s.render(w, r, http.StatusUnauthorized, "login", view{Title: "Log in", Form: map[string]string{"email": form["email"]}, Error: "Email or password is incorrect."})
		return
	}
	var token string
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		if err := db.Audit(r.Context(), tx, "", userID, "auth.login", "user", userID, ip, ""); err != nil {
			return err
		}
		var err error
		token, err = auth.CreateSession(r.Context(), tx, userID, ip, r.UserAgent())
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.setSessionCookie(w, token, int(auth.SessionTTL.Seconds()))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		u := s.user(r)
		s.logErr("logout", s.db.Tx(r.Context(), func(tx *sql.Tx) error {
			if u != nil {
				if err := db.Audit(r.Context(), tx, u.WorkspaceID, u.ID, "auth.logout", "user", u.ID, s.clientIP(r), ""); err != nil {
					return err
				}
			}
			return auth.DeleteSession(r.Context(), tx, c.Value)
		}))
	}
	s.setSessionCookie(w, "", -1)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
