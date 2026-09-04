# BriefRelay

Self-hosted client approval and project handoff portal. One binary, one data folder.

## Run it locally

```bash
cp .env.example .env         # edit BRIEFRELAY_ENV=development for http://localhost
make run                     # builds ./dist/briefrelay and starts it on http://127.0.0.1:8080
```

Open http://127.0.0.1:8080 and the setup page creates the owner account. Setup then locks itself.

## Commands

| Command | What it does |
|---|---|
| `briefrelay serve` | web server + background worker + scheduler (default) |
| `briefrelay check` | preflight: data dir, database, migrations, mail, proxy settings |
| `briefrelay migrate` | apply pending migrations and exit |
| `briefrelay seed` | load the sample workspace into an empty install (owner@demo.test / demo-owner-password) |
| `briefrelay backup FILE.tar.gz` | consistent snapshot of the database and uploaded files; safe while running |
| `briefrelay restore FILE.tar.gz` | unpack a backup into an empty data directory |
| `briefrelay version` | print the build version |
| `GET /healthz` | JSON readiness for application, database, storage, jobs, mail; 503 when degraded |

Docs: [install](docs/install.md) · [user guide](docs/user-guide.md) · [operations](docs/operations.md) ·
[performance](docs/performance.md) · [release checklist](docs/release-checklist.md) · [changelog](CHANGELOG.md).

## What works today (Phases 3 to 6)

Owner and staff: clients and contacts, projects with staff assignment, milestones with client visibility,
deliverables with immutable file or link versions, share and withdraw, comments (internal or client-visible),
invoice records with documents and payment links, waivers, close and reopen, staff invitations, audit log.

Clients: single-use portal invitation, their organization's projects only, client-visible milestones,
shared versions with download, comments, revision requests and approvals bound to one version, the fixed
project brief with clarification thread, client-visible invoices, and final sign-off that closes the project.

Everyone: in-app notifications plus queued email with dedupe keys, so no event is mailed twice; password change
with re-authentication; password reset by single-use link.

Owner: CSV client import and export, project hand-over export (zip), permanent deletion of closed projects and
archived clients, workspace settings (name, logo, contact details, time zone, date format, default currency,
allowed upload types).

Also (Phase 7): search across clients, projects and deliverables; change own email with re-authentication;
per-user email notification switch; delete own comment within 15 minutes (tombstone stays); edit invoice records
while draft or sent; delete a deliverable while nothing was shared; terms notice on client invitations; passwords
checked against the shipped 10k common-password list.

Demo: `BRIEFRELAY_ENV=demo` seeds the sample workspace at start, shows the sample logins on the login page,
never sends email, caps uploads at 5 MB, blocks password changes, and resets everything every hour.

Rules the server enforces are in [docs/product-contract.md](docs/product-contract.md).

Interface: `internal/web/static/app.css` is the whole design system (tokens at the top), `app.js` only adds
confirmations and the current-page marker. The typeface ships with the binary; pages make no external requests.

Hardening (Phase 5): a permission-matrix test covers every route for every role, a seeded benchmark checks the
performance budgets ([docs/performance.md](docs/performance.md)), backup/restore and upgrade drills run in the
test suite, and the release gates are listed in [docs/release-checklist.md](docs/release-checklist.md).

## Development

```bash
make test      # go test -race ./...
make lint      # gofmt, go vet, govulncheck
make perf      # seeded benchmark (500 clients / 5k projects / 25k versions / 100k events) against the p95 budgets
make release   # customer package: binaries, docs, deploy files, changelog, licenses, SBOM, checksums
make package-test  # install from the exact archive, seed, serve, log in, back up
```

Layout:

```
cmd/briefrelay/       entry point and subcommands
internal/config/      .env + environment loading, validation
internal/db/          SQLite open (reader pool + single writer), embedded migrations, audit helper
internal/auth/        argon2id passwords, tokens, DB sessions
internal/jobs/        SQLite-backed queue: retries, backoff, dedupe keys, recurring jobs
internal/storage/     private file store, random names, content sniffing, size limits
internal/mail/        SMTP (STARTTLS or 465), log-only when unconfigured
internal/domain/      approval state machine, roles, invoice statuses (no HTTP, no SQL)
internal/web/         router, middleware, authz.go (all scoping), one file per area, templates
docs/adr/             architecture decisions
```

Migrations: add `internal/db/migrations/NNNN_name.sql`. They apply in order at startup, each in a transaction, once.

Production: bind to 127.0.0.1, put Caddy or nginx in front for TLS, set `BRIEFRELAY_TRUST_PROXY=true`, back up the whole data directory.
