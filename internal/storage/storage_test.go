package storage

import (
	"io"
	"strings"
	"testing"
)

func TestSaveOpenDeleteAndLimit(t *testing.T) {
	l, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	info, err := l.Save(strings.NewReader("<html>hello</html>"), 100)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != 18 || !strings.HasPrefix(info.MediaType, "text/html") || len(info.SHA256) != 64 {
		t.Fatalf("info %+v", info)
	}
	f, err := l.Open(info.Key)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(f)
	f.Close()
	if string(b) != "<html>hello</html>" {
		t.Fatalf("content %q", b)
	}
	if _, err := l.Save(strings.NewReader(strings.Repeat("x", 101)), 100); err != ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
	if _, err := l.Open("../../etc/passwd"); err == nil {
		t.Fatal("path traversal key must be rejected")
	}
	if err := l.Delete(info.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Open(info.Key); err == nil {
		t.Fatal("deleted file still opens")
	}
}
