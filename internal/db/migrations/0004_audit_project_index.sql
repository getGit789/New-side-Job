-- Phase 5: the project activity panel filters audit rows by the project id inside meta.
-- An expression index keeps that a seek instead of a scan over every event in the workspace.
-- The query must use exactly this expression: json_extract(meta, '$.project_id').
CREATE INDEX audit_events_project ON audit_events(json_extract(meta, '$.project_id'), id);
