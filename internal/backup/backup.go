// Package backup writes and restores one archive holding the database snapshot and every uploaded file.
//
// Layout of the .tar.gz: app.db (a consistent VACUUM INTO snapshot), then files/<shard>/<shard>/<key>.
// Restore only ever writes into an empty data directory, so it can never overwrite a live install.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"briefrelay/internal/db"
)

const dbName = "app.db"

// Write streams a backup of d and filesDir to w. It is safe while the app is running:
// VACUUM INTO produces a transactionally consistent copy without stopping writers for long.
func Write(ctx context.Context, d *db.DB, filesDir string, w io.Writer) error {
	snap := filepath.Join(os.TempDir(), fmt.Sprintf("briefrelay-snapshot-%d.db", os.Getpid()))
	os.Remove(snap) // VACUUM INTO refuses to overwrite
	defer os.Remove(snap)
	if _, err := d.W.ExecContext(ctx, `VACUUM INTO ?`, snap); err != nil {
		return fmt.Errorf("snapshot database: %w", err)
	}
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	if err := addFile(tw, snap, dbName); err != nil {
		return err
	}
	err := filepath.WalkDir(filesDir, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(filesDir, path)
		if e.IsDir() {
			if rel == "tmp" { // half-written uploads are not data
				return filepath.SkipDir
			}
			return nil
		}
		return addFile(tw, path, "files/"+filepath.ToSlash(rel))
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func addFile(tw *tar.Writer, path, name string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: st.Size(), ModTime: st.ModTime()}); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

// Restore unpacks an archive made by Write into dataDir, which must not already hold a database.
func Restore(r io.Reader, dataDir string) error {
	if _, err := os.Stat(filepath.Join(dataDir, dbName)); err == nil {
		return fmt.Errorf("%s already contains %s; restore into an empty data directory", dataDir, dbName)
	}
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	sawDB := false
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.ToSlash(h.Name)
		if name != dbName && !strings.HasPrefix(name, "files/") || strings.Contains(name, "..") || filepath.IsAbs(name) {
			return fmt.Errorf("archive entry %q is not a BriefRelay backup entry", h.Name)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		dst := filepath.Join(dataDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return err
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, tr)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		sawDB = sawDB || name == dbName
	}
	if !sawDB {
		return errors.New("archive holds no database")
	}
	return os.MkdirAll(filepath.Join(dataDir, "files", "tmp"), 0o750)
}
