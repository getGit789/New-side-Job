package web

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// AT-9: every route is asserted for every actor (docs/product-contract.md §3).
//
// Cells: "ok" = authorized (any status except 403/404/login redirect; validation
// errors count as authorized), "403" = wrong role, "404" = out of scope.
// Order of columns: owner, assigned staff, unassigned staff, client of org A, client of org B.
// Anonymous users must be sent to /login on every non-public route.
var matrix = map[string][5]string{
	"GET /{$}":                    {"ok", "ok", "ok", "ok", "ok"},
	"GET /notifications":          {"ok", "ok", "ok", "ok", "ok"},
	"POST /notifications/read":    {"ok", "ok", "ok", "ok", "ok"},
	"GET /account":                {"ok", "ok", "ok", "ok", "ok"},
	"POST /account/password":      {"ok", "ok", "ok", "ok", "ok"},
	"POST /account/email":         {"ok", "ok", "ok", "ok", "ok"},
	"POST /account/notifications": {"ok", "ok", "ok", "ok", "ok"},
	"GET /settings":               {"ok", "403", "403", "403", "403"},
	"POST /settings":              {"ok", "403", "403", "403", "403"},
	"GET /search":                 {"ok", "ok", "ok", "403", "403"},

	"GET /clients/export.csv":       {"ok", "403", "403", "403", "403"},
	"POST /clients/import":          {"ok", "403", "403", "403", "403"},
	"POST /clients/{id}/delete":     {"ok", "403", "403", "403", "403"},
	"GET /projects/{id}/export.zip": {"ok", "403", "403", "403", "403"},
	"POST /projects/{id}/delete":    {"ok", "403", "403", "403", "403"},

	"GET /clients":                              {"ok", "ok", "ok", "403", "403"},
	"POST /clients":                             {"ok", "403", "403", "403", "403"},
	"GET /clients/{id}":                         {"ok", "ok", "404", "403", "403"},
	"POST /clients/{id}":                        {"ok", "403", "403", "403", "403"},
	"POST /clients/{id}/contacts":               {"ok", "ok", "404", "403", "403"},
	"POST /clients/{id}/contacts/{cid}/remove":  {"ok", "ok", "404", "403", "403"},
	"POST /clients/{id}/contacts/{cid}/invite":  {"ok", "ok", "404", "403", "403"},
	"POST /clients/{id}/archive":                {"ok", "403", "403", "403", "403"},
	"POST /clients/{id}/unarchive":              {"ok", "403", "403", "403", "403"},
	"GET /projects":                             {"ok", "ok", "ok", "403", "403"},
	"POST /projects":                            {"ok", "ok", "ok", "403", "403"},
	"GET /projects/{id}":                        {"ok", "ok", "404", "403", "403"},
	"POST /projects/{id}":                       {"ok", "ok", "404", "403", "403"},
	"POST /projects/{id}/close":                 {"ok", "ok", "404", "403", "403"},
	"POST /projects/{id}/reopen":                {"ok", "403", "403", "403", "403"},
	"POST /projects/{id}/members":               {"ok", "403", "403", "403", "403"},
	"POST /projects/{id}/members/{uid}/remove":  {"ok", "403", "403", "403", "403"},
	"POST /projects/{id}/milestones":            {"ok", "ok", "404", "403", "403"},
	"POST /milestones/{id}":                     {"ok", "ok", "404", "403", "403"},
	"POST /milestones/{id}/delete":              {"ok", "ok", "404", "403", "403"},
	"POST /projects/{id}/intake/comment":        {"ok", "ok", "404", "403", "403"},
	"POST /projects/{id}/deliverables":          {"ok", "ok", "404", "403", "403"},
	"GET /deliverables/{id}":                    {"ok", "ok", "404", "403", "403"},
	"POST /deliverables/{id}":                   {"ok", "ok", "404", "403", "403"},
	"POST /deliverables/{id}/versions":          {"ok", "ok", "404", "403", "403"},
	"POST /versions/{id}/share":                 {"ok", "ok", "404", "403", "403"},
	"POST /versions/{id}/withdraw":              {"ok", "ok", "404", "403", "403"},
	"POST /versions/{id}/delete":                {"ok", "ok", "404", "403", "403"},
	"GET /versions/{id}/download":               {"ok", "ok", "404", "403", "403"},
	"POST /versions/{id}/comment":               {"ok", "ok", "404", "403", "403"},
	"POST /deliverables/{id}/waive":             {"ok", "403", "403", "403", "403"},
	"POST /deliverables/{id}/delete":            {"ok", "ok", "404", "403", "403"},
	"POST /comments/{id}/delete":                {"ok", "ok", "404", "403", "403"},
	"POST /invoices/{id}":                       {"ok", "ok", "404", "403", "403"},
	"POST /projects/{id}/invoices":              {"ok", "ok", "404", "403", "403"},
	"POST /invoices/{id}/status":                {"ok", "ok", "404", "403", "403"},
	"GET /invoices/{id}/document":               {"ok", "ok", "404", "403", "403"},
	"GET /team":                                 {"ok", "403", "403", "403", "403"},
	"POST /team/invite":                         {"ok", "403", "403", "403", "403"},
	"POST /invitations/{id}/revoke":             {"ok", "403", "403", "403", "403"},
	"POST /team/{uid}/remove":                   {"ok", "403", "403", "403", "403"},
	"GET /activity":                             {"ok", "403", "403", "403", "403"},
	"GET /portal/projects/{id}":                 {"403", "403", "403", "ok", "404"},
	"GET /portal/deliverables/{id}":             {"403", "403", "403", "ok", "404"},
	"POST /portal/versions/{id}/comment":        {"403", "403", "403", "ok", "404"},
	"POST /portal/comments/{id}/delete":         {"403", "403", "403", "ok", "404"},
	"POST /portal/versions/{id}/decide":         {"403", "403", "403", "ok", "404"},
	"GET /portal/versions/{id}/download":        {"403", "403", "403", "ok", "404"},
	"GET /portal/invoices/{id}/document":        {"403", "403", "403", "ok", "404"},
	"GET /portal/projects/{id}/intake":          {"403", "403", "403", "ok", "404"},
	"POST /portal/projects/{id}/intake":         {"403", "403", "403", "ok", "404"},
	"POST /portal/projects/{id}/intake/comment": {"403", "403", "403", "ok", "404"},
	"POST /portal/projects/{id}/signoff":        {"403", "403", "403", "ok", "404"},
}

// Routes that are reachable without a session; they are covered by the journey tests.
var publicRoutes = map[string]bool{
	"GET /healthz": true, "GET /setup": true, "POST /setup": true, "GET /login": true, "POST /login": true,
	"POST /logout": true, "GET /invite/{token}": true, "POST /invite/{token}": true, "/": true,
	"GET /password/forgot": true, "POST /password/forgot": true, "GET /password/reset/{token}": true, "POST /password/reset/{token}": true,
	"GET /logo": true, "GET /static/": true,
}

func TestPermissionMatrix(t *testing.T) {
	// Every registered route must be in the matrix or declared public.
	routeRe := regexp.MustCompile(`mux\.Handle(?:Func)?\("([^"]+)"`)
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, _ := os.ReadFile(f)
		for _, m := range routeRe.FindAllStringSubmatch(string(src), -1) {
			if _, ok := matrix[m[1]]; !ok && !publicRoutes[m[1]] {
				t.Errorf("%s: route %q is not in the permission matrix", f, m[1])
			}
		}
	}

	e := newEnv(t)
	o := setupOwner(e)
	// Org A with project A (assigned staff, contact a); org B with project B (contact b).
	cid := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Org A"}}), 303, "org A"))
	o.want(o.post("/clients/"+cid+"/contacts", url.Values{"name": {"Al"}, "email": {"al@a.test"}}), 303, "contact a")
	ctid := e.scalar(`SELECT id FROM client_contacts WHERE email = 'al@a.test'`)
	// Staff and owner each get their own spare contact and milestone, because their "ok" cells delete them.
	o.want(o.post("/clients/"+cid+"/contacts", url.Values{"name": {"Spare"}, "email": {"spare0@a.test"}}), 303, "spare contact")
	o.want(o.post("/clients/"+cid+"/contacts", url.Values{"name": {"Spare"}, "email": {"spare1@a.test"}}), 303, "spare contact")
	spares := []string{e.scalar(`SELECT id FROM client_contacts WHERE email = 'spare0@a.test'`), e.scalar(`SELECT id FROM client_contacts WHERE email = 'spare1@a.test'`)}
	cidB := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Org B"}}), 303, "org B"))
	o.want(o.post("/clients/"+cidB+"/contacts", url.Values{"name": {"Bea"}, "email": {"bea@b.test"}}), 303, "contact b")
	ctidB := e.scalar(`SELECT id FROM client_contacts WHERE email = 'bea@b.test'`)
	pid := lastSegment(o.want(o.post("/projects", url.Values{"client_org_id": {cid}, "name": {"Project A"}}), 303, "project A"))
	o.want(o.post("/projects", url.Values{"client_org_id": {cidB}, "name": {"Project B"}}), 303, "project B")
	// Records that only the two permanent-delete routes touch.
	pidDel := lastSegment(o.want(o.post("/projects", url.Values{"client_org_id": {cid}, "name": {"Done"}}), 303, "closed project"))
	o.want(o.post("/projects/"+pidDel+"/close", nil), 303, "close it")
	cidDel := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Gone Co"}}), 303, "archived org"))
	o.want(o.post("/clients/"+cidDel+"/archive", nil), 303, "archive it")
	o.want(o.post("/projects/"+pid+"/milestones", url.Values{"title": {"M0"}, "visibility": {"client"}}), 303, "milestone")
	o.want(o.post("/projects/"+pid+"/milestones", url.Values{"title": {"M1"}, "visibility": {"client"}}), 303, "milestone")
	mids := []string{e.scalar(`SELECT id FROM milestones WHERE title = 'M0'`), e.scalar(`SELECT id FROM milestones WHERE title = 'M1'`)}
	did := lastSegment(o.want(o.post("/projects/"+pid+"/deliverables", url.Values{"title": {"Logo"}, "required": {"1"}}), 303, "deliverable"))
	o.want(o.upload("/deliverables/"+did+"/versions", nil, "file", "logo.pdf", "%PDF-1.4 v1"), 303, "v1")
	vid := e.scalar(`SELECT id FROM deliverable_versions WHERE deliverable_id = ?`, did)
	o.want(o.post("/versions/"+vid+"/share", nil), 303, "share v1")
	o.want(o.post("/versions/"+vid+"/comment", url.Values{"body": {"owner note"}, "visibility": {"client"}}), 303, "owner comment")
	commentID := e.scalar(`SELECT id FROM comments WHERE body = 'owner note'`)
	o.want(o.upload("/projects/"+pid+"/invoices", map[string]string{"number": "INV-1", "amount": "10", "currency": "USD", "visibility": "client"}, "document", "inv.pdf", "%PDF-1.4 inv"), 303, "invoice")
	iid := e.scalar(`SELECT id FROM invoices WHERE project_id = ?`, pid)
	o.want(o.post("/invoices/"+iid+"/status", url.Values{"to": {"sent"}}), 303, "send invoice") // clients never see drafts

	o.want(o.post("/team/invite", url.Values{"email": {"in@acme.test"}}), 303, "invite staff in")
	staffIn := acceptInvite(e, "In")
	o.want(o.post("/team/invite", url.Values{"email": {"out@acme.test"}}), 303, "invite staff out")
	staffOut := acceptInvite(e, "Out")
	o.want(o.post("/team/invite", url.Values{"email": {"pending@acme.test"}}), 303, "pending invite")
	invID := e.scalar(`SELECT id FROM invitations WHERE email = 'pending@acme.test'`)
	uid := e.scalar(`SELECT id FROM users WHERE email = 'in@acme.test'`)
	o.want(o.post("/projects/"+pid+"/members", url.Values{"user_id": {uid}}), 303, "assign staff in")
	o.want(o.post("/clients/"+cid+"/contacts/"+ctid+"/invite", nil), 303, "invite contact a")
	clientA := acceptInvite(e, "Al")
	o.want(o.post("/clients/"+cidB+"/contacts/"+ctidB+"/invite", nil), 303, "invite contact b")
	clientB := acceptInvite(e, "Bea")

	idFor := func(pattern string, col int) string {
		if col > 1 {
			col = 1
		}
		switch {
		case pattern == "POST /clients/{id}/delete":
			return cidDel
		case pattern == "POST /projects/{id}/delete":
			return pidDel
		case strings.Contains(pattern, "/clients/{id}"):
			return cid
		case strings.Contains(pattern, "/projects/{id}"):
			return pid
		case strings.Contains(pattern, "/deliverables/{id}"):
			return did
		case strings.Contains(pattern, "/versions/{id}"):
			return vid
		case strings.Contains(pattern, "/comments/{id}"):
			return commentID
		case strings.Contains(pattern, "/milestones/{id}"):
			return mids[col]
		case strings.Contains(pattern, "/invoices/{id}"):
			return iid
		case strings.Contains(pattern, "/invitations/{id}"):
			return invID
		}
		return ""
	}
	call := func(b *browser, pattern string, col int) (int, string) {
		method, path, _ := strings.Cut(pattern, " ")
		path = strings.Replace(path, "{id}", idFor(pattern, col), 1)
		path = strings.NewReplacer("{$}", "", "{cid}", spares[min(col, 1)], "{uid}", uid).Replace(path)
		if method == "GET" {
			r, _ := b.get(path)
			return r.StatusCode, r.Header.Get("Location")
		}
		r := b.post(path, url.Values{})
		return r.StatusCode, r.Header.Get("Location")
	}

	// Denied actors first so successful owner writes (close, remove) cannot mask a leak.
	// Sorted so "invite" runs before "remove" on the spare contact.
	patterns := make([]string, 0, len(matrix))
	for p := range matrix {
		patterns = append(patterns, p)
	}
	sort.Strings(patterns)
	anon := e.browser()
	for _, pattern := range patterns {
		if code, loc := call(anon, pattern, 1); code != 303 || loc != "/login" {
			t.Errorf("anonymous %s: got %d %s, want 303 /login", pattern, code, loc)
		}
	}
	actors := []struct {
		name string
		b    *browser
		col  int
	}{{"client B", clientB, 4}, {"client A", clientA, 3}, {"staff out", staffOut, 2}, {"staff in", staffIn, 1}, {"owner", o, 0}}
	for _, a := range actors {
		for _, pattern := range patterns {
			cells := matrix[pattern]
			code, loc := call(a.b, pattern, a.col)
			got := "ok"
			switch {
			case code == 403:
				got = "403"
			case code == 404:
				got = "404"
			case code == 303 && loc == "/login":
				got = "login"
			}
			if got != cells[a.col] {
				t.Errorf("%s %s: got %s (%d), want %s", a.name, pattern, got, code, cells[a.col])
			}
		}
	}
}
