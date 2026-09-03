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
| `briefrelay version` | print the build version |
| `GET /healthz` | JSON readiness for database, storage, jobs, mail; 503 when degraded |

## Development

```bash
make test      # go test -race ./...
make lint      # gofmt, go vet, govulncheck
make release   # static linux binaries + SHA256SUMS + dependency list in ./dist
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
internal/web/         router, middleware, setup wizard, login, health, templates
docs/adr/             architecture decisions
```

Migrations: add `internal/db/migrations/NNNN_name.sql`. They apply in order at startup, each in a transaction, once.

Production: bind to 127.0.0.1, put Caddy or nginx in front for TLS, set `BRIEFRELAY_TRUST_PROXY=true`, back up the whole data directory.
