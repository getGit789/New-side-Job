-- Phase 6: self-service password reset. Tokens are random, hashed, single use, expire in 1 hour.
CREATE TABLE password_resets (
  token_hash TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  used_at    TEXT
);
CREATE INDEX password_resets_user ON password_resets(user_id);
