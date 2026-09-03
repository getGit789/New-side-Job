package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"briefrelay/internal/auth"
	"briefrelay/internal/db"
	"briefrelay/internal/domain"
)

// Every scoped record is loaded through these helpers, so authorization lives in one file.
// Contract: docs/product-contract.md §1–§3. Denied reads are 404 so existence is not revealed.

var (
	errNotFound  = errors.New("not found")
	errForbidden = errors.New("forbidden")
	errClosed    = errors.New("project is closed")
	errInvalid   = errors.New("invalid input")
)

const pageSize = 25

func (s *Server) user(r *http.Request) *auth.User {
	u, _ := r.Context().Value(userKey).(*auth.User)
	return u
}

func (s *Server) role(r *http.Request) domain.Role {
	if u := s.user(r); u != nil {
		return domain.Role(u.Role)
	}
	return ""
}

func (s *Server) requireStaff(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := s.user(r)
		switch {
		case u == nil:
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		case !domain.Role(u.Role).IsStaff():
			s.errorPage(w, r, http.StatusForbidden, "This area is for the workspace team.")
		default:
			h(w, r)
		}
	}
}

func (s *Server) requireUser(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.user(r) == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		h(w, r)
	}
}

func (s *Server) requireClient(h http.HandlerFunc) http.HandlerFunc {
	return s.requireUser(func(w http.ResponseWriter, r *http.Request) {
		if s.role(r) != domain.RoleClient {
			s.errorPage(w, r, http.StatusForbidden, "This area is for clients.")
			return
		}
		h(w, r)
	})
}

func (s *Server) requireOwner(h http.HandlerFunc) http.HandlerFunc {
	return s.requireStaff(func(w http.ResponseWriter, r *http.Request) {
		if s.role(r) != domain.RoleOwner {
			s.errorPage(w, r, http.StatusForbidden, "Only the workspace owner can do this.")
			return
		}
		h(w, r)
	})
}

// fail maps domain errors to HTTP responses.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errNotFound), errors.Is(err, sql.ErrNoRows):
		s.errorPage(w, r, http.StatusNotFound, "That record does not exist.")
	case errors.Is(err, errForbidden):
		s.errorPage(w, r, http.StatusForbidden, "You cannot do that.")
	case errors.Is(err, errClosed):
		s.errorPage(w, r, http.StatusConflict, "This project is closed. Reopen it to make changes.")
	case errors.Is(err, domain.ErrTransition):
		s.errorPage(w, r, http.StatusConflict, "That change is not allowed from the current state: "+err.Error())
	case errors.Is(err, errInvalid):
		s.errorPage(w, r, http.StatusUnprocessableEntity, err.Error())
	default:
		s.log.Error("request failed", "path", r.URL.Path, "err", err)
		s.errorPage(w, r, http.StatusInternalServerError, "Something went wrong. The error has been logged.")
	}
}

// projectScope returns the SQL condition limiting projects (aliased p) to what the user may see.
func projectScope(u *auth.User) (string, []any) {
	switch domain.Role(u.Role) {
	case domain.RoleOwner:
		return "p.workspace_id = ?", []any{u.WorkspaceID}
	case domain.RoleClient: // contract §6.1: only the contact's own organization, and only while the contact is active
		return "p.workspace_id = ? AND p.client_org_id IN (SELECT client_org_id FROM client_contacts WHERE user_id = ? AND removed_at IS NULL)", []any{u.WorkspaceID, u.ID}
	}
	return "p.workspace_id = ? AND p.id IN (SELECT project_id FROM project_members WHERE user_id = ?)", []any{u.WorkspaceID, u.ID}
}

type Project struct {
	ID, WorkspaceID, ClientOrgID, ClientName, Name, Summary, Status, CreatedAt, UpdatedAt string
	ClosedAt                                                                              sql.NullString
}

func (p Project) Closed() bool { return p.Status == "closed" }

func (s *Server) loadProject(r *http.Request, id string) (Project, error) {
	u := s.user(r)
	cond, args := projectScope(u)
	var p Project
	err := s.db.R.QueryRowContext(r.Context(), `SELECT p.id, p.workspace_id, p.client_org_id, c.name, p.name, p.summary, p.status, p.created_at, p.updated_at, p.closed_at
		FROM projects p JOIN client_orgs c ON c.id = p.client_org_id WHERE p.id = ? AND `+cond, append([]any{id}, args...)...).
		Scan(&p.ID, &p.WorkspaceID, &p.ClientOrgID, &p.ClientName, &p.Name, &p.Summary, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.ClosedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, errNotFound
	}
	return p, err
}

// loadWritableProject is loadProject plus the closed-project rule (§5).
func (s *Server) loadWritableProject(r *http.Request, id string) (Project, error) {
	p, err := s.loadProject(r, id)
	if err == nil && p.Closed() {
		return p, errClosed
	}
	return p, err
}

// projectOf resolves the project id of a child record by table, then loads it with scoping.
func (s *Server) projectOf(r *http.Request, table, id string, writable bool) (Project, error) {
	var pid string
	err := s.db.R.QueryRowContext(r.Context(), `SELECT project_id FROM `+table+` WHERE id = ?`, id).Scan(&pid)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, errNotFound
	}
	if err != nil {
		return Project{}, err
	}
	if writable {
		return s.loadWritableProject(r, pid)
	}
	return s.loadProject(r, pid)
}

// audit records an event with the actor, IP, and optional metadata (never secrets).
func (s *Server) audit(ctx context.Context, tx *sql.Tx, r *http.Request, action, targetType, targetID string, meta map[string]any) error {
	u := s.user(r)
	actor, ws := "", ""
	if u != nil {
		actor, ws = u.ID, u.WorkspaceID
	}
	body := "{}"
	if len(meta) > 0 {
		b, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		body = string(b)
	}
	return db.Audit(ctx, tx, ws, actor, action, targetType, targetID, s.clientIP(r), body)
}

func (s *Server) logErr(msg string, err error) {
	if err != nil {
		s.log.Error(msg, slog.Any("err", err))
	}
}

// pageArgs reads ?page=N (1-based) and returns limit and offset.
func pageArgs(r *http.Request) (page, limit, offset int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	return page, pageSize, (page - 1) * pageSize
}

func invalid(msg string) error { return fmt.Errorf("%w: %s", errInvalid, msg) }

func within(s string, min, max int) bool { return len(s) >= min && len(s) <= max }
