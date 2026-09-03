// Package auth holds password hashing, random tokens, and DB-backed sessions.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"briefrelay/internal/db"
)

// Argon2id parameters (OWASP 2024 minimum: m=19 MiB, t=2, p=1). Stored in the hash so they can change later.
const (
	argonMemory  = 19 * 1024
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
)

const SessionTTL = 7 * 24 * time.Hour

var ErrInvalidHash = errors.New("auth: invalid password hash")

func HashPassword(password string) string {
	salt := randomBytes(16)
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrInvalidHash
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrInvalidHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ErrInvalidHash
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// NewToken returns a 256-bit random URL-safe token. Store only HashToken(token).
func NewToken() string { return base64.RawURLEncoding.EncodeToString(randomBytes(32)) }

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// CreateSession stores a new session and returns the raw cookie value.
func CreateSession(ctx context.Context, tx *sql.Tx, userID, ip, userAgent string) (token string, err error) {
	token = NewToken()
	_, err = tx.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, created_at, expires_at, ip, user_agent) VALUES (?, ?, ?, ?, ?, ?)`,
		HashToken(token), userID, db.Now(), db.Time(time.Now().Add(SessionTTL)), ip, truncate(userAgent, 200))
	return token, err
}

type User struct {
	ID, Email, Name string
	Role            string // owner | staff | client
	WorkspaceID     string
}

// UserBySession resolves a cookie value to a user; ok is false for missing or expired sessions.
func UserBySession(ctx context.Context, d *db.DB, token string) (u User, ok bool, err error) {
	var expires string
	err = d.R.QueryRowContext(ctx, `SELECT u.id, u.email, u.name, m.role, m.workspace_id, s.expires_at
		FROM sessions s JOIN users u ON u.id = s.user_id JOIN memberships m ON m.user_id = u.id
		WHERE s.token_hash = ? ORDER BY m.created_at LIMIT 1`,
		HashToken(token)).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.WorkspaceID, &expires)
	if err == sql.ErrNoRows {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	exp, err := db.ParseTime(expires)
	if err != nil || time.Now().After(exp) {
		return User{}, false, nil
	}
	return u, true, nil
}

func DeleteSession(ctx context.Context, tx *sql.Tx, token string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, HashToken(token))
	return err
}

// DeleteExpiredSessions is run by the scheduler.
func DeleteExpiredSessions(ctx context.Context, d *db.DB) error {
	_, err := d.W.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, db.Now())
	return err
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
