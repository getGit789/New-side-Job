package seed

import (
	"context"
	"path/filepath"
	"testing"

	"briefrelay/internal/db"
	"briefrelay/internal/storage"
)

func TestSeedAndResetAreRepeatable(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	st, _ := storage.NewLocal(filepath.Join(dir, "files"))
	if err := Seed(ctx, d, st, "test"); err != nil {
		t.Fatal(err)
	}
	if err := Seed(ctx, d, st, "test"); err != ErrInstalled {
		t.Fatalf("second seed must refuse, got %v", err)
	}
	count := func(q string) int {
		var n int
		if err := d.R.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(q, err)
		}
		return n
	}
	before := count(`SELECT count(*) FROM deliverable_versions`)
	if before == 0 || count(`SELECT count(*) FROM signoffs`) != 1 || count(`SELECT count(*) FROM users`) != 3 {
		t.Fatal("seed shape is wrong")
	}
	// Simulate demo use, then reset: the extra row disappears and the shape is back.
	d.W.Exec(`INSERT INTO client_orgs (id, workspace_id, name, created_at, updated_at) SELECT 'x', id, 'Visitor', created_at, created_at FROM workspaces`)
	if err := Reset(ctx, d, st, "test"); err != nil {
		t.Fatal(err)
	}
	if count(`SELECT count(*) FROM client_orgs WHERE name = 'Visitor'`) != 0 || count(`SELECT count(*) FROM deliverable_versions`) != before {
		t.Fatal("reset did not restore the sample data")
	}
	if _, ok, _ := d.Setting(ctx, "installed"); !ok {
		t.Fatal("reset must leave the install marked as installed")
	}
	// Every file row must have its blob.
	rows, _ := d.R.Query(`SELECT storage_key FROM files`)
	for rows.Next() {
		var key string
		rows.Scan(&key)
		f, err := st.Open(key)
		if err != nil {
			t.Fatal("missing blob after reset:", key)
		}
		f.Close()
	}
	rows.Close()
}
