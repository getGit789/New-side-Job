# Operations: backup, restore, update, retention

Everything BriefRelay stores lives in one directory, `BRIEFRELAY_DATA_DIR` (default `./data`):

```
data/app.db        SQLite database (plus app.db-wal / app.db-shm while running)
data/files/        uploaded deliverables and invoice documents, random names
data/files/tmp/    half-written uploads, purged hourly after 24 h
```

## Backup

Safe while the server is running. Produces one `.tar.gz` holding a consistent database snapshot and every uploaded file.

```bash
./briefrelay backup /backups/briefrelay-$(date +%F).tar.gz
```

The command refuses to overwrite an existing file. Run it from cron daily and keep at least 7 copies off the machine.
Do **not** copy `app.db` with `cp` while the server runs; the WAL file may be mid-write. Use the command.

## Restore

Restore only writes into a data directory that has no database yet, so it can never clobber a live install.

```bash
systemctl stop briefrelay                       # or docker compose down
mv data data.broken-$(date +%F)
BRIEFRELAY_DATA_DIR=./data ./briefrelay restore /backups/briefrelay-2026-09-01.tar.gz
./briefrelay check
systemctl start briefrelay
```

`check` must print `Preflight passed.` before you start the server.

## Update to a new release

1. Read `CHANGELOG.md` for the release. It lists new settings and any manual step.
2. Pre-update check on the running version:
   ```bash
   ./briefrelay check
   curl -fsS http://127.0.0.1:8080/healthz
   ```
3. Back up (see above). This is the rollback point.
4. Stop the server, replace the binary, start the server. Migrations apply on start and are logged as `migrations applied`.
   To apply them separately first: `./briefrelay migrate`.
5. Confirm: `/healthz` returns `"status":"ok"` and the version you expect.

Migrations are append-only. They add tables, columns and indexes and never drop or rename, so a database
that was upgraded still works with the previous binary if you have to roll back quickly.

## Rollback

- Quick: stop, put the previous binary back, start. The newer schema is backward-safe (tested in `internal/db`).
- Full: stop, move `data` aside, `restore` the pre-update backup, start the previous binary.

## Health endpoint

`GET /healthz` returns JSON with `status`, `version`, `installed`, per-check results for `application`,
`database`, `storage`, `jobs`, `mail`, and queue counts. HTTP 503 when any check fails. No secrets.
Mail shows "not configured" rather than failing when `BRIEFRELAY_SMTP_HOST` is empty.

## Retention and deletion

| Data | Rule |
|---|---|
| Sessions | expire after 7 days; expired rows purged hourly |
| Invitations | expire after 7 days; single use; revocable from Team |
| Deliverable versions, decisions, sign-offs, audit events | never deleted; they are the approval record |
| Comments | never deleted (no delete action yet; the schema keeps a `deleted_at` tombstone column for it) |
| Uploaded files | a version's file is removed only when a draft version is deleted; the blob is purged 7 days later |
| Finished background jobs | purged daily |
| Client contact removed | loses portal access at once (sessions deleted); their comments and decisions stay |
| Staff removed | loses access at once; audit rows keep their id |

Not built yet (planned for Phase 6): project archive export, permanent client-organization deletion,
account self-deletion. Until then, deletion of a whole client is a manual database operation; back up first.

## Logs

JSON lines on stdout. Level from `BRIEFRELAY_LOG_LEVEL`. Passwords, tokens and file contents are never logged.
Invitation links are logged only when mail is not configured, so a local install can still be tested.
