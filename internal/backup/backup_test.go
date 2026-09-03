package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"briefrelay/internal/db"
	"briefrelay/internal/storage"
)

// The backup-and-restore drill: what goes in must come out, into a fresh directory only.
func TestWriteThenRestore(t *testing.T) {
	ctx := context.Background()
	src := t.TempDir()
	d, err := db.Open(filepath.Join(src, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := d.Tx(ctx, func(tx *sql.Tx) error { return db.SetSetting(ctx, tx, "installed", "yes") }); err != nil {
		t.Fatal(err)
	}
	st, _ := storage.NewLocal(filepath.Join(src, "files"))
	info, err := st.Save(strings.NewReader("%PDF-1.4 hello"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(src, "files", "tmp", "upload-abandoned"), []byte("junk"), 0o600)

	var buf bytes.Buffer
	if err := Write(ctx, d, st.Dir, &buf); err != nil {
		t.Fatal(err)
	}
	d.Close()

	dst := t.TempDir()
	if err := Restore(bytes.NewReader(buf.Bytes()), dst); err != nil {
		t.Fatal(err)
	}
	if err := Restore(bytes.NewReader(buf.Bytes()), dst); err == nil {
		t.Fatal("restore into a directory that holds a database must refuse")
	}
	d2, err := db.Open(filepath.Join(dst, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	if applied, err := d2.Migrate(ctx); err != nil || len(applied) != 0 {
		t.Fatalf("restored database must already be migrated: %v %v", applied, err)
	}
	if v, ok, _ := d2.Setting(ctx, "installed"); !ok || v != "yes" {
		t.Fatalf("setting lost in restore: %q %v", v, ok)
	}
	st2, _ := storage.NewLocal(filepath.Join(dst, "files"))
	f, err := st2.Open(info.Key)
	if err != nil {
		t.Fatal("uploaded file missing after restore:", err)
	}
	body, _ := io.ReadAll(f)
	f.Close()
	if string(body) != "%PDF-1.4 hello" {
		t.Fatalf("file content changed: %q", body)
	}
	if _, err := os.Stat(filepath.Join(dst, "files", "tmp", "upload-abandoned")); err == nil {
		t.Fatal("temp uploads must not be backed up")
	}
}

func TestRestoreRejectsPathTraversal(t *testing.T) {
	var buf bytes.Buffer
	// A hand-made archive with an entry escaping the data directory.
	newGzipTar(&buf, map[string]string{"app.db": "x", "files/../../etc/passwd": "pwned"})
	if err := Restore(bytes.NewReader(buf.Bytes()), t.TempDir()); err == nil {
		t.Fatal("traversal entry must be rejected")
	}
}

func newGzipTar(w io.Writer, entries map[string]string) io.Writer {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for name, body := range entries {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(body))})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return w
}
