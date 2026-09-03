// Package seed fills an empty install with a synthetic agency: the public demo and the
// customer "sample data" option. Reset wipes everything and seeds again (demo mode, hourly).
package seed

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"briefrelay/internal/auth"
	"briefrelay/internal/db"
	"briefrelay/internal/storage"
)

// Sample credentials. They are printed on the demo login page and restored on every reset.
const (
	OwnerEmail, OwnerPassword   = "owner@demo.test", "demo-owner-password"
	StaffEmail, StaffPassword   = "staff@demo.test", "demo-staff-password"
	ClientEmail, ClientPassword = "client@demo.test", "demo-client-password"
)

var ErrInstalled = fmt.Errorf("this install already has data; seed only runs on an empty database")

// Seed creates the sample workspace. It refuses when setup has already happened.
func Seed(ctx context.Context, d *db.DB, st *storage.Local, version string) error {
	if _, installed, err := d.Setting(ctx, "installed"); err != nil {
		return err
	} else if installed {
		return ErrInstalled
	}
	// Files are written before the transaction; a failed seed leaves harmless orphans in files/.
	logo1, err := st.Save(strings.NewReader("%PDF-1.4\n% BriefRelay sample: logo, first draft\n"), 1<<20)
	if err != nil {
		return err
	}
	logo2, err := st.Save(strings.NewReader("%PDF-1.4\n% BriefRelay sample: logo, second draft\n"), 1<<20)
	if err != nil {
		return err
	}
	inv, err := st.Save(strings.NewReader("%PDF-1.4\n% BriefRelay sample: invoice INV-001\n"), 1<<20)
	if err != nil {
		return err
	}
	return d.Tx(ctx, func(tx *sql.Tx) error {
		s := seeder{tx: tx, t: time.Now().Add(-21 * 24 * time.Hour)}
		ws := s.id()
		s.exec(`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, ws, "Northwind Studio", s.now(), s.now())
		owner := s.user(ws, "owner", "Olivia Owner", OwnerEmail, OwnerPassword)
		staff := s.user(ws, "staff", "Sam Staff", StaffEmail, StaffPassword)
		client := s.user(ws, "client", "Casey Client", ClientEmail, ClientPassword)

		blue := s.org(ws, owner, "Blue Bakery", "hello@bluebakery.test", "Pays on time. Prefers PDF proofs.")
		s.contact(blue, client, "Casey Client", ClientEmail, "Marketing lead")
		s.contact(blue, "", "Pat Baker", "pat@bluebakery.test", "Founder")
		green := s.org(ws, owner, "Green Grocer", "info@greengrocer.test", "")
		s.contact(green, "", "Gil Green", "gil@greengrocer.test", "Owner")
		red := s.org(ws, owner, "Red Records", "", "Archived after the 2025 rebrand.")
		s.exec(`UPDATE client_orgs SET archived_at = ? WHERE id = ?`, s.now(), red)

		// Project 1: active, mid-review. This is the one the demo client logs into.
		p1 := s.project(ws, blue, owner, "Blue Bakery website", "Five-page marketing site with online ordering link.")
		s.exec(`INSERT INTO project_members (project_id, user_id, created_at) VALUES (?, ?, ?)`, p1, staff, s.now())
		answers, _ := json.Marshal(map[string]string{
			"goals": "Sell more birthday cakes online.", "audience": "Families within 10 km.", "deliverables": "Logo refresh, brand book, website copy.",
			"deadline": "End of next month.", "budget": "3,000 to 4,000 USD.", "references": "https://example.test/bakery-we-like", "notes": "Keep the blue.",
		})
		s.exec(`INSERT INTO intake_responses (id, project_id, version, status, answers, submitted_by, submitted_at, created_at, updated_at) VALUES (?, ?, 1, 'submitted', ?, ?, ?, ?, ?)`,
			s.id(), p1, string(answers), client, s.now(), s.now(), s.now())
		s.comment(p1, "intake", p1, owner, "client", "Which cakes should be featured first on the ordering page?")
		s.comment(p1, "intake", p1, client, "client", "Birthday cakes first, then cupcakes.")
		s.exec(`INSERT INTO milestones (id, project_id, title, target_date, status, visibility, latest_update, sort_order, created_by, created_at, updated_at) VALUES (?, ?, 'Kickoff', ?, 'done', 'client', 'Brief approved.', 1, ?, ?, ?)`, s.id(), p1, s.date(-14), owner, s.now(), s.now())
		s.exec(`INSERT INTO milestones (id, project_id, title, target_date, status, visibility, latest_update, sort_order, created_by, created_at, updated_at) VALUES (?, ?, 'Design review', ?, 'in_progress', 'client', 'Second logo draft shared.', 2, ?, ?, ?)`, s.id(), p1, s.date(3), owner, s.now(), s.now())
		s.exec(`INSERT INTO milestones (id, project_id, title, target_date, status, visibility, latest_update, sort_order, created_by, created_at, updated_at) VALUES (?, ?, 'Internal QA', ?, 'planned', 'internal', '', 3, ?, ?, ?)`, s.id(), p1, s.date(10), staff, s.now(), s.now())

		logo := s.deliverable(p1, owner, "Logo refresh", "Primary mark plus a one-colour variant.", 1)
		f1 := s.file(logo1, "logo-v1.pdf", staff)
		v1 := s.version(logo, 1, "file", f1, "", "First draft with three colour options.", "revision_requested", staff)
		s.exec(`INSERT INTO decisions (id, version_id, type, by_user, note, created_at) VALUES (?, ?, 'revision_requested', ?, 'Option B is close, but the blue is too dark.', ?)`, s.id(), v1, client, s.now())
		s.comment(p1, "version", v1, client, "client", "Can we see option B with the lighter blue from the old sign?")
		s.comment(p1, "version", v1, staff, "internal", "Client always picks the middle option. Prepare B only.")
		f2 := s.file(logo2, "logo-v2.pdf", staff)
		v2 := s.version(logo, 2, "file", f2, "", "Option B with the lighter blue.", "shared", staff)
		s.comment(p1, "version", v2, staff, "client", "Lighter blue as requested. Let us know.")
		s.notify(client, "version.shared", v2, "Logo refresh v2 is ready for review", "/portal/deliverables/"+logo)

		brand := s.deliverable(p1, owner, "Brand book", "Colours, type, usage rules.", 1)
		bv := s.version(brand, 1, "link", "", "https://example.test/blue-bakery-brand-book", "Shared as a link to the design tool.", "approved", owner)
		s.exec(`INSERT INTO decisions (id, version_id, type, by_user, note, created_at) VALUES (?, ?, 'approved', ?, 'Approved. Nice work.', ?)`, s.id(), bv, client, s.now())
		copyD := s.deliverable(p1, staff, "Homepage copy", "Optional: only if the client wants us to write it.", 0)
		s.version(copyD, 1, "link", "", "https://example.test/homepage-copy-draft", "Draft, not shared yet.", "draft", staff)

		invf := s.file(inv, "INV-001.pdf", owner)
		s.invoice(p1, owner, "INV-001", 150000, "USD", s.date(-7), "paid", "https://pay.example.test/inv-001", invf, "client")
		s.invoice(p1, owner, "INV-002", 150000, "USD", s.date(14), "sent", "https://pay.example.test/inv-002", "", "client")
		s.invoice(p1, owner, "INV-003", 50000, "USD", "", "draft", "", "", "internal")
		for _, a := range []string{"project.created", "intake.submitted", "version.shared", "decision.revision_requested", "version.shared", "decision.approved", "invoice.paid"} {
			s.audit(ws, owner, a, "project", p1, p1)
		}

		// Project 2: closed with a sign-off and a waiver, to show the handoff record.
		p2 := s.project(ws, green, owner, "Green Grocer menu boards", "Printed menu boards for two shops.")
		md := s.deliverable(p2, owner, "Menu board artwork", "", 1)
		mv := s.version(md, 1, "link", "", "https://example.test/menu-boards-final", "Print-ready.", "approved", owner)
		s.exec(`INSERT INTO decisions (id, version_id, type, by_user, note, created_at) VALUES (?, ?, 'approved', ?, '', ?)`, s.id(), mv, owner, s.now())
		photos := s.deliverable(p2, owner, "Shop photos", "", 1)
		s.exec(`INSERT INTO waivers (id, deliverable_id, by_user, reason, created_at) VALUES (?, ?, ?, 'Client supplied their own photos.', ?)`, s.id(), photos, owner, s.now())
		snap, _ := json.Marshal(map[string]any{"deliverables": []map[string]any{
			{"ID": md, "Title": "Menu board artwork", "LatestState": "approved", "LatestNumber": 1, "Satisfied": true},
			{"ID": photos, "Title": "Shop photos", "LatestState": "", "LatestNumber": 0, "Waiver": "Client supplied their own photos.", "Satisfied": true},
		}})
		s.exec(`INSERT INTO signoffs (id, project_id, by_user, snapshot, ip, created_at) VALUES (?, ?, ?, ?, '203.0.113.7', ?)`, s.id(), p2, owner, string(snap), s.now())
		s.exec(`UPDATE projects SET status = 'closed', closed_at = ?, closed_by = ? WHERE id = ?`, s.now(), owner, p2)
		s.audit(ws, owner, "signoff.recorded", "project", p2, p2)
		s.audit(ws, owner, "project.closed", "project", p2, p2)

		// Project 3: brand new, empty, so the empty states are visible.
		s.project(ws, blue, staff, "Blue Bakery spring campaign", "")

		for k, v := range map[string]string{"installed": "1", "installed_at": s.now(), "installed_version": version, "seeded": "1"} {
			if err := db.SetSetting(ctx, tx, k, v); err != nil {
				return err
			}
		}
		return s.err
	})
}

// Reset wipes every business table and uploaded file, then seeds again. Demo mode only.
func Reset(ctx context.Context, d *db.DB, st *storage.Local, version string) error {
	err := d.Tx(ctx, func(tx *sql.Tx) error {
		// Child tables first; jobs are kept so the recurring reset job itself is not re-enqueued.
		for _, t := range []string{"notifications", "signoffs", "waivers", "comments", "decisions", "deliverable_versions", "deliverables",
			"milestones", "intake_responses", "invoices", "project_members", "projects", "client_contacts", "client_orgs", "invitations",
			"password_resets", "sessions", "memberships", "files", "audit_events", "users", "workspaces", "settings"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+t); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	entries, _ := os.ReadDir(st.Dir)
	for _, e := range entries {
		if e.Name() != "tmp" {
			os.RemoveAll(filepath.Join(st.Dir, e.Name()))
		}
	}
	return Seed(ctx, d, st, version)
}

// seeder collects the first error so the inserts above read as data, not error handling.
type seeder struct {
	tx  *sql.Tx
	t   time.Time
	err error
}

func (s *seeder) id() string { return db.NewID() }

// now advances a few minutes per call so ordering by created_at looks natural.
func (s *seeder) now() string {
	s.t = s.t.Add(7 * time.Minute)
	return db.Time(s.t)
}

func (s *seeder) date(days int) string { return time.Now().AddDate(0, 0, days).Format("2006-01-02") }

func (s *seeder) exec(q string, args ...any) {
	if s.err != nil {
		return
	}
	_, s.err = s.tx.Exec(q, args...)
}

func (s *seeder) user(ws, role, name, email, password string) string {
	id := s.id()
	s.exec(`INSERT INTO users (id, email, name, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, id, email, name, auth.HashPassword(password), s.now(), s.now())
	s.exec(`INSERT INTO memberships (workspace_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`, ws, id, role, s.now())
	return id
}

func (s *seeder) org(ws, by, name, email, notes string) string {
	id := s.id()
	s.exec(`INSERT INTO client_orgs (id, workspace_id, name, email, notes, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, ws, name, email, notes, by, s.now(), s.now())
	return id
}

func (s *seeder) contact(org, userID, name, email, title string) {
	var uid any
	if userID != "" {
		uid = userID
	}
	s.exec(`INSERT INTO client_contacts (id, client_org_id, user_id, name, email, title, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, s.id(), org, uid, name, email, title, s.now())
}

func (s *seeder) project(ws, org, by, name, summary string) string {
	id := s.id()
	s.exec(`INSERT INTO projects (id, workspace_id, client_org_id, name, summary, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, ws, org, name, summary, by, s.now(), s.now())
	s.exec(`INSERT INTO project_members (project_id, user_id, created_at) VALUES (?, ?, ?)`, id, by, s.now())
	return id
}

func (s *seeder) deliverable(project, by, title, desc string, required int) string {
	id := s.id()
	s.exec(`INSERT INTO deliverables (id, project_id, title, description, required, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, project, title, desc, required, by, s.now(), s.now())
	return id
}

func (s *seeder) file(info storage.Info, name, by string) string {
	id := s.id()
	s.exec(`INSERT INTO files (id, storage_key, name, size, sha256, media_type, uploaded_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, info.Key, name, info.Size, info.SHA256, info.MediaType, by, s.now())
	return id
}

func (s *seeder) version(deliverable string, n int, kind, fileID, url, note, state, by string) string {
	id := s.id()
	var fid, shared any
	if fileID != "" {
		fid = fileID
	}
	if state != "draft" {
		shared = s.now()
	}
	s.exec(`INSERT INTO deliverable_versions (id, deliverable_id, number, kind, file_id, url, note, state, shared_at, shared_by, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, deliverable, n, kind, fid, url, note, state, shared, by, by, s.now(), s.now())
	return id
}

func (s *seeder) comment(project, targetType, targetID, author, visibility, body string) {
	s.exec(`INSERT INTO comments (id, target_type, target_id, project_id, author_id, body, visibility, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, s.id(), targetType, targetID, project, author, body, visibility, s.now())
}

func (s *seeder) invoice(project, by, number string, cents int64, cur, due, status, payURL, fileID, vis string) {
	var fid any
	if fileID != "" {
		fid = fileID
	}
	s.exec(`INSERT INTO invoices (id, project_id, number, amount_cents, currency, due_date, status, payment_url, file_id, visibility, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.id(), project, number, cents, cur, due, status, payURL, fid, vis, by, s.now(), s.now())
}

func (s *seeder) notify(user, kind, record, title, url string) {
	s.exec(`INSERT INTO notifications (id, user_id, kind, title, url, created_at) VALUES (?, ?, ?, ?, ?, ?)`, s.id(), user, kind, title, url, s.now())
	_ = record
}

func (s *seeder) audit(ws, actor, action, targetType, targetID, projectID string) {
	s.exec(`INSERT INTO audit_events (workspace_id, actor_id, action, target_type, target_id, ip, meta, created_at) VALUES (?, ?, ?, ?, ?, '203.0.113.7', ?, ?)`,
		ws, actor, action, targetType, targetID, `{"project_id":"`+projectID+`"}`, s.now())
}
