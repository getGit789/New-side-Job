package auth

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"briefrelay/internal/db"
)

func TestPasswordRoundTrip(t *testing.T) {
	h := HashPassword("correct horse")
	if ok, err := VerifyPassword(h, "correct horse"); err != nil || !ok {
		t.Fatalf("valid password rejected: ok=%v err=%v", ok, err)
	}
	if ok, _ := VerifyPassword(h, "wrong"); ok {
		t.Fatal("wrong password accepted")
	}
	if _, err := VerifyPassword("garbage", "x"); err == nil {
		t.Fatal("malformed hash must error")
	}
}

func TestSessionLifecycle(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var token string
	err = d.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO users (id,email,name,password_hash,created_at,updated_at) VALUES ('u1','a@b.c','A','x',?,?)`, db.Now(), db.Now()); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO workspaces (id,name,created_at,updated_at) VALUES ('w1','W',?,?)`, db.Now(), db.Now()); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO memberships (workspace_id,user_id,role,created_at) VALUES ('w1','u1','owner',?)`, db.Now()); err != nil {
			return err
		}
		token, err = CreateSession(ctx, tx, "u1", "127.0.0.1", "test")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	u, ok, err := UserBySession(ctx, d, token)
	if err != nil || !ok || u.ID != "u1" || u.Role != "owner" || u.WorkspaceID != "w1" {
		t.Fatalf("lookup: ok=%v err=%v u=%+v", ok, err, u)
	}
	if _, ok, _ := UserBySession(ctx, d, "not-a-token"); ok {
		t.Fatal("unknown token must not resolve")
	}
	if err := d.Tx(ctx, func(tx *sql.Tx) error { return DeleteSession(ctx, tx, token) }); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := UserBySession(ctx, d, token); ok {
		t.Fatal("deleted session must not resolve")
	}
}

func TestCheckPassword(t *testing.T) {
	for pw, ok := range map[string]bool{"short": false, "1234567890123": true, "trustno1trustno1": true, "Password1234": true} {
		if (CheckPassword(pw) == nil) != ok {
			t.Errorf("CheckPassword(%q) ok=%v, want %v", pw, !ok, ok)
		}
	}
	// A list entry of 12+ characters must be refused even though it passes the length rule.
	for _, line := range strings.Split(commonList, "\n") {
		if len(line) >= 12 {
			if CheckPassword(line) == nil {
				t.Errorf("common password %q accepted", line)
			}
			return
		}
	}
}
