-- Foundation: identity, sessions, invitations, files, jobs, audit, settings.
-- Timestamps are RFC3339 UTC text. IDs are random hex so nothing can be enumerated.

CREATE TABLE workspaces (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE users (
  id            TEXT PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE,          -- stored lower-cased
  name          TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

CREATE TABLE memberships (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role         TEXT NOT NULL CHECK (role IN ('owner','staff','client')),
  created_at   TEXT NOT NULL,
  PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX memberships_user ON memberships(user_id);

CREATE TABLE sessions (
  token_hash TEXT PRIMARY KEY,                 -- sha256 of the cookie value
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  ip         TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user ON sessions(user_id);

CREATE TABLE invitations (
  id           TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  email        TEXT NOT NULL,
  role         TEXT NOT NULL CHECK (role IN ('staff','client')),
  token_hash   TEXT NOT NULL UNIQUE,
  created_by   TEXT NOT NULL REFERENCES users(id),
  created_at   TEXT NOT NULL,
  expires_at   TEXT NOT NULL,
  accepted_at  TEXT,
  revoked_at   TEXT
);

CREATE TABLE files (
  id          TEXT PRIMARY KEY,
  storage_key TEXT NOT NULL UNIQUE,            -- random name on disk, never the user's name
  name        TEXT NOT NULL,
  size        INTEGER NOT NULL,
  sha256      TEXT NOT NULL,
  media_type  TEXT NOT NULL,
  uploaded_by TEXT REFERENCES users(id),
  created_at  TEXT NOT NULL,
  deleted_at  TEXT
);

CREATE TABLE jobs (
  id           INTEGER PRIMARY KEY,
  kind         TEXT NOT NULL,
  payload      BLOB,
  dedupe_key   TEXT UNIQUE,                    -- set it when running twice would confuse a client
  run_at       TEXT NOT NULL,
  attempts     INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 5,
  locked_at    TEXT,
  last_error   TEXT,
  done_at      TEXT,
  created_at   TEXT NOT NULL
);
CREATE INDEX jobs_ready ON jobs(run_at) WHERE done_at IS NULL;

CREATE TABLE audit_events (
  id           INTEGER PRIMARY KEY,
  workspace_id TEXT,
  actor_id     TEXT,
  action       TEXT NOT NULL,                  -- e.g. auth.login, setup.completed
  target_type  TEXT NOT NULL DEFAULT '',
  target_id    TEXT NOT NULL DEFAULT '',
  ip           TEXT NOT NULL DEFAULT '',
  meta         TEXT NOT NULL DEFAULT '{}',     -- JSON, never secrets
  created_at   TEXT NOT NULL
);
CREATE INDEX audit_events_ws_time ON audit_events(workspace_id, created_at);

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
