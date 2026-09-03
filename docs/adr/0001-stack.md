# ADR 0001 — Language and stack

**Status:** accepted (2026-09-03). Benchmark gate remains open until Phase 5 seeds the reference data set.

## Decision

Go (1.27), standard library first, SQLite (pure-Go driver, WAL), one static binary that runs the web server,
background worker, and scheduler. No separate services. HTML rendered on the server.

## Why Go and not Rust, PHP, or Node

| Criterion (plan §7) | Go | Rust | PHP/Laravel | Node |
|---|---|---|---|---|
| 1. Runtime performance for this workload (DB and file I/O bound) | Excellent | Excellent, marginal gain | Adequate | Good |
| 2. Ease of install for buyers | One file + one folder | Same | Composer, PHP version, cron, queue worker | Node version, npm install |
| 3. Security support and dependency health | 4 direct deps | Many crates | Large surface | Very large surface |
| 4. Auth/migration/queue/mail/test tooling | stdlib + small code | Immature for this shape | Mature | Mature |
| 5. Runs without creator cloud | Yes | Yes | Yes | Yes |
| 6. Predictable resource use | ~30 MB RSS | ~10 MB | Depends on host | ~80 MB |
| 7. Maintainable by one senior engineer | Yes | Slow iteration | Yes | Yes |
| 8. Packaging and upgrade | Replace binary, migrations auto-apply | Same | Copy tree, run artisan | Copy tree, npm ci |
| 9. Marketplace fit | Lower than PHP | Lower | Highest | Medium |

"Fastest" is asked for. Rust's raw edge does not show up in a workload that waits on SQLite and disk, and it costs
criteria 4 and 7. Go is the fastest language that still scores well on install and maintenance.

**Known cost of this choice:** CodeCanyon's largest audience runs PHP shared hosting, which cannot run a Go binary.
The buyer profile in the plan (VPS, Docker, or "ask their developer") can. If Phase 0 validation shows most buyers
are on shared PHP hosting, this ADR is reversed and the same foundation shape is rebuilt on Laravel.

## Consequences

- Dependencies: `modernc.org/sqlite` (no C compiler needed), `golang.org/x/crypto` (argon2id), `golang.org/x/time` (rate limit). Everything else is stdlib.
- SQLite is the only supported database in v1 (plan §6.5: one canonical path). Seeded scale (500 clients, 5k projects,
  25k versions, 100k events) is far inside SQLite's comfort zone. PostgreSQL adapter is a v1.2 candidate at most.
- Jobs and scheduler live in the same process and the same database; a job is enqueued in the same transaction as
  the business write, so nothing is ever queued for a change that did not commit.
- CSRF uses the stdlib `http.CrossOriginProtection` (Fetch-Metadata based), so forms carry no token.
- Benchmark harness for the p95 budgets is built in Phase 5 with the seed command; this ADR is re-read then.
