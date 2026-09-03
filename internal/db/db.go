// Package db opens the SQLite database and applies embedded migrations.
//
// SQLite allows one writer at a time, so we keep two pools: a single-connection
// writer that opens every transaction as BEGIN IMMEDIATE (no upgrade deadlocks),
// and a small reader pool for everything else.
package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type DB struct {
	R *sql.DB // reads
	W *sql.DB // writes, single connection
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	base := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	w, err := sql.Open("sqlite", base+"&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	w.SetMaxOpenConns(1)
	r, err := sql.Open("sqlite", base)
	if err != nil {
		w.Close()
		return nil, err
	}
	r.SetMaxOpenConns(8)
	d := &DB{R: r, W: w}
	if err := d.W.Ping(); err != nil {
		d.Close()
		return nil, fmt.Errorf("open database: %w", err)
	}
	return d, nil
}

func (d *DB) Close() error {
	rerr := d.R.Close()
	werr := d.W.Close()
	if rerr != nil {
		return rerr
	}
	return werr
}

// Tx runs fn inside a write transaction and commits if fn returns nil.
func (d *DB) Tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.W.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Migrate applies every embedded migration not yet recorded, each in its own transaction.
func (d *DB) Migrate(ctx context.Context) (applied []string, err error) {
	if _, err := d.W.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		var n int
		if err := d.W.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version = ?`, name).Scan(&n); err != nil {
			return applied, err
		}
		if n > 0 {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return applied, err
		}
		err = d.Tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(body)); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, name, Now())
			return err
		})
		if err != nil {
			return applied, fmt.Errorf("migration %s: %w", name, err)
		}
		applied = append(applied, name)
	}
	return applied, nil
}

// Now returns the canonical timestamp format used in every table.
func Now() string { return Time(time.Now()) }

func Time(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func ParseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }

// NewID returns a 24-character random hex identifier.
func NewID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// Setting reads one key from settings; ok is false when the key is absent.
func (d *DB) Setting(ctx context.Context, key string) (value string, ok bool, err error) {
	err = d.R.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return value, err == nil, err
}

func SetSetting(ctx context.Context, tx *sql.Tx, key, value string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// Audit appends an audit event. meta must already be JSON and must never contain secrets.
func Audit(ctx context.Context, tx *sql.Tx, workspaceID, actorID, action, targetType, targetID, ip, metaJSON string) error {
	if metaJSON == "" {
		metaJSON = "{}"
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_events (workspace_id, actor_id, action, target_type, target_id, ip, meta, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		nullIfEmpty(workspaceID), nullIfEmpty(actorID), action, targetType, targetID, ip, metaJSON, Now())
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
