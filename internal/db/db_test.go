package db

import (
	"context"
	"path/filepath"
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
