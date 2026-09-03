-- Phase 3: clients, projects, intake, milestones, deliverables, versions, decisions, invoices.
-- Rules: docs/product-contract.md §2.

CREATE TABLE client_orgs (
  id           TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  email        TEXT NOT NULL DEFAULT '',
  phone        TEXT NOT NULL DEFAULT '',
  notes        TEXT NOT NULL DEFAULT '',        -- internal, never shown to clients
  archived_at  TEXT,
  created_by   TEXT REFERENCES users(id),
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL
);
CREATE INDEX client_orgs_ws ON client_orgs(workspace_id, archived_at, name);

CREATE TABLE client_contacts (
  id            TEXT PRIMARY KEY,
  client_org_id TEXT NOT NULL REFERENCES client_orgs(id) ON DELETE CASCADE,
  user_id       TEXT REFERENCES users(id),      -- set when the invitation is accepted
  name          TEXT NOT NULL,
  email         TEXT NOT NULL,
  title         TEXT NOT NULL DEFAULT '',
  removed_at    TEXT,
  created_at    TEXT NOT NULL
);
CREATE INDEX client_contacts_org ON client_contacts(client_org_id);
CREATE UNIQUE INDEX client_contacts_user ON client_contacts(user_id) WHERE user_id IS NOT NULL;

CREATE TABLE projects (
  id            TEXT PRIMARY KEY,
  workspace_id  TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  client_org_id TEXT NOT NULL REFERENCES client_orgs(id),
  name          TEXT NOT NULL,
  summary       TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','closed')),
  closed_at     TEXT,
  closed_by     TEXT REFERENCES users(id),
  created_by    TEXT REFERENCES users(id),
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);
CREATE INDEX projects_ws ON projects(workspace_id, status, updated_at);
CREATE INDEX projects_org ON projects(client_org_id);

CREATE TABLE project_members (
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, user_id)
);
CREATE INDEX project_members_user ON project_members(user_id);

CREATE TABLE intake_responses (
  id           TEXT PRIMARY KEY,
  project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  version      INTEGER NOT NULL,
  status       TEXT NOT NULL CHECK (status IN ('draft','submitted')),
  answers      TEXT NOT NULL DEFAULT '{}',       -- JSON, fixed field set
  submitted_by TEXT REFERENCES users(id),
  submitted_at TEXT,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  UNIQUE (project_id, version)
);

CREATE TABLE milestones (
  id            TEXT PRIMARY KEY,
  project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  title         TEXT NOT NULL,
  target_date   TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned','in_progress','done')),
  visibility    TEXT NOT NULL DEFAULT 'internal' CHECK (visibility IN ('internal','client')),
  latest_update TEXT NOT NULL DEFAULT '',
  sort_order    INTEGER NOT NULL DEFAULT 0,
  created_by    TEXT REFERENCES users(id),
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);
CREATE INDEX milestones_project ON milestones(project_id, sort_order);

CREATE TABLE deliverables (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  title       TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  required    INTEGER NOT NULL DEFAULT 1,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  created_by  TEXT REFERENCES users(id),
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
CREATE INDEX deliverables_project ON deliverables(project_id, sort_order);

CREATE TABLE deliverable_versions (
  id             TEXT PRIMARY KEY,
  deliverable_id TEXT NOT NULL REFERENCES deliverables(id) ON DELETE CASCADE,
  number         INTEGER NOT NULL,
  kind           TEXT NOT NULL CHECK (kind IN ('file','link')),
  file_id        TEXT REFERENCES files(id),
  url            TEXT NOT NULL DEFAULT '',
  note           TEXT NOT NULL DEFAULT '',
  state          TEXT NOT NULL DEFAULT 'draft' CHECK (state IN ('draft','shared','revision_requested','approved','withdrawn')),
  shared_at      TEXT,
  shared_by      TEXT REFERENCES users(id),
  reopen_reason  TEXT NOT NULL DEFAULT '',
  created_by     TEXT REFERENCES users(id),
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL,
  UNIQUE (deliverable_id, number)
);

CREATE TABLE decisions (
  id         TEXT PRIMARY KEY,
  version_id TEXT NOT NULL REFERENCES deliverable_versions(id),
  type       TEXT NOT NULL CHECK (type IN ('revision_requested','approved')),
  by_user    TEXT NOT NULL REFERENCES users(id),
  note       TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX decisions_version ON decisions(version_id);

CREATE TABLE invoices (
  id           TEXT PRIMARY KEY,
  project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  number       TEXT NOT NULL,
  amount_cents INTEGER NOT NULL CHECK (amount_cents >= 0),
  currency     TEXT NOT NULL,
  due_date     TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','sent','paid','canceled')),
  payment_url  TEXT NOT NULL DEFAULT '',
  file_id      TEXT REFERENCES files(id),
  visibility   TEXT NOT NULL DEFAULT 'client' CHECK (visibility IN ('internal','client')),
  created_by   TEXT REFERENCES users(id),
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL
);
CREATE INDEX invoices_project ON invoices(project_id);
