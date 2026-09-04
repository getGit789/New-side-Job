-- Phase 7: workspace settings (plan §5.4), per-user email preference (contract §7).
ALTER TABLE workspaces ADD COLUMN contact      TEXT NOT NULL DEFAULT '';
ALTER TABLE workspaces ADD COLUMN time_zone    TEXT NOT NULL DEFAULT 'UTC';
ALTER TABLE workspaces ADD COLUMN date_format  TEXT NOT NULL DEFAULT '2006-01-02';
ALTER TABLE workspaces ADD COLUMN currency     TEXT NOT NULL DEFAULT 'USD';
ALTER TABLE workspaces ADD COLUMN allowed_ext  TEXT NOT NULL DEFAULT '';   -- comma list; empty = any type not on the deny-list
ALTER TABLE workspaces ADD COLUMN logo_file_id TEXT REFERENCES files(id);
ALTER TABLE users ADD COLUMN email_notifications INTEGER NOT NULL DEFAULT 1;
