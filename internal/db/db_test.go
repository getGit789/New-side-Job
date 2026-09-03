package db

import (
	"context"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

func TestMigrateIsRepeatable(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	applied, err := d.Migrate(ctx)
	if err != nil || len(applied) == 0 {
		t.Fatalf("first migrate: applied=%v err=%v", applied, err)
	}
	applied, err = d.Migrate(ctx)
	if err != nil || len(applied) != 0 {
		t.Fatalf("second migrate must be a no-op: applied=%v err=%v", applied, err)
	}
	var mode string
	if err := d.W.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode=%q err=%v", mode, err)
	}
	var fk int
	if err := d.R.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys=%d err=%v", fk, err)
	}
}

// Upgrade drill: a database left by the first release (0001 + 0002 only, with data)
// must migrate to the current schema with every row intact.
func TestUpgradeFromFirstRelease(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	d.W.Exec(`CREATE TABLE schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`)
	for _, name := range []string{"0001_foundation.sql", "0002_domain.sql"} {
		body, _ := migrations.ReadFile("migrations/" + name)
		if _, err := d.W.ExecContext(ctx, string(body)); err != nil {
			t.Fatal(name, err)
		}
		d.W.Exec(`INSERT INTO schema_migrations VALUES (?, ?)`, name, Now())
	}
	now := Now()
	for _, q := range []string{
		`INSERT INTO workspaces VALUES ('w1', 'Acme', ?, ?)`,
		`INSERT INTO users (id, email, name, password_hash, created_at, updated_at) VALUES ('u1', 'a@a.test', 'Ann', 'x', ?, ?)`,
		`INSERT INTO client_orgs (id, workspace_id, name, created_at, updated_at) VALUES ('c1', 'w1', 'Blue', ?, ?)`,
		`INSERT INTO projects (id, workspace_id, client_org_id, name, created_at, updated_at) VALUES ('p1', 'w1', 'c1', 'Site', ?, ?)`,
		`INSERT INTO invitations (id, workspace_id, email, role, token_hash, created_by, created_at, expires_at) VALUES ('i1', 'w1', 'b@b.test', 'staff', 'h', 'u1', ?, ?)`,
	} {
		if _, err := d.W.Exec(q, now, now); err != nil {
			t.Fatal(q, err)
		}
	}
	applied, err := d.Migrate(ctx)
	if err != nil || len(applied) < 2 {
		t.Fatalf("upgrade: applied=%v err=%v", applied, err)
	}
	var n int
	if err := d.R.QueryRow(`SELECT count(*) FROM projects p JOIN client_orgs c ON c.id = p.client_org_id JOIN workspaces w ON w.id = c.workspace_id`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows lost in upgrade: n=%d err=%v", n, err)
	}
	if _, err := d.W.Exec(`UPDATE invitations SET contact_id = NULL WHERE id = 'i1'`); err != nil {
		t.Fatal("new column missing after upgrade:", err)
	}
	if _, err := d.R.Query(`SELECT id FROM comments LIMIT 1`); err != nil {
		t.Fatal("new table missing after upgrade:", err)
	}
}

// Migrations are append-only and never destroy data, so a downgrade of the binary
// against an upgraded database keeps working (plan §6.2 "backward-safe").
func TestMigrationsAreAppendOnlyAndSafe(t *testing.T) {
	entries, _ := fs.ReadDir(migrations, "migrations")
	name := regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)
	destructive := regexp.MustCompile(`(?i)\bDROP\s+(TABLE|COLUMN)\b|\bALTER\s+TABLE\s+\w+\s+RENAME\b`)
	for i, e := range entries {
		m := name.FindStringSubmatch(e.Name())
		if m == nil {
			t.Errorf("%s: migration names are NNNN_snake_case.sql", e.Name())
			continue
		}
		if n, _ := strconv.Atoi(m[1]); n != i+1 {
			t.Errorf("%s: expected number %04d (no gaps, no reordering)", e.Name(), i+1)
		}
		body, _ := migrations.ReadFile("migrations/" + e.Name())
		if loc := destructive.FindIndex(body); loc != nil {
			t.Errorf("%s: destructive statement %q; add a new table or column instead", e.Name(), body[loc[0]:loc[1]])
		}
	}
}
