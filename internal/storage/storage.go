// Package storage writes uploaded files to a private directory with random names.
// Files are written to a temp file and renamed, so an interrupted upload never
// leaves a half-written object under a real key.
package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrTooLarge = errors.New("storage: file exceeds the size limit")

type Local struct {
	Dir string
}

type Info struct {
	Key       string
	Size      int64
	SHA256    string
	MediaType string // detected from content, not from the client
}

func NewLocal(dir string) (*Local, error) {
	if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0o750); err != nil {
		return nil, err
	}
	return &Local{Dir: dir}, nil
}

// Save streams r to disk, enforcing maxBytes. It never trusts the client's file name or content type.
func (l *Local) Save(r io.Reader, maxBytes int64) (Info, error) {
	tmp, err := os.CreateTemp(filepath.Join(l.Dir, "tmp"), "upload-*")
	if err != nil {
		return Info{}, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	head := make([]byte, 512)
	n, _ := io.ReadFull(r, head)
	head = head[:n]
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(io.MultiReader(strings.NewReader(string(head)), r), maxBytes+1))
	if err != nil {
		return Info{}, err
	}
	if size > maxBytes {
		return Info{}, ErrTooLarge
	}
	if err := tmp.Sync(); err != nil {
		return Info{}, err
	}
	tmp.Close()

	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return Info{}, err
	}
	key := hex.EncodeToString(b[:])
	final := l.path(key)
	if err := os.MkdirAll(filepath.Dir(final), 0o750); err != nil {
		return Info{}, err
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		return Info{}, err
	}
	return Info{Key: key, Size: size, SHA256: hex.EncodeToString(h.Sum(nil)), MediaType: http.DetectContentType(head)}, nil
}

func (l *Local) Open(key string) (*os.File, error) {
	if !validKey(key) {
		return nil, os.ErrNotExist
	}
	return os.Open(l.path(key))
}

func (l *Local) Delete(key string) error {
	if !validKey(key) {
		return os.ErrNotExist
	}
	err := os.Remove(l.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Writable is used by health and preflight checks.
func (l *Local) Writable() error {
	f, err := os.CreateTemp(filepath.Join(l.Dir, "tmp"), "probe-*")
	if err != nil {
		return fmt.Errorf("files dir not writable: %w", err)
	}
	f.Close()
	return os.Remove(f.Name())
}

// CleanTemp removes abandoned upload temp files older than the given age.
func (l *Local) CleanTemp(olderThan time.Duration) error {
	entries, err := os.ReadDir(filepath.Join(l.Dir, "tmp"))
	if err != nil {
		return err
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > olderThan {
			os.Remove(filepath.Join(l.Dir, "tmp", e.Name()))
		}
	}
	return nil
}

// path shards keys into ab/cd/abcd... so one directory never holds every file.
func (l *Local) path(key string) string {
	return filepath.Join(l.Dir, key[:2], key[2:4], key)
}

func validKey(key string) bool {
	if len(key) != 32 {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}
