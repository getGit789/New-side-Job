package web

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sort"
	"testing"
	"time"

	"briefrelay/internal/db"
)

// Performance budgets (plan §6.1) at the required seeded scale:
// 500 clients, 5,000 projects, 25,000 versions, 100,000 activity events.
// Opt in with BRIEFRELAY_PERF=1 (make perf); seeding takes a while.
func TestPerformanceBudgets(t *testing.T) {
	if os.Getenv("BRIEFRELAY_PERF") == "" {
		t.Skip("set BRIEFRELAY_PERF=1 to run the seeded benchmark")
	}
	e := newEnv(t)
	o := setupOwner(e)
	ws := e.scalar(`SELECT id FROM workspaces`)
	owner := e.scalar(`SELECT id FROM users`)

	start := time.Now()
	var pid, did string
	err := e.db.Tx(t.Context(), func(tx *sql.Tx) error {
		now := db.Now()
		org, _ := tx.Prepare(`INSERT INTO client_orgs (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`)
		contact, _ := tx.Prepare(`INSERT INTO client_contacts (id, client_org_id, name, email, created_at) VALUES (?, ?, ?, ?, ?)`)
		project, _ := tx.Prepare(`INSERT INTO projects (id, workspace_id, client_org_id, name, summary, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`)
		deliverable, _ := tx.Prepare(`INSERT INTO deliverables (id, project_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`)
		version, _ := tx.Prepare(`INSERT INTO deliverable_versions (id, deliverable_id, number, kind, url, state, created_by, created_at, updated_at) VALUES (?, ?, ?, 'link', 'https://example.test/v', ?, ?, ?, ?)`)
		audit, _ := tx.Prepare(`INSERT INTO audit_events (workspace_id, actor_id, action, target_type, target_id, meta, created_at) VALUES (?, ?, 'project.updated', 'project', ?, ?, ?)`)
		states := []string{"approved", "revision_requested", "approved", "withdrawn", "shared"}
		for c := 0; c < 500; c++ {
			cid := db.NewID()
			org.Exec(cid, ws, fmt.Sprintf("Client %03d", c), now, now)
			contact.Exec(db.NewID(), cid, "Contact", fmt.Sprintf("c%d@example.test", c), now)
			for p := 0; p < 10; p++ {
				id := db.NewID()
				project.Exec(id, ws, cid, fmt.Sprintf("Project %03d-%d", c, p), "Seeded", now, now)
				pid = id
				d := db.NewID()
				deliverable.Exec(d, id, "Deliverable", now, now)
				did = d
				for v := 1; v <= 5; v++ { // 5,000 deliverables × 5 = 25,000 versions
					version.Exec(db.NewID(), d, v, states[v-1], owner, now, now)
				}
				for a := 0; a < 20; a++ { // 5,000 × 20 = 100,000 events
					audit.Exec(ws, owner, id, `{"project_id":"`+id+`"}`, now)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("seeded in %s", time.Since(start).Round(time.Millisecond))

	// A client of the last project, for the portal pages.
	cid := e.scalar(`SELECT client_org_id FROM projects WHERE id = ?`, pid)
	ctid := e.scalar(`SELECT id FROM client_contacts WHERE client_org_id = ?`, cid)
	o.want(o.post("/clients/"+cid+"/contacts/"+ctid+"/invite", nil), 303, "invite")
	c := acceptInvite(e, "Perf")

	p95 := func(name string, budget time.Duration, hit func()) {
		const n = 40
		d := make([]time.Duration, n)
		for i := range d {
			s := time.Now()
			hit()
			d[i] = time.Since(s)
		}
		sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
		got := d[n*95/100]
		status := "ok"
		if got > budget {
			status = "OVER BUDGET"
			t.Errorf("%s: p95 %s > %s", name, got, budget)
		}
		t.Logf("%-32s p95 %6s  median %6s  budget %s  %s", name, got.Round(time.Millisecond), d[n/2].Round(time.Millisecond), budget, status)
	}
	get := func(b *browser, path string) func() {
		return func() {
			if r, _ := b.get(path); r.StatusCode != 200 {
				t.Fatalf("%s: %d", path, r.StatusCode)
			}
		}
	}
	read, write, search := 300*time.Millisecond, 500*time.Millisecond, 500*time.Millisecond
	p95("GET /projects", read, get(o, "/projects"))
	p95("GET /projects?q=", search, get(o, "/projects?q="+url.QueryEscape("Project 42")))
	p95("GET /clients", read, get(o, "/clients"))
	p95("GET /clients?q=", search, get(o, "/clients?q=Client+4"))
	p95("GET /projects/{id}", read, get(o, "/projects/"+pid))
	p95("GET /clients/{id}", read, get(o, "/clients/"+cid))
	p95("GET /deliverables/{id}", read, get(o, "/deliverables/"+did))
	p95("GET /activity", read, get(o, "/activity"))
	p95("GET / (dashboard)", read, get(o, "/"))
	p95("GET /portal/projects/{id}", read, get(c, "/portal/projects/"+pid))
	p95("GET /portal/deliverables/{id}", read, get(c, "/portal/deliverables/"+did))
	p95("POST milestone (write)", write, func() {
		o.want(o.post("/projects/"+pid+"/milestones", url.Values{"title": {"Perf"}, "visibility": {"internal"}}), 303, "milestone")
	})
	p95("POST comment (write)", write, func() {
		vid := e.scalar(`SELECT id FROM deliverable_versions WHERE deliverable_id = ? AND number = 5`, did)
		c.want(c.post("/portal/versions/"+vid+"/comment", url.Values{"body": {"Looks fine"}}), 303, "comment")
	})
}
