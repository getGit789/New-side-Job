-- Phase 4: comments, waivers, sign-offs, notifications, contact invitations.

ALTER TABLE invitations ADD COLUMN contact_id TEXT REFERENCES client_contacts(id);

CREATE TABLE comments (
  id          TEXT PRIMARY KEY,
  target_type TEXT NOT NULL CHECK (target_type IN ('version','intake')),
  target_id   TEXT NOT NULL,
  project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  author_id   TEXT NOT NULL REFERENCES users(id),
  body        TEXT NOT NULL,
  visibility  TEXT NOT NULL CHECK (visibility IN ('internal','client')),
  created_at  TEXT NOT NULL,
  deleted_at  TEXT
);
CREATE INDEX comments_target ON comments(target_type, target_id, created_at);

CREATE TABLE waivers (
  id             TEXT PRIMARY KEY,
  deliverable_id TEXT NOT NULL REFERENCES deliverables(id) ON DELETE CASCADE,
  by_user        TEXT NOT NULL REFERENCES users(id),
  reason         TEXT NOT NULL,
  created_at     TEXT NOT NULL
);
CREATE INDEX waivers_deliverable ON waivers(deliverable_id);

CREATE TABLE signoffs (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  by_user    TEXT NOT NULL REFERENCES users(id),
  snapshot   TEXT NOT NULL,                     -- JSON: required deliverables, approved versions, waivers
  ip         TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX signoffs_project ON signoffs(project_id);

CREATE TABLE notifications (
  id         TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL,
  title      TEXT NOT NULL,
  url        TEXT NOT NULL,
  read_at    TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX notifications_user ON notifications(user_id, read_at, created_at);
